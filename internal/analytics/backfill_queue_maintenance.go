package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/metrics"
)

// BackfillQueueMaintenanceOptions configures periodic queue hygiene and retries.
type BackfillQueueMaintenanceOptions struct {
	StaleRunningAfter time.Duration
	RequeueFailedMax  int
	RepairSessionsMax int
}

// BackfillQueueMaintenanceReport summarizes one maintenance pass.
type BackfillQueueMaintenanceReport struct {
	Cleanup          BackfillCleanupReport `json:"cleanup"`
	SessionsRepaired int                   `json:"sessionsRepaired"`
	GoldRequeued     int                   `json:"goldRequeued"`
	SilverRequeued   int                   `json:"silverRequeued"`
}

func isRetriableGoldFailure(errMsg string) bool {
	msg := strings.ToLower(strings.TrimSpace(errMsg))
	if msg == "" {
		return false
	}
	permanent := []string{
		"no vod chat rows",
		"no vod chat comments fetched",
		"no_chat_data_in_range",
		"vod was not found",
		"chat unavailable",
		"broadcaster id is missing",
		"vod_unavailable",
		"video not found",
		"not found or no longer available",
		"[cleanup missing stream]",
		"[cleanup duplicate]",
	}
	for _, needle := range permanent {
		if strings.Contains(msg, needle) {
			return false
		}
	}
	retriable := []string{
		"foreign key",
		"violates foreign key",
		"duplicate key",
		"timeout",
		"deadline exceeded",
		"context deadline exceeded",
		"i/o timeout",
		"connection refused",
		"connection reset",
		"server closed idle connection",
		"temporary",
		"temporarily unavailable",
		"too many requests",
		"rate limit",
		"throttle",
		"backoff",
		"cloudflare",
		"browser",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"missing from analytics_streams",
		"ensure session before gold sync",
		"gold vod segments incomplete",
	}
	for _, needle := range retriable {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func isRetriableSilverFailure(errMsg string) bool {
	msg := strings.ToLower(strings.TrimSpace(errMsg))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "backoff") ||
		strings.Contains(msg, "cloudflare") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "browser")
}

// RunBackfillQueueMaintenance reclaims stale jobs, skips poison rows, and requeues retriable failures.
func RunBackfillQueueMaintenance(ctx context.Context, db *pgxpool.Pool, opts BackfillQueueMaintenanceOptions) (BackfillQueueMaintenanceReport, error) {
	var report BackfillQueueMaintenanceReport
	if db == nil {
		return report, fmt.Errorf("db unavailable")
	}
	if opts.StaleRunningAfter <= 0 {
		opts.StaleRunningAfter = defaultBackfillStaleRunningAfter
	}
	if opts.RequeueFailedMax <= 0 {
		opts.RequeueFailedMax = 25
	}
	if opts.RepairSessionsMax <= 0 {
		opts.RepairSessionsMax = 50
	}

	cleanup, err := CleanupBackfillQueue(ctx, db, BackfillCleanupOptions{
		DryRun:            false,
		StaleRunningAfter: opts.StaleRunningAfter,
	})
	if err != nil {
		return report, err
	}
	report.Cleanup = cleanup

	store := NewStore(db)
	rows, err := db.Query(ctx, `
		SELECT DISTINCT stream_id
		FROM backfill_jobs
		WHERE tier IN ('gold','gold_full')
		  AND status='failed'
		  AND (
			error ILIKE '%missing from analytics_streams%'
			OR error ILIKE '%ensure session before gold sync%'
		  )
		LIMIT $1`, opts.RepairSessionsMax)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	for rows.Next() {
		var streamID string
		if err := rows.Scan(&streamID); err != nil {
			return report, err
		}
		if err := store.EnsureSessionForStream(ctx, streamID); err == nil {
			report.SessionsRepaired++
		}
	}
	if err := rows.Err(); err != nil {
		return report, err
	}

	report.GoldRequeued, err = requeueRetriableFailedJobs(ctx, db, []string{"gold", "gold_full"}, opts.RequeueFailedMax, isRetriableGoldFailure)
	if err != nil {
		return report, err
	}
	report.SilverRequeued, err = requeueRetriableFailedJobs(ctx, db, []string{"silver"}, opts.RequeueFailedMax, isRetriableSilverFailure)
	if err != nil {
		return report, err
	}
	metrics.RefreshBackfillJobGauges(ctx, db, opts.StaleRunningAfter)
	return report, nil
}

func requeueRetriableFailedJobs(ctx context.Context, db *pgxpool.Pool, tiers []string, max int, match func(string) bool) (int, error) {
	if db == nil || max <= 0 || len(tiers) == 0 {
		return 0, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id, COALESCE(error,'')
		FROM backfill_jobs
		WHERE tier = ANY($1::text[])
		  AND status='failed'
		  AND attempt < $2
		  AND COALESCE(stream_id,'') <> ''
		  AND EXISTS (SELECT 1 FROM analytics_streams s WHERE s.stream_id = backfill_jobs.stream_id)
		  AND NOT EXISTS (
			SELECT 1 FROM backfill_jobs active
			WHERE active.id <> backfill_jobs.id
			  AND active.stream_id = backfill_jobs.stream_id
			  AND active.status IN ('queued','running')
		  )
		ORDER BY updated_at ASC
		LIMIT $3`, tiers, maxBackfillSyncAttempts, max*3)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	requeued := 0
	for rows.Next() {
		if requeued >= max {
			break
		}
		var id int64
		var errMsg string
		if err := rows.Scan(&id, &errMsg); err != nil {
			return requeued, err
		}
		if !match(errMsg) {
			continue
		}
		tag, err := db.Exec(ctx, `
			UPDATE backfill_jobs
			SET status='queued',
			    export_status='pending',
			    error=NULL,
			    next_run_at=now(),
			    updated_at=now()
			WHERE id=$1 AND status='failed'`, id)
		if err != nil {
			return requeued, err
		}
		if tag.RowsAffected() > 0 {
			requeued++
		}
	}
	return requeued, rows.Err()
}

type backfillMaintainerLog interface {
	Info(string, ...any)
	Warn(string, ...any)
}

// StartBackfillQueueMaintainer runs queue hygiene on an interval while backfill workers are enabled.
func StartBackfillQueueMaintainer(ctx context.Context, db *pgxpool.Pool, interval time.Duration, opts BackfillQueueMaintenanceOptions, log backfillMaintainerLog) {
	if db == nil || interval <= 0 {
		return
	}
	go func() {
		run := func(trigger string) {
			report, err := RunBackfillQueueMaintenance(ctx, db, opts)
			if err != nil {
				if log != nil {
					log.Warn("backfill queue maintenance failed", "trigger", trigger, "err", err)
				}
				return
			}
			if log == nil {
				return
			}
			changed := report.Cleanup.StaleFailed + report.Cleanup.StaleRequeued +
				report.Cleanup.MarkedSkipped + report.Cleanup.DuplicatesSkipped +
				report.SessionsRepaired + report.GoldRequeued + report.SilverRequeued
			if changed > 0 {
				log.Info("backfill queue maintenance completed",
					"trigger", trigger,
					"stale_failed", report.Cleanup.StaleFailed,
					"stale_requeued", report.Cleanup.StaleRequeued,
					"missing_skipped", report.Cleanup.MarkedSkipped,
					"duplicates_skipped", report.Cleanup.DuplicatesSkipped,
					"sessions_repaired", report.SessionsRepaired,
					"gold_requeued", report.GoldRequeued,
					"silver_requeued", report.SilverRequeued,
				)
			}
		}
		run("startup")
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
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
