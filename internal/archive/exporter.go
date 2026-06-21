package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/metrics"
)

// StreamExportData is the analytics stream row exported to blob storage.
type StreamExportData struct {
	StreamID          string     `json:"streamId"`
	BroadcasterID     string     `json:"broadcasterId"`
	Login             string     `json:"login"`
	DisplayName       string     `json:"displayName,omitempty"`
	Title             string     `json:"title,omitempty"`
	Category          string     `json:"category,omitempty"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	VodID             string     `json:"vodId,omitempty"`
	ViewerSource      string     `json:"viewerSource,omitempty"`
	CanonicalStreamID string     `json:"canonicalStreamId,omitempty"`
	AvgViewers        int        `json:"avgViewers"`
	PeakViewers       int        `json:"peakViewers"`
	ViewerSamples     int        `json:"viewerSamples"`
	ExportedAt        time.Time  `json:"exportedAt"`
}

// RollupExportLine is one JSONL row for minute rollups.
type RollupExportLine struct {
	MinuteTS          time.Time      `json:"minuteTs"`
	ViewerAvg         int            `json:"viewerAvg"`
	ViewerMax         int            `json:"viewerMax"`
	ViewerLatest      int            `json:"viewerLatest"`
	ViewerSamples     int            `json:"viewerSamples"`
	ChatCount         int            `json:"chatCount"`
	TotalEmoteCount   int            `json:"totalEmoteCount"`
	SevenTVEmoteCount int            `json:"seventvEmoteCount"`
	Emotes            map[string]int `json:"emotes,omitempty"`
}

// AnalyticsDB reads stream + rollup rows for export.
type AnalyticsDB interface {
	ExportStreamRow(ctx context.Context, streamID string) (*StreamExportData, error)
	ExportRollups(ctx context.Context, streamID string) ([]RollupExportLine, error)
}

// VODChatExportLine is one JSONL row for archived VOD chat messages.
type VODChatExportLine struct {
	ID             int64           `json:"id"`
	StreamID       string          `json:"streamId"`
	MinuteTS       time.Time       `json:"minuteTs"`
	MessageID      string          `json:"messageId"`
	DisplayName    string          `json:"displayName"`
	CommenterLogin string          `json:"commenterLogin,omitempty"`
	SenderHash     string          `json:"senderHash"`
	Text           string          `json:"text"`
	EmoteFrags     json.RawMessage `json:"emoteFrags,omitempty"`
	OffsetSeconds  int             `json:"offsetSeconds"`
	SyncedAt       time.Time       `json:"syncedAt"`
}

// VODChatDB reads persisted VOD chat messages for export.
type VODChatDB interface {
	ExportVODChatMessages(ctx context.Context, streamID string) ([]VODChatExportLine, error)
}

// PgxAnalyticsDB implements AnalyticsDB and VODChatDB against Postgres.
type PgxAnalyticsDB struct {
	db *pgxpool.Pool
}

func NewPgxAnalyticsDB(db *pgxpool.Pool) *PgxAnalyticsDB {
	return &PgxAnalyticsDB{db: db}
}

func (d *PgxAnalyticsDB) ExportStreamRow(ctx context.Context, streamID string) (*StreamExportData, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("archive export: db unavailable")
	}
	var row StreamExportData
	var endedAt *time.Time
	var canonicalID, viewerSource string
	err := d.db.QueryRow(ctx, `
		SELECT stream_id, COALESCE(broadcaster_id,''), login, COALESCE(display_name,''),
			COALESCE(title,''), COALESCE(category,''), started_at, ended_at,
			COALESCE(vod_id,''), COALESCE(viewer_source,''),
			COALESCE(NULLIF(canonical_stream_id,''), stream_id),
			avg_viewers, peak_viewers, viewer_samples
		FROM analytics_streams
		WHERE stream_id = $1`, streamID,
	).Scan(
		&row.StreamID, &row.BroadcasterID, &row.Login, &row.DisplayName,
		&row.Title, &row.Category, &row.StartedAt, &endedAt,
		&row.VodID, &viewerSource, &canonicalID,
		&row.AvgViewers, &row.PeakViewers, &row.ViewerSamples,
	)
	if err != nil {
		return nil, err
	}
	row.EndedAt = endedAt
	row.ViewerSource = viewerSource
	row.CanonicalStreamID = canonicalID
	row.ExportedAt = time.Now().UTC()
	return &row, nil
}

func (d *PgxAnalyticsDB) ExportRollups(ctx context.Context, streamID string) ([]RollupExportLine, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("archive export: db unavailable")
	}
	rows, err := d.db.Query(ctx, `
		SELECT minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		FROM analytics_minute_rollups
		WHERE stream_id = $1
		ORDER BY minute_ts ASC`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RollupExportLine
	for rows.Next() {
		var line RollupExportLine
		var emotesRaw []byte
		if err := rows.Scan(
			&line.MinuteTS, &line.ViewerAvg, &line.ViewerMax, &line.ViewerLatest, &line.ViewerSamples,
			&line.ChatCount, &line.TotalEmoteCount, &line.SevenTVEmoteCount, &emotesRaw,
		); err != nil {
			return nil, err
		}
		if len(emotesRaw) > 0 {
			_ = json.Unmarshal(emotesRaw, &line.Emotes)
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func (d *PgxAnalyticsDB) ExportVODChatMessages(ctx context.Context, streamID string) ([]VODChatExportLine, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("archive export: db unavailable")
	}
	rows, err := d.db.Query(ctx, `
		SELECT id, stream_id, minute_ts, message_id, display_name, COALESCE(commenter_login,''),
			sender_hash, text, emote_frags, offset_seconds, synced_at
		FROM analytics_vod_chat_messages
		WHERE stream_id = $1
		ORDER BY offset_seconds ASC, id ASC`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VODChatExportLine
	for rows.Next() {
		var line VODChatExportLine
		if err := rows.Scan(
			&line.ID, &line.StreamID, &line.MinuteTS, &line.MessageID, &line.DisplayName,
			&line.CommenterLogin, &line.SenderHash, &line.Text, &line.EmoteFrags,
			&line.OffsetSeconds, &line.SyncedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

// SyncExporter implements analytics.SyncArchiveExporter and analytics.VODChatExporter.
type SyncExporter struct {
	writer        *Writer
	db            AnalyticsDB
	chatDB        VODChatDB
	emoteExporter *EmoteExporter
}

func NewSyncExporter(writer *Writer, db AnalyticsDB) *SyncExporter {
	exp := &SyncExporter{writer: writer, db: db}
	if chatDB, ok := db.(VODChatDB); ok {
		exp.chatDB = chatDB
	}
	return exp
}

func (e *SyncExporter) WithEmoteExporter(emoteExporter *EmoteExporter) *SyncExporter {
	if e != nil {
		e.emoteExporter = emoteExporter
	}
	return e
}

func (e *SyncExporter) ExportSync(ctx context.Context, streamID, channel, resultMessage string) error {
	if e == nil || e.writer == nil || e.db == nil {
		return fmt.Errorf("archive exporter is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("archive export: stream id is required")
	}
	return e.writer.ExportStream(ctx, streamID, e.db)
}

func (e *SyncExporter) ExportVODChat(ctx context.Context, streamID string) error {
	if e == nil || e.writer == nil || e.chatDB == nil {
		return fmt.Errorf("archive vod chat exporter is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("archive export: stream id is required")
	}
	return e.writer.ExportVODChat(ctx, streamID, e.chatDB)
}

func (w *Writer) ExportStream(ctx context.Context, streamID string, db AnalyticsDB) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	stream, err := db.ExportStreamRow(ctx, streamID)
	if err != nil {
		return fmt.Errorf("export stream row: %w", err)
	}
	sessionRaw, err := json.Marshal(stream)
	if err != nil {
		return err
	}
	sessionRes, err := w.putGzip(ctx, StreamSessionBlobKey(streamID), sessionRaw)
	if err != nil {
		return fmt.Errorf("upload stream session: %w", err)
	}
	sessionRes.RowCount = 1
	if err := w.confirmManifest(ctx, ArtifactAnalyticsStream, streamID, sessionRes); err != nil {
		return err
	}

	if login := strings.ToLower(strings.TrimSpace(stream.Login)); login != "" {
		channelRes, err := w.putGzip(ctx, StreamChannelBlobKey(login, streamID), sessionRaw)
		if err != nil {
			return fmt.Errorf("upload stream channel index: %w", err)
		}
		channelRes.RowCount = 1
		channelKey := fmt.Sprintf("streams:%s:%s", login, streamID)
		if err := w.confirmManifest(ctx, ArtifactAnalyticsStream, channelKey, channelRes); err != nil {
			return err
		}
	}

	rollups, err := db.ExportRollups(ctx, streamID)
	if err != nil {
		return fmt.Errorf("export rollups: %w", err)
	}
	var buf strings.Builder
	for _, line := range rollups {
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	rollupPayload := []byte(buf.String())
	rollupRes, err := w.putGzip(ctx, RollupsBlobKey(streamID), rollupPayload)
	if err != nil {
		return fmt.Errorf("upload rollups: %w", err)
	}
	rollupRes.RowCount = int64(len(rollups))
	durationMin := 0
	if stream.EndedAt != nil && stream.StartedAt.Before(*stream.EndedAt) {
		durationMin = int(stream.EndedAt.Sub(stream.StartedAt).Minutes())
	}
	minCoverage := 0.5
	if w != nil && w.opts.SilverPartialMinCoverage > 0 {
		minCoverage = w.opts.SilverPartialMinCoverage
	}
	_, rollupStatus := ComputeSilverCoverage(rollups, durationMin, minCoverage)
	rollupKey := ViewerRollupKey(streamID)
	rollupRec := ExportRecord{
		ArtifactType:          ArtifactAnalyticsRollups,
		NaturalKey:            rollupKey,
		GCSURI:                rollupRes.URI,
		ETag:                  rollupRes.ETag,
		RowCount:              rollupRes.RowCount,
		ByteSize:              rollupRes.ByteSize,
		Status:                rollupStatus,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "silver",
		StreamID:              streamID,
		ChannelLogin:          strings.ToLower(strings.TrimSpace(stream.Login)),
		ContentSHA256:         rollupRes.ContentSHA256,
		UncompressedSizeBytes: rollupRes.UncompressedSizeBytes,
	}
	if sidecar := BuildSilverSidecar(streamID, stream.Login, rollups, durationMin, minCoverage); w != nil && w.opts.WriteSidecarManifest {
		if raw, err := sidecar.JSON(); err == nil {
			_, _ = w.blob.Put(ctx, SilverSidecarBlobKey(streamID), raw, "application/json")
		}
	}
	if w.manifest != nil {
		if err := w.manifest.Upsert(ctx, rollupRec); err != nil {
			return err
		}
	} else if err := w.confirmManifest(ctx, ArtifactAnalyticsRollups, rollupKey, rollupRes); err != nil {
		return err
	}
	if hiveRes, err := w.putGzip(ctx, HiveViewerRollupBlobKey(streamID), rollupPayload); err == nil {
		hiveRes.RowCount = rollupRes.RowCount
		hiveKey := fmt.Sprintf("%s:hive", streamID)
		_ = w.confirmManifest(ctx, ArtifactAnalyticsRollups, hiveKey, hiveRes)
	}
	observeTier0CoverageFromRollups(rollups)
	return nil
}

func observeTier0CoverageFromRollups(rollups []RollupExportLine) {
	if len(rollups) == 0 {
		return
	}
	withSamples := 0
	for _, line := range rollups {
		if line.ViewerSamples > 0 {
			withSamples++
		}
	}
	metrics.RecordTier0CoveragePct(float64(withSamples) / float64(len(rollups)) * 100)
}

func (w *Writer) ExportVODChat(ctx context.Context, streamID string, db VODChatDB) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	if db == nil {
		return fmt.Errorf("archive export: vod chat db is required")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("archive export: stream id is required")
	}
	messages, err := db.ExportVODChatMessages(ctx, streamID)
	if err != nil {
		return fmt.Errorf("export vod chat messages: %w", err)
	}
	var buf strings.Builder
	for _, line := range messages {
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	chatRes, err := w.putGzip(ctx, VODChatBlobKey(streamID), []byte(buf.String()))
	if err != nil {
		return fmt.Errorf("upload vod chat: %w", err)
	}
	chatRes.RowCount = int64(len(messages))
	naturalKey := fmt.Sprintf("vod_chat:%s", streamID)
	if err := w.confirmManifest(ctx, ArtifactVODChatMessage, naturalKey, chatRes); err != nil {
		return err
	}
	return w.exportVODChatProvenance(ctx, streamID, db)
}

func (w *Writer) exportVODChatProvenance(ctx context.Context, streamID string, db VODChatDB) error {
	if w == nil || w.blob == nil {
		return nil
	}
	login := ""
	if rowDB, ok := db.(AnalyticsDB); ok {
		if row, err := rowDB.ExportStreamRow(ctx, streamID); err == nil && row != nil {
			login = row.Login
		}
	}
	var prov VODChatProvenance
	if provBuilder, ok := db.(interface {
		BuildVODChatProvenance(context.Context, string, string) VODChatProvenance
	}); ok {
		prov = provBuilder.BuildVODChatProvenance(ctx, streamID, login)
	} else {
		prov = VODChatProvenance{
			StreamID:              streamID,
			Login:                 login,
			EmoteSnapshotStrategy: defaultEmoteSnapshotStrategy,
			ExportedAt:            time.Now().UTC(),
		}
	}
	raw, err := json.Marshal(prov)
	if err != nil {
		return err
	}
	_, err = w.putGzip(ctx, VODChatProvenanceBlobKey(streamID), raw)
	return err
}

// ExportTTChartJSON uploads semi-raw TT chart JSON for re-parse (TASK-012).
func (w *Writer) ExportTTChartJSON(ctx context.Context, streamID string, chartJSON []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" || len(chartJSON) == 0 {
		return nil
	}
	if w.opts.SilverRawTTMaxBytes > 0 && len(chartJSON) > w.opts.SilverRawTTMaxBytes {
		return fmt.Errorf("tt chart json exceeds max bytes (%d > %d)", len(chartJSON), w.opts.SilverRawTTMaxBytes)
	}
	if !json.Valid(chartJSON) {
		return fmt.Errorf("tt chart json is not valid JSON")
	}
	res, err := w.putGzip(ctx, TTChartJSONBlobKey(streamID), chartJSON)
	if err != nil {
		return fmt.Errorf("upload tt chart json: %w", err)
	}
	res.RowCount = 1
	fetchedAt := time.Now().UTC()
	naturalKey := TTChartJSONKey(streamID, fetchedAt)
	rec := ExportRecord{
		ArtifactType:          ArtifactTTChartJSON,
		NaturalKey:            naturalKey,
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              1,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            fetchedAt,
		Tier:                  "silver",
		StreamID:              streamID,
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if w.manifest == nil {
		return nil
	}
	return w.manifest.Upsert(ctx, rec)
}

// ExportTTDetail uploads optional TwitchTracker HTML (gap-fill / debug artifact).
func (w *Writer) ExportTTDetail(ctx context.Context, login, streamID string, html []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	streamID = strings.TrimSpace(streamID)
	if login == "" || streamID == "" {
		return fmt.Errorf("archive export: login and stream id are required for tt-detail")
	}
	if len(html) == 0 {
		return nil
	}
	res, err := w.putGzip(ctx, TTDetailBlobKey(login, streamID), html)
	if err != nil {
		return fmt.Errorf("upload tt-detail: %w", err)
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("tt-detail:%s:%s", login, streamID)
	return w.confirmManifest(ctx, ArtifactTTDetailHTML, naturalKey, res)
}

func (w *Writer) ExportPostgresDump(ctx context.Context, date string, sqlGzip []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	res, err := w.blob.Put(ctx, PostgresNightlyBlobKey(date), sqlGzip, "application/gzip")
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, "postgres_nightly", date, res)
}

func (w *Writer) ExportTopRoster(ctx context.Context, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	res, err := w.putGzip(ctx, TopRosterBlobKey(), payload)
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, "tracked_roster", "top200", res)
}

// ExportTop500 uploads the reproducible Bronze channel list (top-N + always-tracked).
func (w *Writer) ExportTop500(ctx context.Context, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	res, err := w.putGzip(ctx, Top500BlobKey(), payload)
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, ArtifactBronzeTop500, "top500", res)
}

// ExportChannelSummary uploads TwitchTracker summary JSON for one login.
func (w *Writer) ExportChannelSummary(ctx context.Context, login string, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("archive export: login is required for channel summary")
	}
	if len(payload) == 0 {
		return fmt.Errorf("archive export: empty summary payload for %q", login)
	}
	key := ChannelSummaryBlobKey(login)
	res, err := w.blob.Put(ctx, key, payload, "application/json")
	if err != nil {
		return fmt.Errorf("upload channel summary: %w", err)
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("summary:%s", login)
	return w.confirmManifest(ctx, ArtifactBronzeChannelSummary, naturalKey, res)
}

// ExportVODIndex uploads Helix archive VOD list as JSONL.gz for one login.
func (w *Writer) ExportVODIndex(ctx context.Context, login string, lines []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("archive export: login is required for vod index")
	}
	res, err := w.putGzip(ctx, VODIndexBlobKey(login), lines)
	if err != nil {
		return fmt.Errorf("upload vod index: %w", err)
	}
	rowCount := int64(bytes.Count(lines, []byte{'\n'}))
	if len(lines) > 0 && lines[len(lines)-1] != '\n' {
		rowCount++
	}
	if rowCount == 0 && len(lines) > 0 {
		rowCount = 1
	}
	res.RowCount = rowCount
	naturalKey := fmt.Sprintf("vod_index:%s", login)
	return w.confirmManifest(ctx, ArtifactBronzeVODIndex, naturalKey, res)
}
