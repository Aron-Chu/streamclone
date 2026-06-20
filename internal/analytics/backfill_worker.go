package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBackfillSyncAttempts = 3

// BackfillJob is one durable queue row.
type BackfillJob struct {
	ID           int64
	Tier         string
	StreamID     string
	Login        string
	EgressSlot   int
	Attempt      int
	ExportStatus string
	Status       string
	NextRunAt    time.Time
	Error        string
}

// BackfillWorker processes queued TT gap-fill jobs.
type BackfillWorker struct {
	db       *pgxpool.Pool
	sync     *SyncService
	exporter SyncArchiveExporter
	interval time.Duration
}

func NewBackfillWorker(db *pgxpool.Pool, sync *SyncService, exporter SyncArchiveExporter, interval time.Duration) *BackfillWorker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &BackfillWorker{db: db, sync: sync, exporter: exporter, interval: interval}
}

func (w *BackfillWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.db == nil || w.sync == nil {
		return nil
	}
	job, err := w.claimNext(ctx)
	if err != nil || job == nil {
		return err
	}
	_, syncErr := w.sync.SyncHistoricalStream(ctx, job.StreamID, job.Login, true, false, "")
	outcome := resolveBackfillOutcome(*job, syncErr, time.Now())
	if syncErr == nil && w.exporter != nil {
		if err := w.exporter.ExportSync(ctx, job.StreamID, job.Login, "backfill viewers-only"); err != nil {
			outcome.exportStatus = "failed"
			outcome.errMsg = err.Error()
		}
	}
	if outcome.requeue {
		_, err = w.db.Exec(ctx, `
			UPDATE backfill_jobs
			SET status=$2, export_status=$3, error=NULLIF($4,''), attempt=$5, next_run_at=$6, updated_at=now()
			WHERE id=$1`, job.ID, outcome.status, outcome.exportStatus, outcome.errMsg, outcome.attempt, outcome.nextRunAt)
		return err
	}
	_, err = w.db.Exec(ctx, `
		UPDATE backfill_jobs
		SET status=$2, export_status=$3, error=NULLIF($4,''), attempt=$5, updated_at=now()
		WHERE id=$1`, job.ID, outcome.status, outcome.exportStatus, outcome.errMsg, outcome.attempt)
	return err
}

type backfillOutcome struct {
	status       string
	exportStatus string
	errMsg       string
	attempt      int
	nextRunAt    time.Time
	requeue      bool
}

func resolveBackfillOutcome(job BackfillJob, syncErr error, now time.Time) backfillOutcome {
	nextAttempt := job.Attempt + 1
	if syncErr != nil {
		if isSyncTimeoutError(syncErr) && nextAttempt < maxBackfillSyncAttempts {
			return backfillOutcome{
				status:       "queued",
				exportStatus: job.ExportStatus,
				errMsg:       syncErr.Error(),
				attempt:      nextAttempt,
				nextRunAt:    now.Add(backfillRetryDelay(nextAttempt)),
				requeue:      true,
			}
		}
		return backfillOutcome{
			status:       "failed",
			exportStatus: "failed",
			errMsg:       syncErr.Error(),
			attempt:      nextAttempt,
		}
	}
	return backfillOutcome{
		status:       "done",
		exportStatus: "confirmed",
		attempt:      nextAttempt,
	}
}

func isSyncTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "i/o timeout")
}

func backfillRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 60 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > 5*time.Minute {
			return 5 * time.Minute
		}
	}
	return delay
}

func (w *BackfillWorker) claimNext(ctx context.Context) (*BackfillJob, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var job BackfillJob
	err = tx.QueryRow(ctx, `
		SELECT id, tier, stream_id, login, egress_slot, attempt, export_status, status, next_run_at, COALESCE(error,'')
		FROM backfill_jobs
		WHERE status = 'queued' AND next_run_at <= now()
		ORDER BY next_run_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(
		&job.ID, &job.Tier, &job.StreamID, &job.Login, &job.EgressSlot, &job.Attempt,
		&job.ExportStatus, &job.Status, &job.NextRunAt, &job.Error,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE backfill_jobs SET status='running', updated_at=now() WHERE id=$1`, job.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &job, nil
}

func StartBackfillWorker(ctx context.Context, worker *BackfillWorker, log interface {
	Warn(string, ...any)
}) {
	if worker == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(worker.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := worker.RunOnce(ctx); err != nil && log != nil {
					log.Warn("backfill worker tick failed", "err", err)
				}
			}
		}
	}()
}

func ListBackfillJobs(ctx context.Context, db *pgxpool.Pool, limit int) ([]BackfillJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(ctx, `
		SELECT id, tier, stream_id, login, egress_slot, attempt, export_status, status, next_run_at, COALESCE(error,'')
		FROM backfill_jobs
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackfillJob
	for rows.Next() {
		var job BackfillJob
		if err := rows.Scan(
			&job.ID, &job.Tier, &job.StreamID, &job.Login, &job.EgressSlot, &job.Attempt,
			&job.ExportStatus, &job.Status, &job.NextRunAt, &job.Error,
		); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}
