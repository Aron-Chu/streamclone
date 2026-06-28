package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BackfillCleanupOptions struct {
	DryRun            bool          `json:"dryRun"`
	StaleRunningAfter time.Duration `json:"staleRunningAfter"`
}

type BackfillCleanupReport struct {
	DryRun            bool `json:"dryRun"`
	StaleRunning      int  `json:"staleRunning"`
	MissingStreamOpen int  `json:"missingStreamOpen"`
	DuplicateOpenRows int  `json:"duplicateOpenRows"`
	MarkedSkipped     int  `json:"markedSkipped"`
	DuplicatesSkipped int  `json:"duplicatesSkipped"`
	StaleFailed       int  `json:"staleFailed"`
	StaleRequeued     int  `json:"staleRequeued"`
}

func CleanupBackfillQueue(ctx context.Context, db *pgxpool.Pool, opts BackfillCleanupOptions) (BackfillCleanupReport, error) {
	var report BackfillCleanupReport
	report.DryRun = opts.DryRun
	if db == nil {
		return report, fmt.Errorf("db unavailable")
	}
	if opts.StaleRunningAfter <= 0 {
		opts.StaleRunningAfter = defaultBackfillStaleRunningAfter
	}
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM backfill_jobs
		WHERE status='running' AND updated_at < now() - $1::interval`, opts.StaleRunningAfter).Scan(&report.StaleRunning); err != nil {
		return report, err
	}
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM backfill_jobs bj
		LEFT JOIN analytics_streams s ON s.stream_id=bj.stream_id
		WHERE bj.status IN ('queued','running')
		  AND COALESCE(bj.stream_id,'') <> ''
		  AND s.stream_id IS NULL`).Scan(&report.MissingStreamOpen); err != nil {
		return report, err
	}
	if err := db.QueryRow(ctx, `
		WITH ranked AS (
			SELECT id, row_number() OVER (
				PARTITION BY tier, stream_id, login
				ORDER BY
					CASE status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,
					updated_at DESC,
					id ASC
			) AS rn
			FROM backfill_jobs
			WHERE status IN ('queued','running','failed')
		)
		SELECT COUNT(*) FROM ranked WHERE rn > 1`).Scan(&report.DuplicateOpenRows); err != nil {
		return report, err
	}
	if opts.DryRun {
		return report, nil
	}
	failed, requeued, err := ReclaimStaleRunningJobs(ctx, db, opts.StaleRunningAfter)
	if err != nil {
		return report, err
	}
	report.StaleFailed = failed
	report.StaleRequeued = requeued
	tag, err := db.Exec(ctx, `
		UPDATE backfill_jobs bj
		SET status='skipped',
		    export_status='skipped',
		    error=COALESCE(NULLIF(error,''),'') || ' [cleanup missing stream]',
		    updated_at=now()
		WHERE bj.status IN ('queued','running')
		  AND COALESCE(bj.stream_id,'') <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM analytics_streams s WHERE s.stream_id=bj.stream_id
		  )`)
	if err != nil {
		return report, err
	}
	report.MarkedSkipped = int(tag.RowsAffected())
	tag, err = db.Exec(ctx, `
		WITH ranked AS (
			SELECT id, row_number() OVER (
				PARTITION BY tier, stream_id, login
				ORDER BY
					CASE status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,
					updated_at DESC,
					id ASC
			) AS rn
			FROM backfill_jobs
			WHERE status IN ('queued','running','failed')
		)
		UPDATE backfill_jobs bj
		SET status='skipped',
		    export_status='skipped',
		    error=COALESCE(NULLIF(error,''),'') || ' [cleanup duplicate]',
		    updated_at=now()
		FROM ranked
		WHERE bj.id=ranked.id AND ranked.rn > 1`)
	if err != nil {
		return report, err
	}
	report.DuplicatesSkipped = int(tag.RowsAffected())
	return report, nil
}
