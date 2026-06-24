package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	EmoteSnapshotStrategyWeekly = "weekly_snapshot"
	EmoteSnapshotStrategyEvent  = "eventapi_delta"
	defaultEmoteSnapshotStrategy = EmoteSnapshotStrategyWeekly
)

// EmoteSnapshotLine is one provider emote in a cold-storage snapshot.
type EmoteSnapshotLine struct {
	EmoteID         string `json:"emoteId"`
	ProviderEmoteID string `json:"providerEmoteId,omitempty"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	ProviderSetID   string `json:"providerSetId,omitempty"`
}

// EmoteSnapshotMeta accompanies snapshot blobs.
type EmoteSnapshotMeta struct {
	Login         string    `json:"login"`
	Provider      string    `json:"provider"`
	ProviderSetID string    `json:"providerSetId,omitempty"`
	EmoteHash     string    `json:"emoteHash,omitempty"`
	Count         int       `json:"count"`
	ExportedAt    time.Time `json:"exportedAt"`
	Strategy      string    `json:"strategy,omitempty"`
}

// EmoteChangelogLine is one EventAPI or manual delta row.
type EmoteChangelogLine struct {
	Provider      string          `json:"provider"`
	Login         string          `json:"login"`
	EventType     string          `json:"eventType"`
	ProviderSetID string          `json:"providerSetId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	RecordedAt    time.Time       `json:"recordedAt"`
}

// EmoteSnapshotDB reads provider emote metadata for export.
type EmoteSnapshotDB interface {
	ListProviderEmotes(ctx context.Context, login, provider string) ([]EmoteSnapshotLine, error)
	ProviderSetSnapshot(ctx context.Context, login, provider string) (providerSetID, emoteHash string, count int, ok bool, err error)
}

type VODChatProvenance struct {
	StreamID              string            `json:"streamId"`
	Login                 string            `json:"login,omitempty"`
	EmoteSnapshotStrategy string            `json:"emoteSnapshotStrategy,omitempty"`
	EmoteProviderURIs     map[string]string `json:"emoteProviderUris,omitempty"`
	EmoteProviderHashes   map[string]string `json:"emoteProviderHashes,omitempty"`
	ExportedAt            time.Time         `json:"exportedAt"`
}

func EmoteSnapshotBlobKey(provider, login, date string) string {
	provider = emoteProviderBlobSlug(provider)
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("emotes/snapshots/provider=%s/login=%s/date=%s/part-000.json.gz", provider, login, date)
}

func EmoteChangelogBlobKey(provider, login, date string) string {
	provider = emoteProviderBlobSlug(provider)
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("emotes/changelog/provider=%s/login=%s/date=%s/events.jsonl.gz", provider, login, date)
}

func emoteProviderBlobSlug(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "seventv", "7tv":
		return "7tv"
	case "ffz":
		return "ffz"
	case "bttv":
		return "bttv"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func VODChatProvenanceBlobKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("vod_chat/stream_id=%s/provenance.json.gz", streamID)
}

type emoteSnapshotLister interface {
	ListSnapshotLogins(ctx context.Context, limit int) ([]string, error)
	ListAlwaysTrackedLogins(ctx context.Context) ([]string, error)
}

// EmoteExporter uploads emote metadata snapshots and changelogs.
type EmoteExporter struct {
	writer                *Writer
	db                    EmoteSnapshotDB
	list                  emoteSnapshotLister
	changelogDiffEnabled  bool
	ffzSnapshotEnabled    bool
	bttvSnapshotEnabled   bool
}

func NewEmoteExporter(writer *Writer, db EmoteSnapshotDB) *EmoteExporter {
	exp := &EmoteExporter{writer: writer, db: db}
	if lister, ok := db.(emoteSnapshotLister); ok {
		exp.list = lister
	}
	return exp
}

func (e *EmoteExporter) WithChangelogDiff(enabled bool) *EmoteExporter {
	if e != nil {
		e.changelogDiffEnabled = enabled
	}
	return e
}

func (e *EmoteExporter) WithProviderSnapshots(ffzEnabled, bttvEnabled bool) *EmoteExporter {
	if e != nil {
		e.ffzSnapshotEnabled = ffzEnabled
		e.bttvSnapshotEnabled = bttvEnabled
	}
	return e
}

func (e *EmoteExporter) ExportSnapshot(ctx context.Context, provider, login, strategy string) error {
	if e == nil || e.writer == nil || e.db == nil {
		return fmt.Errorf("emote exporter is not configured")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	login = strings.ToLower(strings.TrimSpace(login))
	if provider == "" || login == "" {
		return fmt.Errorf("emote export: provider and login are required")
	}
	if strategy == "" {
		strategy = defaultEmoteSnapshotStrategy
	}
	lines, err := e.db.ListProviderEmotes(ctx, login, provider)
	if err != nil {
		return err
	}
	setID, hash, count, _, err := e.db.ProviderSetSnapshot(ctx, login, provider)
	if err != nil {
		return err
	}
	if count == 0 {
		count = len(lines)
	}
	if count == 0 && len(lines) == 0 {
		return nil
	}
	date := time.Now().UTC().Format("2006-01-02")
	doc := NewEmoteSnapshotDocument(EmoteSnapshotMeta{
		Login:         login,
		Provider:      provider,
		ProviderSetID: setID,
		EmoteHash:     hash,
		Count:         count,
		ExportedAt:    time.Now().UTC(),
		Strategy:      strategy,
	}, lines)
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if e.changelogDiffEnabled {
		if prior, priorErr := e.loadPriorSnapshotLines(ctx, provider, login, date); priorErr == nil && len(prior) > 0 {
			adds, removes := diffEmoteSnapshots(prior, lines)
			if err := e.writeDiffChangelog(ctx, provider, login, adds, removes); err != nil {
				return err
			}
		}
	}
	res, err := e.writer.putGzip(ctx, EmoteSnapshotBlobKey(provider, login, date), raw)
	if err != nil {
		recordArchiveExportFailed(ArtifactEmoteSnapshot)
		return err
	}
	res.RowCount = int64(len(lines))
	naturalKey := EmoteSnapshotKey(provider, login, date)
	return e.writer.confirmManifest(ctx, ArtifactEmoteSnapshot, naturalKey, res)
}

func (e *EmoteExporter) AppendChangelog(ctx context.Context, line EmoteChangelogLine) error {
	if e == nil || e.writer == nil {
		return fmt.Errorf("emote exporter is not configured")
	}
	line.Provider = strings.ToLower(strings.TrimSpace(line.Provider))
	line.Login = strings.ToLower(strings.TrimSpace(line.Login))
	if line.Provider == "" || line.Login == "" || strings.TrimSpace(line.EventType) == "" {
		return fmt.Errorf("emote changelog: provider, login, and eventType are required")
	}
	if line.RecordedAt.IsZero() {
		line.RecordedAt = time.Now().UTC()
	}
	date := line.RecordedAt.UTC().Format("2006-01-02")
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	payload := append(raw, '\n')
	res, err := e.writer.putGzip(ctx, EmoteChangelogBlobKey(line.Provider, line.Login, date), payload)
	if err != nil {
		recordArchiveExportFailed(ArtifactEmoteChangelog)
		return err
	}
	res.RowCount = 1
	naturalKey := EmoteChangelogKey(line.Provider, line.Login, line.EventType, line.RecordedAt)
	return e.writer.confirmManifest(ctx, ArtifactEmoteChangelog, naturalKey, res)
}

// ExportWeeklySnapshots exports 7TV snapshots for roster + always-tracked channels.
func (e *EmoteExporter) ExportWeeklySnapshots(ctx context.Context, force bool) (exported, skipped int, err error) {
	if e == nil || e.db == nil {
		return 0, 0, nil
	}
	_ = force
	logins := map[string]bool{}
	if e.list != nil {
		roster, listErr := e.list.ListSnapshotLogins(ctx, 500)
		if listErr != nil {
			return 0, 0, listErr
		}
		for _, login := range roster {
			logins[login] = true
		}
		always, alwaysErr := e.list.ListAlwaysTrackedLogins(ctx)
		if alwaysErr != nil {
			return 0, 0, alwaysErr
		}
		for _, login := range always {
			logins[login] = true
		}
	}
	for login := range logins {
		providers := []string{"seventv"}
		if e.ffzSnapshotEnabled {
			providers = append(providers, "ffz")
		}
		if e.bttvSnapshotEnabled {
			providers = append(providers, "bttv")
		}
		for _, provider := range providers {
			if exportErr := e.ExportSnapshot(ctx, provider, login, EmoteSnapshotStrategyWeekly); exportErr != nil {
				skipped++
				continue
			}
			exported++
		}
	}
	return exported, skipped, nil
}

// ExportGlobalSevenTVSnapshot uploads the 7TV global emote set (login=global).
func (e *EmoteExporter) ExportGlobalSevenTVSnapshot(ctx context.Context, apiURL string) error {
	if e == nil || e.writer == nil {
		return fmt.Errorf("emote exporter is not configured")
	}
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if apiURL == "" {
		apiURL = "https://7tv.io/v3"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/emote-sets/global", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("7tv global returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	date := time.Now().UTC().Format("2006-01-02")
	res, err := e.writer.putGzip(ctx, EmoteSnapshotBlobKey("7tv", "global", date), body)
	if err != nil {
		recordArchiveExportFailed(ArtifactEmoteSnapshotGlobal)
		return err
	}
	res.RowCount = 1
	naturalKey := EmoteSnapshotGlobalKey(date)
	return e.writer.confirmManifest(ctx, ArtifactEmoteSnapshotGlobal, naturalKey, res)
}

func (e *EmoteExporter) BuildVODChatProvenance(ctx context.Context, streamID, login string) VODChatProvenance {
	prov := VODChatProvenance{
		StreamID:              streamID,
		Login:                 login,
		EmoteSnapshotStrategy: defaultEmoteSnapshotStrategy,
		EmoteProviderURIs:     map[string]string{},
		EmoteProviderHashes:   map[string]string{},
		ExportedAt:            time.Now().UTC(),
	}
	if e == nil || e.db == nil || login == "" {
		return prov
	}
	for _, provider := range []string{"seventv", "ffz", "bttv"} {
		setID, hash, count, ok, err := e.db.ProviderSetSnapshot(ctx, login, provider)
		if err != nil || !ok || count == 0 {
			continue
		}
		date := time.Now().UTC().Format("2006-01-02")
		if e.writer != nil && e.writer.blob != nil {
			prov.EmoteProviderURIs[provider] = e.writer.blob.BlobURI(EmoteSnapshotBlobKey(provider, login, date))
		}
		if hash != "" {
			prov.EmoteProviderHashes[provider] = hash
		}
		_ = setID
	}
	return prov
}

func RecordArchiveExportConfirmed(artifactType string) {
	recordArchiveExportConfirmed(artifactType)
}

func RecordArchiveExportFailed(artifactType string) {
	recordArchiveExportFailed(artifactType)
}

func diffEmoteSnapshots(prev, next []EmoteSnapshotLine) (adds, removes []EmoteSnapshotLine) {
	prevMap := map[string]EmoteSnapshotLine{}
	for _, line := range prev {
		id := strings.TrimSpace(line.EmoteID)
		if id == "" {
			id = strings.TrimSpace(line.ProviderEmoteID)
		}
		if id != "" {
			prevMap[id] = line
		}
	}
	nextMap := map[string]EmoteSnapshotLine{}
	for _, line := range next {
		id := strings.TrimSpace(line.EmoteID)
		if id == "" {
			id = strings.TrimSpace(line.ProviderEmoteID)
		}
		if id != "" {
			nextMap[id] = line
		}
	}
	for id, line := range nextMap {
		if _, ok := prevMap[id]; !ok {
			adds = append(adds, line)
		}
	}
	for id, line := range prevMap {
		if _, ok := nextMap[id]; !ok {
			removes = append(removes, line)
		}
	}
	return adds, removes
}

func (e *EmoteExporter) writeDiffChangelog(ctx context.Context, provider, login string, adds, removes []EmoteSnapshotLine) error {
	now := time.Now().UTC()
	for _, line := range adds {
		payload, _ := json.Marshal(line)
		if err := e.AppendChangelog(ctx, EmoteChangelogLine{
			Provider: provider, Login: login, EventType: "add", Payload: payload, RecordedAt: now,
		}); err != nil {
			return err
		}
	}
	for _, line := range removes {
		payload, _ := json.Marshal(line)
		if err := e.AppendChangelog(ctx, EmoteChangelogLine{
			Provider: provider, Login: login, EventType: "remove", Payload: payload, RecordedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *EmoteExporter) loadPriorSnapshotLines(ctx context.Context, provider, login, currentDate string) ([]EmoteSnapshotLine, error) {
	if e == nil || e.writer == nil || e.writer.manifest == nil {
		return nil, fmt.Errorf("prior snapshot loader unavailable")
	}
	store, ok := e.writer.manifest.(*ManifestStore)
	if !ok {
		return nil, fmt.Errorf("prior snapshot loader unavailable")
	}
	priorKey, err := store.PriorEmoteSnapshotKey(ctx, provider, login, currentDate)
	if err != nil || priorKey == "" {
		return nil, err
	}
	parts := strings.Split(priorKey, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid prior natural key")
	}
	priorDate := parts[len(parts)-1]
	raw, err := e.writer.blob.Get(ctx, EmoteSnapshotBlobKey(provider, login, priorDate))
	if err != nil {
		return nil, err
	}
	gz, err := Gunzip(raw)
	if err != nil {
		return nil, err
	}
	var doc EmoteSnapshotDocument
	if err := json.Unmarshal(gz, &doc); err != nil {
		return nil, err
	}
	return doc.Emotes, nil
}
