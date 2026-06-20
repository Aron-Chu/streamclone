package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bronzeHelixVODLimit = 80

// BronzeIndexState is one row in bronze_index_state.
type BronzeIndexState struct {
	Login           string     `json:"login"`
	LastHelixAt     *time.Time `json:"lastHelixAt,omitempty"`
	LastSummaryAt   *time.Time `json:"lastSummaryAt,omitempty"`
	HelixBlobURI    string     `json:"helixBlobUri,omitempty"`
	SummaryBlobURI  string     `json:"summaryBlobUri,omitempty"`
	HelixRowCount   int        `json:"helixRowCount"`
	Error           string     `json:"error,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type bronzeHelixClient interface {
	Enabled() bool
	ArchivedStreamHistory(ctx context.Context, login string, limit int) ([]ArchivedVOD, error)
}

type bronzeArchiveExporter interface {
	ExportTop500(ctx context.Context, payload []byte) error
	ExportVODIndex(ctx context.Context, login string, lines []byte) error
	ExportChannelSummary(ctx context.Context, login string, payload []byte) error
}

// BronzeIndexer rate-limits Helix VOD index + TwitchTracker summary exports for the roster.
type BronzeIndexer struct {
	db               *pgxpool.Pool
	helix            bronzeHelixClient
	writer           bronzeArchiveExporter
	metadataURL      string
	ttAPIURL         string
	userAgent        string
	httpClient       *http.Client
	topN             int
	alwaysTracked    map[string]bool
	helixConcurrency int
	ttConcurrency    int
	channelsPerTick  int
}

func NewBronzeIndexer(
	db *pgxpool.Pool,
	helix bronzeHelixClient,
	metadataURL, ttAPIURL, userAgent string,
	topN int,
	always []string,
	helixConcurrency, ttConcurrency int,
) *BronzeIndexer {
	if topN <= 0 {
		topN = 500
	}
	if helixConcurrency <= 0 {
		helixConcurrency = 2
	}
	if ttConcurrency <= 0 {
		ttConcurrency = 4
	}
	tracked := map[string]bool{}
	for _, login := range always {
		if login = normalizeLogin(login); login != "" {
			tracked[login] = true
		}
	}
	channelsPerTick := 1
	if topN > 0 {
		// Target full roster pass within ~24h at 5m ticks (288 ticks/day).
		channelsPerTick = (topN + 287) / 288
		if channelsPerTick < 1 {
			channelsPerTick = 1
		}
		if channelsPerTick > 4 {
			channelsPerTick = 4
		}
	}
	return &BronzeIndexer{
		db:               db,
		helix:            helix,
		metadataURL:      strings.TrimRight(metadataURL, "/"),
		ttAPIURL:         strings.TrimRight(ttAPIURL, "/"),
		userAgent:        userAgent,
		httpClient:       &http.Client{Timeout: 20 * time.Second},
		topN:             topN,
		alwaysTracked:    tracked,
		helixConcurrency: helixConcurrency,
		ttConcurrency:    ttConcurrency,
		channelsPerTick:  channelsPerTick,
	}
}

func (b *BronzeIndexer) WithWriter(writer bronzeArchiveExporter) *BronzeIndexer {
	if b != nil {
		b.writer = writer
	}
	return b
}

func (b *BronzeIndexer) WithHTTPClient(client *http.Client) *BronzeIndexer {
	if b != nil && client != nil {
		b.httpClient = client
	}
	return b
}

// RunOnce exports the channel list and indexes up to channelsPerTick stale logins.
func (b *BronzeIndexer) RunOnce(ctx context.Context) error {
	if b == nil || b.db == nil {
		return fmt.Errorf("bronze indexer unavailable")
	}
	if b.writer == nil {
		return fmt.Errorf("bronze indexer: archive writer not configured")
	}
	logins, err := b.buildChannelList(ctx)
	if err != nil {
		return err
	}
	if err := b.exportChannelList(ctx, logins); err != nil {
		return err
	}
	targets, err := b.pickStaleLogins(ctx, logins)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	helixSem := make(chan struct{}, b.helixConcurrency)
	ttSem := make(chan struct{}, b.ttConcurrency)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for _, login := range targets {
		login := login
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.indexLogin(ctx, login, helixSem, ttSem); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (b *BronzeIndexer) buildChannelList(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var logins []string
	add := func(login string) {
		login = normalizeLogin(login)
		if login == "" || seen[login] {
			return
		}
		seen[login] = true
		logins = append(logins, login)
	}
	for login := range b.alwaysTracked {
		add(login)
	}
	rows, err := b.db.Query(ctx, `
		SELECT login
		FROM tracked_streamers
		ORDER BY last_rank ASC NULLS LAST, login ASC
		LIMIT $1`, b.topN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		add(login)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(logins) == 0 && b.metadataURL != "" {
		items, fetchErr := b.fetchMetadataTopStreams(ctx)
		if fetchErr != nil {
			return nil, fetchErr
		}
		for _, item := range items {
			add(item.Login)
		}
	}
	if len(logins) > b.topN {
		logins = logins[:b.topN]
	}
	return logins, nil
}

type bronzeMetadataItem struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	Viewers     int    `json:"viewers"`
}

func (b *BronzeIndexer) fetchMetadataTopStreams(ctx context.Context) ([]bronzeMetadataItem, error) {
	url := fmt.Sprintf("%s/v1/streams?limit=%d", b.metadataURL, b.topN)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("metadata streams status %d", resp.StatusCode)
	}
	var page struct {
		Items []bronzeMetadataItem `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (b *BronzeIndexer) exportChannelList(ctx context.Context, logins []string) error {
	items := make([]map[string]any, 0, len(logins))
	for i, login := range logins {
		items = append(items, map[string]any{
			"rank":  i + 1,
			"login": login,
		})
	}
	payload, err := json.Marshal(map[string]any{
		"updatedAt": time.Now().UTC(),
		"topN":      b.topN,
		"logins":    logins,
		"items":     items,
	})
	if err != nil {
		return err
	}
	return b.writer.ExportTop500(ctx, payload)
}

func (b *BronzeIndexer) pickStaleLogins(ctx context.Context, roster []string) ([]string, error) {
	if len(roster) == 0 {
		return nil, nil
	}
	states := make(map[string]*BronzeIndexState, len(roster))
	for _, login := range roster {
		state, err := b.loadState(ctx, login)
		if err != nil {
			return nil, err
		}
		states[login] = state
	}
	return pickBronzeCandidates(roster, states, b.channelsPerTick, time.Now().UTC().Add(-24*time.Hour)), nil
}

func pickBronzeCandidates(roster []string, states map[string]*BronzeIndexState, limit int, staleCutoff time.Time) []string {
	type candidate struct {
		login string
		score time.Time
	}
	var candidates []candidate
	for _, login := range roster {
		state := states[login]
		if state == nil || state.LastHelixAt == nil || state.LastSummaryAt == nil ||
			state.LastHelixAt.Before(staleCutoff) || state.LastSummaryAt.Before(staleCutoff) {
			score := time.Time{}
			if state != nil {
				if state.LastHelixAt != nil {
					score = *state.LastHelixAt
				}
				if state.LastSummaryAt != nil && (score.IsZero() || state.LastSummaryAt.Before(score)) {
					score = *state.LastSummaryAt
				}
			}
			candidates = append(candidates, candidate{login: login, score: score})
		}
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score.Before(candidates[i].score) ||
				(candidates[j].score.IsZero() && !candidates[i].score.IsZero()) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	if limit <= 0 {
		limit = 1
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.login)
	}
	return out
}

func (b *BronzeIndexer) indexLogin(ctx context.Context, login string, helixSem, ttSem chan struct{}) error {
	login = normalizeLogin(login)
	if login == "" {
		return nil
	}
	var helixErr, summaryErr error
	var helixRows int
	var helixURI, summaryURI string

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		helixSem <- struct{}{}
		defer func() { <-helixSem }()
		helixRows, helixURI, helixErr = b.exportHelixIndex(ctx, login)
	}()
	go func() {
		defer wg.Done()
		ttSem <- struct{}{}
		defer func() { <-ttSem }()
		summaryURI, summaryErr = b.exportTTSummary(ctx, login)
	}()
	wg.Wait()

	now := time.Now().UTC()
	errMsg := ""
	if helixErr != nil {
		errMsg = "helix: " + helixErr.Error()
	}
	if summaryErr != nil {
		if errMsg != "" {
			errMsg += "; "
		}
		errMsg += "summary: " + summaryErr.Error()
	}
	_, err := b.db.Exec(ctx, `
		INSERT INTO bronze_index_state (
			login, last_helix_at, last_summary_at, helix_blob_uri, summary_blob_uri,
			helix_row_count, error, updated_at
		)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,''),$8)
		ON CONFLICT (login) DO UPDATE SET
			last_helix_at = CASE WHEN $2 IS NOT NULL THEN $2 ELSE bronze_index_state.last_helix_at END,
			last_summary_at = CASE WHEN $3 IS NOT NULL THEN $3 ELSE bronze_index_state.last_summary_at END,
			helix_blob_uri = COALESCE(NULLIF($4,''), bronze_index_state.helix_blob_uri),
			summary_blob_uri = COALESCE(NULLIF($5,''), bronze_index_state.summary_blob_uri),
			helix_row_count = CASE WHEN $6 > 0 THEN $6 ELSE bronze_index_state.helix_row_count END,
			error = NULLIF($7,''),
			updated_at = $8`,
		login,
		nullableTime(helixErr == nil, now),
		nullableTime(summaryErr == nil, now),
		helixURI, summaryURI, helixRows, errMsg, now,
	)
	if err != nil {
		return err
	}
	if helixErr != nil {
		return helixErr
	}
	if summaryErr != nil {
		return summaryErr
	}
	return nil
}

func nullableTime(ok bool, t time.Time) *time.Time {
	if !ok {
		return nil
	}
	return &t
}

func (b *BronzeIndexer) exportHelixIndex(ctx context.Context, login string) (rowCount int, blobURI string, err error) {
	if b.helix == nil || !b.helix.Enabled() {
		return 0, "", fmt.Errorf("helix client not configured")
	}
	vods, err := b.helix.ArchivedStreamHistory(ctx, login, bronzeHelixVODLimit)
	if err != nil {
		return 0, "", err
	}
	var buf strings.Builder
	for _, vod := range vods {
		line, err := json.Marshal(vod)
		if err != nil {
			return 0, "", err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := b.writer.ExportVODIndex(ctx, login, []byte(buf.String())); err != nil {
		return 0, "", err
	}
	return len(vods), fmt.Sprintf("channels/vod_index/%s.jsonl.gz", login), nil
}

func (b *BronzeIndexer) exportTTSummary(ctx context.Context, login string) (blobURI string, err error) {
	if b.ttAPIURL == "" {
		return "", fmt.Errorf("twitchtracker api url not configured")
	}
	endpoint := fmt.Sprintf("%s/channels/summary/%s", b.ttAPIURL, url.PathEscape(login))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if b.userAgent != "" {
		req.Header.Set("User-Agent", b.userAgent)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitchtracker summary status %d", resp.StatusCode)
	}
	if !json.Valid(body) {
		return "", fmt.Errorf("twitchtracker summary invalid json for %q", login)
	}
	if err := b.writer.ExportChannelSummary(ctx, login, body); err != nil {
		return "", err
	}
	return fmt.Sprintf("channels/summary/%s.json", login), nil
}

func (b *BronzeIndexer) loadState(ctx context.Context, login string) (*BronzeIndexState, error) {
	var state BronzeIndexState
	var lastHelix, lastSummary *time.Time
	err := b.db.QueryRow(ctx, `
		SELECT login, last_helix_at, last_summary_at,
			COALESCE(helix_blob_uri,''), COALESCE(summary_blob_uri,''),
			COALESCE(helix_row_count,0), COALESCE(error,''), updated_at
		FROM bronze_index_state
		WHERE login = $1`, login,
	).Scan(
		&state.Login, &lastHelix, &lastSummary,
		&state.HelixBlobURI, &state.SummaryBlobURI,
		&state.HelixRowCount, &state.Error, &state.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	state.LastHelixAt = lastHelix
	state.LastSummaryAt = lastSummary
	return &state, nil
}

// ListBronzeIndexState returns recent bronze progress rows for CLI status.
func ListBronzeIndexState(ctx context.Context, db *pgxpool.Pool, limit int) ([]BronzeIndexState, error) {
	if db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(ctx, `
		SELECT login, last_helix_at, last_summary_at,
			COALESCE(helix_blob_uri,''), COALESCE(summary_blob_uri,''),
			COALESCE(helix_row_count,0), COALESCE(error,''), updated_at
		FROM bronze_index_state
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BronzeIndexState
	for rows.Next() {
		var state BronzeIndexState
		var lastHelix, lastSummary *time.Time
		if err := rows.Scan(
			&state.Login, &lastHelix, &lastSummary,
			&state.HelixBlobURI, &state.SummaryBlobURI,
			&state.HelixRowCount, &state.Error, &state.UpdatedAt,
		); err != nil {
			return nil, err
		}
		state.LastHelixAt = lastHelix
		state.LastSummaryAt = lastSummary
		out = append(out, state)
	}
	return out, rows.Err()
}

func StartBronzeWorker(ctx context.Context, indexer *BronzeIndexer, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if indexer == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func() {
			if err := indexer.RunOnce(ctx); err != nil && log != nil {
				log.Warn("bronze indexer tick failed", "err", err)
				return
			}
			if log != nil {
				log.Info("bronze indexer tick completed")
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
