package analytics

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	publicEmoteCorpusAll               = "__all__"
	publicEmoteMinimumTrackedMinutes   = int64(300)
	publicEmoteMinimumCoveragePct      = 60.0
	publicEmoteMinimumConfidencePct    = 60.0
	publicEmoteMinimumTotalUses        = int64(100)
	publicEmoteProviderFreshnessMax    = 2 * time.Hour
	publicEmoteProviderStaleMax        = 72 * time.Hour
	publicEmoteProviderRefreshLookback = 24 * time.Hour
	publicEmoteProviderRefreshJob      = "public_emote_provider_refresh"
	publicEmoteProviderRefreshLock     = int64(73002003)
)

var (
	errPublicEmoteProviderStoreUnavailable = errors.New("public emote provider store unavailable")
	errPublicEmoteProviderRefreshRunning   = errors.New("public emote provider refresh already running")
)

type PublicEmoteProviderMaterializationStats struct {
	RangeStart   time.Time
	RangeEnd     time.Time
	RowsUpserted int64
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	Status       string
	ErrorCode    string
	RunID        int64
}

type PublicEmoteMaterializationRun struct {
	RunID         int64
	JobName       string
	SchemaVersion string
	RangeValue    string
	Status        string
	RangeStart    time.Time
	RangeEnd      time.Time
	StartedAt     time.Time
	FinishedAt    time.Time
	RowsUpserted  int64
	ErrorCode     string
	UpdatedAt     time.Time
}

const publicEmoteProviderDeleteSQL = `
	DELETE FROM public_emote_provider_hourly_rollups
	WHERE corpus_key = $3
	  AND bucket_hour >= $1
	  AND bucket_hour < $2`

const publicEmoteProviderAdvisoryLockSQL = `SELECT pg_try_advisory_xact_lock($1)`

const publicEmoteProviderMaterializeSQL = `
	WITH tracked AS (
		SELECT date_trunc('hour', minute_ts) AS bucket_hour,
		       COUNT(*)::bigint AS tracked_minutes
		FROM analytics_minute_rollups
		WHERE minute_ts >= $1 AND minute_ts < $2
		GROUP BY 1
	),
	provider_usage AS (
		SELECT date_trunc('hour', minute_ts) AS bucket_hour,
		       lower(provider) AS provider,
		       COALESCE(SUM(use_count), 0)::bigint AS total_uses,
		       COUNT(DISTINCT (stream_id, minute_ts))::bigint AS emote_minutes,
		       CASE WHEN COALESCE(SUM(use_count), 0) > 0
		            THEN LEAST(100, GREATEST(0, SUM((confidence::double precision * 100) * use_count)::double precision / SUM(use_count)))
		            ELSE 0 END AS confidence
		FROM emote_usage_minute_rollups
		WHERE minute_ts >= $1 AND minute_ts < $2
		  AND use_count > 0
		  AND COALESCE(provider, '') <> ''
		GROUP BY 1, 2
	),
	corpus_usage AS (
		SELECT date_trunc('hour', minute_ts) AS bucket_hour,
		       COALESCE(SUM(use_count), 0)::bigint AS total_uses,
		       COUNT(DISTINCT (stream_id, minute_ts))::bigint AS emote_minutes,
		       CASE WHEN COALESCE(SUM(use_count), 0) > 0
		            THEN LEAST(100, GREATEST(0, SUM((confidence::double precision * 100) * use_count)::double precision / SUM(use_count)))
		            ELSE 0 END AS confidence
		FROM emote_usage_minute_rollups
		WHERE minute_ts >= $1 AND minute_ts < $2
		  AND use_count > 0
		  AND COALESCE(provider, '') <> ''
		GROUP BY 1
	)
	INSERT INTO public_emote_provider_hourly_rollups (
		bucket_hour, corpus_key, provider, total_uses, tracked_minutes, emote_minutes, coverage_pct, confidence, generated_at
	)
	SELECT p.bucket_hour,
	       $3,
	       p.provider,
	       p.total_uses,
	       t.tracked_minutes,
	       p.emote_minutes,
	       CASE WHEN t.tracked_minutes > 0 THEN LEAST(100, GREATEST(0, (p.emote_minutes::double precision / t.tracked_minutes::double precision) * 100)) ELSE 0 END,
	       p.confidence,
	       now()
	FROM provider_usage p
	JOIN tracked t ON t.bucket_hour = p.bucket_hour
	UNION ALL
	SELECT t.bucket_hour,
	       $3,
	       $3,
	       COALESCE(c.total_uses, 0),
	       t.tracked_minutes,
	       COALESCE(c.emote_minutes, 0),
	       CASE WHEN t.tracked_minutes > 0 THEN LEAST(100, GREATEST(0, (COALESCE(c.emote_minutes, 0)::double precision / t.tracked_minutes::double precision) * 100)) ELSE 0 END,
	       COALESCE(c.confidence, 0),
	       now()
	FROM tracked t
	LEFT JOIN corpus_usage c ON c.bucket_hour = t.bucket_hour
	ON CONFLICT (bucket_hour, corpus_key, provider) DO UPDATE SET
		total_uses = EXCLUDED.total_uses,
		tracked_minutes = EXCLUDED.tracked_minutes,
		emote_minutes = EXCLUDED.emote_minutes,
		coverage_pct = EXCLUDED.coverage_pct,
		confidence = EXCLUDED.confidence,
		generated_at = EXCLUDED.generated_at`

const publicEmoteProviderCorpusQuery = `
	SELECT COALESCE(SUM(total_uses), 0)::bigint,
	       COALESCE(SUM(tracked_minutes), 0)::bigint,
	       COALESCE(SUM(emote_minutes), 0)::bigint,
	       CASE WHEN COALESCE(SUM(tracked_minutes), 0) > 0
	            THEN LEAST(100, GREATEST(0, (COALESCE(SUM(emote_minutes), 0)::double precision / SUM(tracked_minutes)::double precision) * 100))
	            ELSE 0 END,
	       CASE WHEN COALESCE(SUM(total_uses), 0) > 0
	            THEN LEAST(100, GREATEST(0, SUM(confidence * total_uses)::double precision / SUM(total_uses)))
	            ELSE 0 END,
	       COALESCE(MAX(generated_at), to_timestamp(0))
	FROM public_emote_provider_hourly_rollups
	WHERE corpus_key = $3
	  AND provider = $3
	  AND bucket_hour >= $1
	  AND bucket_hour < $2`

const publicEmoteProviderRowsQuery = `
	WITH provider_rows AS (
		SELECT provider,
		       COALESCE(SUM(total_uses), 0)::bigint AS total_uses,
		       COALESCE(SUM(emote_minutes), 0)::bigint AS emote_minutes,
		       CASE WHEN COALESCE(SUM(total_uses), 0) > 0
		            THEN LEAST(100, GREATEST(0, SUM(confidence * total_uses)::double precision / SUM(total_uses)))
		            ELSE 0 END AS confidence
		FROM public_emote_provider_hourly_rollups
		WHERE corpus_key = $3
		  AND provider <> $3
		  AND bucket_hour >= $1
		  AND bucket_hour < $2
		GROUP BY provider
	),
	corpus AS (
		SELECT COALESCE(SUM(total_uses), 0)::bigint AS total_uses,
		       COALESCE(SUM(tracked_minutes), 0)::bigint AS tracked_minutes
		FROM public_emote_provider_hourly_rollups
		WHERE corpus_key = $3
		  AND provider = $3
		  AND bucket_hour >= $1
		  AND bucket_hour < $2
	)
	SELECT p.provider,
	       p.total_uses,
	       CASE WHEN c.total_uses > 0 THEN LEAST(100, GREATEST(0, (p.total_uses::double precision / c.total_uses::double precision) * 100)) ELSE 0 END,
	       c.tracked_minutes,
	       CASE WHEN c.tracked_minutes > 0 THEN LEAST(100, GREATEST(0, (p.emote_minutes::double precision / c.tracked_minutes::double precision) * 100)) ELSE 0 END,
	       p.confidence
	FROM provider_rows p
	CROSS JOIN corpus c
	WHERE p.total_uses > 0
	ORDER BY p.total_uses DESC, p.confidence DESC, p.provider ASC`

const publicEmoteProviderLatestRunQuery = `
	SELECT run_id, job_name, schema_version, range_value, status, range_start, range_end,
	       started_at, COALESCE(finished_at, to_timestamp(0)), rows_upserted, COALESCE(error_code, ''), updated_at
	FROM public_emote_materialization_runs
	WHERE job_name = $1
	  AND schema_version = $2
	ORDER BY updated_at DESC, run_id DESC
	LIMIT 1`

const publicEmoteProviderLatestSuccessQuery = `
	SELECT COALESCE(finished_at, updated_at)
	FROM public_emote_materialization_runs
	WHERE job_name = $1
	  AND schema_version = $2
	  AND status = 'success'
	ORDER BY updated_at DESC, run_id DESC
	LIMIT 1`

type PublicEmoteProviderLandscape struct {
	Rows           []PublicProviderPreview
	TotalUses      int64
	TrackedMinutes int64
	EmoteMinutes   int64
	CoveragePct    float64
	Confidence     float64
	GeneratedAt    time.Time
	StalenessSec   int64
	LastRun        PublicEmoteMaterializationRun
	LastSuccessAt  time.Time
}

func publicEmotesRangeDuration(rangeValue string) time.Duration {
	switch parsePublicEmotesRange(rangeValue) {
	case "24h":
		return 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

func publicEmoteMaterializationWindow(now time.Time, rangeValue string) (time.Time, time.Time) {
	end := now.UTC().Truncate(time.Hour).Add(time.Hour)
	start := now.UTC().Add(-publicEmotesRangeDuration(rangeValue)).Truncate(time.Hour)
	if !start.Before(end) {
		start = end.Add(-time.Hour)
	}
	return start, end
}

func publicEmoteProviderRefreshWindow(now time.Time) (time.Time, time.Time) {
	end := now.UTC().Truncate(time.Hour).Add(time.Hour)
	return end.Add(-publicEmoteProviderRefreshLookback), end
}

func (s *Store) MaterializePublicEmoteProviderHourlyRollups(ctx context.Context, rangeValue string, now time.Time) error {
	_, err := s.materializePublicEmoteProviderHourlyRollups(ctx, rangeValue, now)
	return err
}

func (s *Store) materializePublicEmoteProviderHourlyRollups(ctx context.Context, rangeValue string, now time.Time) (PublicEmoteProviderMaterializationStats, error) {
	start, end := publicEmoteMaterializationWindow(now, rangeValue)
	return s.materializePublicEmoteProviderWindow(ctx, start, end, now)
}

func (s *Store) materializePublicEmoteProviderWindow(ctx context.Context, start, end, now time.Time) (PublicEmoteProviderMaterializationStats, error) {
	stats := PublicEmoteProviderMaterializationStats{StartedAt: now.UTC(), Status: "running"}
	if s == nil || s.db == nil {
		return stats, errPublicEmoteProviderStoreUnavailable
	}
	stats.RangeStart = start
	stats.RangeEnd = end
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err := tx.QueryRow(ctx, publicEmoteProviderAdvisoryLockSQL, publicEmoteProviderRefreshLock).Scan(&locked); err != nil {
		return stats, err
	}
	if !locked {
		stats.Status = "skipped"
		stats.ErrorCode = "refresh_already_running"
		return stats, errPublicEmoteProviderRefreshRunning
	}
	if _, err := tx.Exec(ctx, publicEmoteProviderDeleteSQL, start, end, publicEmoteCorpusAll); err != nil {
		return stats, err
	}
	tag, err := tx.Exec(ctx, publicEmoteProviderMaterializeSQL, start, end, publicEmoteCorpusAll)
	if err != nil {
		return stats, err
	}
	stats.RowsUpserted = tag.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return stats, err
	}
	stats.FinishedAt = time.Now().UTC()
	stats.Duration = stats.FinishedAt.Sub(stats.StartedAt)
	stats.Status = "success"
	return stats, nil
}

func (s *Store) RefreshPublicEmoteProviderMaterialization(ctx context.Context, rangeValue string, now time.Time) (PublicEmoteProviderMaterializationStats, error) {
	rangeValue = parsePublicEmotesRange(rangeValue)
	stats := PublicEmoteProviderMaterializationStats{StartedAt: now.UTC(), Status: "running"}
	if s == nil || s.db == nil {
		return stats, errPublicEmoteProviderStoreUnavailable
	}
	start, end := publicEmoteProviderRefreshWindow(now)
	stats.RangeStart = start
	stats.RangeEnd = end
	runID, err := s.insertPublicEmoteMaterializationRun(ctx, rangeValue, stats)
	if err != nil {
		return stats, err
	}
	stats.RunID = runID
	materialized, err := s.materializePublicEmoteProviderWindow(ctx, start, end, now)
	stats.RowsUpserted = materialized.RowsUpserted
	stats.FinishedAt = time.Now().UTC()
	stats.Duration = stats.FinishedAt.Sub(stats.StartedAt)
	if err != nil {
		stats.Status = "failed"
		if errors.Is(err, errPublicEmoteProviderRefreshRunning) {
			stats.Status = "skipped"
		}
		stats.ErrorCode = publicEmoteProviderErrorCode(err)
		_ = s.finishPublicEmoteMaterializationRun(ctx, runID, stats)
		return stats, err
	}
	stats.Status = "success"
	if err := s.finishPublicEmoteMaterializationRun(ctx, runID, stats); err != nil {
		return stats, err
	}
	return stats, nil
}

func publicEmoteProviderErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errPublicEmoteProviderStoreUnavailable) {
		return "store_unavailable"
	}
	if errors.Is(err, errPublicEmoteProviderRefreshRunning) {
		return "refresh_already_running"
	}
	code := strings.ToLower(strings.TrimSpace(err.Error()))
	code = strings.NewReplacer(" ", "_", ":", "", "\n", "_", "\t", "_").Replace(code)
	if code == "" {
		return "unknown"
	}
	if len(code) > 120 {
		code = code[:120]
	}
	return code
}

func (s *Store) insertPublicEmoteMaterializationRun(ctx context.Context, rangeValue string, stats PublicEmoteProviderMaterializationStats) (int64, error) {
	var runID int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO public_emote_materialization_runs (
			job_name, schema_version, range_value, status, range_start, range_end, started_at, rows_upserted, updated_at
		) VALUES ($1, $2, $3, 'running', $4, $5, $6, 0, now())
		RETURNING run_id`, publicEmoteProviderRefreshJob, publicEmotesOverviewSchemaVersion, rangeValue, stats.RangeStart, stats.RangeEnd, stats.StartedAt).Scan(&runID)
	return runID, err
}

func (s *Store) finishPublicEmoteMaterializationRun(ctx context.Context, runID int64, stats PublicEmoteProviderMaterializationStats) error {
	_, err := s.db.Exec(ctx, `
		UPDATE public_emote_materialization_runs
		SET status = $2,
		    finished_at = $3,
		    rows_upserted = $4,
		    error_code = NULLIF($5, ''),
		    updated_at = now()
		WHERE run_id = $1`, runID, stats.Status, stats.FinishedAt, stats.RowsUpserted, stats.ErrorCode)
	return err
}

func (s *Store) PublicEmoteProviderLandscape(ctx context.Context, rangeValue string, now time.Time) (PublicEmoteProviderLandscape, error) {
	var out PublicEmoteProviderLandscape
	if s == nil || s.db == nil {
		return out, errPublicEmoteProviderStoreUnavailable
	}
	start, end := publicEmoteMaterializationWindow(now, rangeValue)
	if err := s.db.QueryRow(ctx, publicEmoteProviderCorpusQuery, start, end, publicEmoteCorpusAll).Scan(
		&out.TotalUses,
		&out.TrackedMinutes,
		&out.EmoteMinutes,
		&out.CoveragePct,
		&out.Confidence,
		&out.GeneratedAt,
	); err != nil {
		return out, err
	}
	if !out.GeneratedAt.IsZero() {
		out.StalenessSec = int64(now.UTC().Sub(out.GeneratedAt).Seconds())
		if out.StalenessSec < 0 {
			out.StalenessSec = 0
		}
	}
	if run, err := s.latestPublicEmoteMaterializationRun(ctx, parsePublicEmotesRange(rangeValue)); err == nil {
		out.LastRun = run
	}
	if lastSuccessAt, err := s.latestPublicEmoteProviderSuccessAt(ctx, parsePublicEmotesRange(rangeValue)); err == nil && !lastSuccessAt.IsZero() {
		out.LastSuccessAt = lastSuccessAt
		out.GeneratedAt = lastSuccessAt
		out.StalenessSec = int64(now.UTC().Sub(lastSuccessAt).Seconds())
		if out.StalenessSec < 0 {
			out.StalenessSec = 0
		}
	}
	rows, err := s.db.Query(ctx, publicEmoteProviderRowsQuery, start, end, publicEmoteCorpusAll)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var row PublicProviderPreview
		if err := rows.Scan(&row.Provider, &row.TotalUses, &row.SharePct, &row.TrackedMinutes, &row.CoveragePct, &row.Confidence); err != nil {
			return out, err
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}

func (s *Store) latestPublicEmoteMaterializationRun(ctx context.Context, rangeValue string) (PublicEmoteMaterializationRun, error) {
	var run PublicEmoteMaterializationRun
	if s == nil || s.db == nil {
		return run, errPublicEmoteProviderStoreUnavailable
	}
	err := s.db.QueryRow(ctx, publicEmoteProviderLatestRunQuery, publicEmoteProviderRefreshJob, publicEmotesOverviewSchemaVersion).Scan(
		&run.RunID,
		&run.JobName,
		&run.SchemaVersion,
		&run.RangeValue,
		&run.Status,
		&run.RangeStart,
		&run.RangeEnd,
		&run.StartedAt,
		&run.FinishedAt,
		&run.RowsUpserted,
		&run.ErrorCode,
		&run.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, nil
	}
	return run, err
}

func (s *Store) latestPublicEmoteProviderSuccessAt(ctx context.Context, rangeValue string) (time.Time, error) {
	var finishedAt time.Time
	if s == nil || s.db == nil {
		return finishedAt, errPublicEmoteProviderStoreUnavailable
	}
	err := s.db.QueryRow(ctx, publicEmoteProviderLatestSuccessQuery, publicEmoteProviderRefreshJob, publicEmotesOverviewSchemaVersion).Scan(&finishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return finishedAt, err
}
