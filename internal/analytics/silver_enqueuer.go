package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
)

// VODCatalogReader loads bronze Helix VOD index rows for one login.
type VODCatalogReader interface {
	LoadVODIndex(ctx context.Context, login string) ([]ArchivedVOD, error)
}

// ArchiveVODCatalog reads channels/vod_index/{login}.jsonl.gz from archive blob storage.
type ArchiveVODCatalog struct {
	blob archive.BlobStore
}

func NewArchiveVODCatalog(blob archive.BlobStore) *ArchiveVODCatalog {
	return &ArchiveVODCatalog{blob: blob}
}

func (c *ArchiveVODCatalog) LoadVODIndex(ctx context.Context, login string) ([]ArchivedVOD, error) {
	if c == nil || c.blob == nil {
		return nil, fmt.Errorf("archive vod catalog is not configured")
	}
	login = normalizeLogin(login)
	if login == "" {
		return nil, fmt.Errorf("login is required")
	}
	raw, err := c.blob.Get(ctx, archive.VODIndexBlobKey(login))
	if err != nil {
		return nil, err
	}
	data, err := archive.Gunzip(raw)
	if err != nil {
		return nil, err
	}
	return parseVODIndexJSONL(data), nil
}

func parseVODIndexJSONL(data []byte) []ArchivedVOD {
	out := make([]ArchivedVOD, 0, 8)
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var vod ArchivedVOD
		if err := json.Unmarshal([]byte(line), &vod); err != nil {
			continue
		}
		if vod.StreamID == "" {
			continue
		}
		out = append(out, vod)
	}
	return out
}

func filterVODsSince(vods []ArchivedVOD, since time.Time) []ArchivedVOD {
	if since.IsZero() {
		return vods
	}
	out := make([]ArchivedVOD, 0, len(vods))
	for _, vod := range vods {
		if vod.StartedAt.IsZero() {
			continue
		}
		if !vod.StartedAt.Before(since) {
			out = append(out, vod)
		}
	}
	return out
}

// SilverEnqueuerConfig bounds automatic silver backfill from bronze VOD catalogs.
type SilverEnqueuerConfig struct {
	SinceDays int
	TopN      int
	MaxPerRun int
	Interval  time.Duration
}

// SilverEnqueuer inserts silver-tier backfill_jobs from bronze vod_index blobs.
type SilverEnqueuer struct {
	db        *pgxpool.Pool
	catalog   VODCatalogReader
	sinceDays int
	topN      int
	maxPerRun int
	interval  time.Duration
}

func NewSilverEnqueuer(db *pgxpool.Pool, catalog VODCatalogReader, cfg SilverEnqueuerConfig) *SilverEnqueuer {
	if cfg.SinceDays <= 0 {
		cfg.SinceDays = 60
	}
	if cfg.TopN <= 0 {
		cfg.TopN = 200
	}
	if cfg.MaxPerRun <= 0 {
		cfg.MaxPerRun = 25
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	return &SilverEnqueuer{
		db:        db,
		catalog:   catalog,
		sinceDays: cfg.SinceDays,
		topN:      cfg.TopN,
		maxPerRun: cfg.MaxPerRun,
		interval:  cfg.Interval,
	}
}

type SilverEnqueueResult struct {
	LoginsScanned int `json:"loginsScanned"`
	VODsSeen      int `json:"vodsSeen"`
	Enqueued      int `json:"enqueued"`
	Skipped       int `json:"skipped"`
}

func (e *SilverEnqueuer) RunOnce(ctx context.Context) (SilverEnqueueResult, error) {
	out := SilverEnqueueResult{}
	if e == nil || e.db == nil || e.catalog == nil {
		return out, nil
	}
	logins, err := e.listBronzeLogins(ctx)
	if err != nil {
		return out, err
	}
	out.LoginsScanned = len(logins)
	since := time.Now().UTC().Add(-time.Duration(e.sinceDays) * 24 * time.Hour)
	store := NewStore(e.db)
	for _, login := range logins {
		if out.Enqueued >= e.maxPerRun {
			break
		}
		vods, err := e.catalog.LoadVODIndex(ctx, login)
		if err != nil {
			continue
		}
		vods = filterVODsSince(vods, since)
		out.VODsSeen += len(vods)
		for _, vod := range vods {
			if out.Enqueued >= e.maxPerRun {
				break
			}
			if !vod.StartedAt.IsZero() {
				_ = store.UpsertStreamPlaceholder(ctx, vod.StreamID, "", login, vod.Title, vod.StartedAt)
			}
			inserted, err := insertSilverBackfillJob(ctx, e.db, vod.StreamID, login)
			if err != nil {
				return out, err
			}
			if inserted {
				out.Enqueued++
			} else {
				out.Skipped++
			}
		}
	}
	return out, nil
}

func (e *SilverEnqueuer) listBronzeLogins(ctx context.Context) ([]string, error) {
	rows, err := e.db.Query(ctx, `
		SELECT t.login
		FROM tracked_streamers t
		JOIN bronze_index_state b ON b.login = t.login
		WHERE b.last_helix_at IS NOT NULL
		  AND b.helix_row_count > 0
		ORDER BY t.last_rank ASC NULLS LAST, t.login ASC
		LIMIT $1`, e.topN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logins []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		logins = append(logins, normalizeLogin(login))
	}
	return logins, rows.Err()
}

func insertSilverBackfillJob(ctx context.Context, db *pgxpool.Pool, streamID, login string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" || login == "" {
		return false, fmt.Errorf("stream id and login are required")
	}
	tag, err := db.Exec(ctx, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status, next_run_at)
		SELECT 'silver', $1, $2, 'queued', 'pending', now()
		WHERE NOT EXISTS (
			SELECT 1 FROM backfill_jobs
			WHERE stream_id = $1
			  AND tier = 'silver'
			  AND status IN ('queued', 'running', 'done', 'skipped')
		)`, streamID, login)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func StartSilverEnqueuer(ctx context.Context, enqueuer *SilverEnqueuer, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if enqueuer == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(enqueuer.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				result, err := enqueuer.RunOnce(ctx)
				if err != nil && log != nil {
					log.Warn("silver enqueuer tick failed", "err", err)
				} else if result.Enqueued > 0 && log != nil {
					log.Info("silver enqueuer inserted jobs",
						"enqueued", result.Enqueued,
						"skipped", result.Skipped,
						"logins", result.LoginsScanned,
						"vods_seen", result.VODsSeen,
					)
				}
			}
		}
	}()
}
