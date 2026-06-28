package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	GoldVODAvailabilityDiscovered         = "discovered"
	GoldVODAvailabilityEligible           = "eligible"
	GoldVODAvailabilityLoaded             = "loaded"
	GoldVODAvailabilityUnknownUnavailable = "unknown_unavailable"

	GoldVODStatusNotQueued = "not_queued"
	GoldVODStatusQueued    = "queued"
	GoldVODStatusRunning   = "running"
	GoldVODStatusDone      = "done"
	GoldVODStatusFailed    = "failed"
	GoldVODStatusSkipped   = "skipped"
)

// Top500GoldVODInventoryConfig bounds VOD discovery and direct Gold enqueue.
type Top500GoldVODInventoryConfig struct {
	SinceDays     int
	TopN          int
	MaxPerRun     int
	DirectEnqueue bool
	Interval      time.Duration
}

// Top500GoldVODInventoryResult is the operator-facing summary for one pass.
type Top500GoldVODInventoryResult struct {
	LoginsScanned int `json:"loginsScanned"`
	VODsSeen      int `json:"vodsSeen"`
	Upserted      int `json:"upserted"`
	GoldEnqueued  int `json:"goldEnqueued"`
	Skipped       int `json:"skipped"`
}

// Top500GoldVODInventory builds a durable VOD inventory from Top 500 roster
// VOD catalogs, then optionally creates Gold chat jobs directly per VOD.
type Top500GoldVODInventory struct {
	db            *pgxpool.Pool
	catalog       VODCatalogReader
	sinceDays     int
	topN          int
	maxPerRun     int
	directEnqueue bool
	interval      time.Duration
}

func NewTop500GoldVODInventory(db *pgxpool.Pool, catalog VODCatalogReader, cfg Top500GoldVODInventoryConfig) *Top500GoldVODInventory {
	if cfg.SinceDays <= 0 {
		cfg.SinceDays = 90
	}
	if cfg.TopN <= 0 || cfg.TopN > 500 {
		cfg.TopN = 500
	}
	if cfg.MaxPerRun <= 0 {
		cfg.MaxPerRun = 25
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	return &Top500GoldVODInventory{
		db:            db,
		catalog:       catalog,
		sinceDays:     cfg.SinceDays,
		topN:          cfg.TopN,
		maxPerRun:     cfg.MaxPerRun,
		directEnqueue: cfg.DirectEnqueue,
		interval:      cfg.Interval,
	}
}

func (b *Top500GoldVODInventory) RunOnce(ctx context.Context) (Top500GoldVODInventoryResult, error) {
	out := Top500GoldVODInventoryResult{}
	if b == nil || b.db == nil || b.catalog == nil {
		return out, nil
	}
	logins, err := b.listTop500Logins(ctx)
	if err != nil {
		return out, err
	}
	out.LoginsScanned = len(logins)
	since := time.Now().UTC().Add(-time.Duration(b.sinceDays) * 24 * time.Hour)
	store := NewStore(b.db)
	for _, login := range logins {
		if b.maxPerRun > 0 && out.Upserted >= b.maxPerRun {
			break
		}
		vods, err := b.catalog.LoadVODIndex(ctx, login)
		if err != nil {
			continue
		}
		vods = filterVODsSince(vods, since)
		out.VODsSeen += len(vods)
		for _, vod := range vods {
			if b.maxPerRun > 0 && out.Upserted >= b.maxPerRun {
				break
			}
			vod.StreamID = strings.TrimSpace(vod.StreamID)
			vod.VideoID = strings.TrimSpace(vod.VideoID)
			if vod.StreamID == "" || vod.VideoID == "" {
				out.Skipped++
				continue
			}
			if !vod.StartedAt.IsZero() {
				_ = store.UpsertStreamPlaceholder(ctx, vod.StreamID, "", login, vod.Title, vod.StartedAt)
			}
			_ = store.SetStreamVodID(ctx, vod.StreamID, vod.VideoID, "top500_vod_inventory")
			upserted, err := b.upsertVOD(ctx, login, vod)
			if err != nil {
				return out, err
			}
			if !upserted {
				out.Skipped++
				continue
			}
			out.Upserted++
			if b.directEnqueue {
				inserted, err := EnqueueGoldVODJob(ctx, b.db, vod.StreamID, login, vod.VideoID)
				if err != nil {
					return out, err
				}
				if inserted {
					out.GoldEnqueued++
				}
			}
		}
	}
	return out, nil
}

func (b *Top500GoldVODInventory) listTop500Logins(ctx context.Context) ([]string, error) {
	rows, err := b.db.Query(ctx, `
		SELECT login
		FROM top500_channels
		WHERE enabled
		ORDER BY rank ASC, login ASC
		LIMIT $1`, b.topN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		if login = normalizeLogin(login); login != "" {
			out = append(out, login)
		}
	}
	return out, rows.Err()
}

func (b *Top500GoldVODInventory) upsertVOD(ctx context.Context, login string, vod ArchivedVOD) (bool, error) {
	login = normalizeLogin(login)
	if login == "" || strings.TrimSpace(vod.VideoID) == "" {
		return false, nil
	}
	availability := GoldVODAvailabilityEligible
	if vod.StreamID == "" {
		availability = GoldVODAvailabilityDiscovered
	}
	tag, err := b.db.Exec(ctx, `
		INSERT INTO top500_vod_inventory (
			vod_id, stream_id, login, top500_rank, title, category_name, started_at, ended_at,
			duration_minutes, availability_state, source, last_checked_at, updated_at
		)
		SELECT $1, $2, $3, c.rank, $4, $5, NULLIF($6, '0001-01-01 00:00:00+00'::timestamptz), NULLIF($7, '0001-01-01 00:00:00+00'::timestamptz),
			$8, $9, 'bronze_vod_index', now(), now()
		FROM top500_channels c
		WHERE c.login = $3
		ON CONFLICT (vod_id) DO UPDATE SET
			stream_id = COALESCE(NULLIF(EXCLUDED.stream_id, ''), top500_vod_inventory.stream_id),
			login = EXCLUDED.login,
			top500_rank = EXCLUDED.top500_rank,
			title = EXCLUDED.title,
			category_name = EXCLUDED.category_name,
			started_at = EXCLUDED.started_at,
			ended_at = EXCLUDED.ended_at,
			duration_minutes = EXCLUDED.duration_minutes,
			availability_state = CASE
				WHEN top500_vod_inventory.availability_state IN ('loaded', 'expired', 'deleted', 'private_or_sub_only', 'no_chat', 'region_blocked', 'gql_forbidden')
				THEN top500_vod_inventory.availability_state
				ELSE EXCLUDED.availability_state
			END,
			last_checked_at = now(),
			updated_at = now()`,
		strings.TrimSpace(vod.VideoID),
		strings.TrimSpace(vod.StreamID),
		login,
		strings.TrimSpace(vod.Title),
		strings.TrimSpace(vod.Category),
		vod.StartedAt,
		vod.EndedAt,
		vod.DurationMinutes,
		availability,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// EnqueueGoldVODJob creates a Gold chat job for a discovered VOD without making
// Silver viewer-chart success a hard prerequisite.
func EnqueueGoldVODJob(ctx context.Context, db *pgxpool.Pool, streamID, login, vodID string) (bool, error) {
	jobID, inserted, err := insertGoldBackfillJobReturningID(ctx, db, streamID, login)
	if err != nil {
		return false, err
	}
	if !inserted {
		return false, nil
	}
	_, _ = db.Exec(ctx, `
		UPDATE top500_vod_inventory
		SET gold_status='queued',
		    gold_backfill_job_id=$2,
		    gold_queued_at=now(),
		    updated_at=now()
		WHERE vod_id=$1`, strings.TrimSpace(vodID), jobID)
	return true, nil
}

func markGoldVODInventoryOutcome(ctx context.Context, db *pgxpool.Pool, job BackfillJob, outcome backfillOutcome) {
	if db == nil || !isGoldFullTier(job.Tier) {
		return
	}
	status := GoldVODStatusFailed
	availability := GoldVODAvailabilityUnknownUnavailable
	var completedAt *time.Time
	switch outcome.status {
	case "done":
		status = GoldVODStatusDone
		availability = GoldVODAvailabilityLoaded
		now := time.Now().UTC()
		completedAt = &now
	case "queued":
		status = GoldVODStatusQueued
		availability = GoldVODAvailabilityEligible
	case "skipped":
		status = GoldVODStatusSkipped
	case "running":
		status = GoldVODStatusRunning
	}
	_, _ = db.Exec(ctx, `
		UPDATE top500_vod_inventory
		SET gold_status=$2,
		    availability_state=$3,
		    gold_completed_at=COALESCE($4, gold_completed_at),
		    archive_export_status=$5,
		    error=COALESCE(NULLIF($6,''), error),
		    updated_at=now()
		WHERE stream_id=$1`,
		strings.TrimSpace(job.StreamID),
		status,
		availability,
		completedAt,
		outcome.exportStatus,
		outcome.errMsg,
	)
}

// Top500GoldVODInventoryCoverage is a compact diagnostic summary for corpus ops.
type Top500GoldVODInventoryCoverage struct {
	Total       int `json:"total"`
	Queued      int `json:"queued"`
	Running     int `json:"running"`
	Done        int `json:"done"`
	Failed      int `json:"failed"`
	Unavailable int `json:"unavailable"`
}

func BuildTop500GoldVODInventoryCoverage(ctx context.Context, db *pgxpool.Pool) (Top500GoldVODInventoryCoverage, error) {
	var out Top500GoldVODInventoryCoverage
	if db == nil {
		return out, fmt.Errorf("db unavailable")
	}
	err := db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE gold_status='queued'),
			COUNT(*) FILTER (WHERE gold_status='running'),
			COUNT(*) FILTER (WHERE gold_status='done'),
			COUNT(*) FILTER (WHERE gold_status='failed'),
			COUNT(*) FILTER (WHERE availability_state IN ('expired','deleted','private_or_sub_only','no_chat','region_blocked','gql_forbidden','unknown_unavailable'))
		FROM top500_vod_inventory`).Scan(&out.Total, &out.Queued, &out.Running, &out.Done, &out.Failed, &out.Unavailable)
	if err != nil && err == pgx.ErrNoRows {
		return out, nil
	}
	return out, err
}

func StartTop500GoldVODInventory(ctx context.Context, builder *Top500GoldVODInventory, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if builder == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(builder.interval)
		defer ticker.Stop()
		run := func(trigger string) {
			result, err := builder.RunOnce(ctx)
			if err != nil {
				if log != nil {
					log.Warn("top500 gold vod inventory tick failed", "trigger", trigger, "err", err)
				}
				return
			}
			if log != nil && (result.Upserted > 0 || result.GoldEnqueued > 0) {
				log.Info("top500 gold vod inventory tick completed",
					"trigger", trigger,
					"logins", result.LoginsScanned,
					"vods_seen", result.VODsSeen,
					"upserted", result.Upserted,
					"gold_enqueued", result.GoldEnqueued,
					"skipped", result.Skipped,
				)
			}
		}
		run("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
			}
		}
	}()
}
