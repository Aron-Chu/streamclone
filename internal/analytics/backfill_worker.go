package analytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive/jobtracker"
	"streamclone/internal/metrics"
)

const maxBackfillSyncAttempts = 3

const defaultBackfillStaleRunningAfter = 2 * time.Hour
const defaultBackfillHeartbeatInterval = 60 * time.Second
const defaultStaleReclaimerInterval = 5 * time.Minute

const reclaimRunningOnStartupSQL = `
	UPDATE backfill_jobs
	SET status=CASE WHEN attempt + 1 >= 3 THEN 'failed' ELSE 'queued' END,
	    export_status=CASE WHEN attempt + 1 >= 3 THEN 'failed' ELSE export_status END,
	    next_run_at=CASE WHEN attempt + 1 >= 3 THEN next_run_at ELSE now() END,
	    attempt=attempt + 1,
	    error=COALESCE(error,'') || ' [startup reclaim]', updated_at=now()
	WHERE status='running'`

const backfillPanicRequeueSQL = `
	UPDATE backfill_jobs
	SET status=CASE WHEN attempt + 1 >= 3 THEN 'failed' ELSE 'queued' END,
	    export_status=CASE WHEN attempt + 1 >= 3 THEN 'failed' ELSE export_status END,
	    next_run_at=CASE WHEN attempt + 1 >= 3 THEN next_run_at ELSE now() END,
	    attempt=attempt + 1,
	    error=COALESCE(error,'') || ' [panic reclaim]', updated_at=now()
	WHERE id=$1 AND status='running'`

var backfillStartupReclaimOnce sync.Once

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

// BackfillWorkerOptions configures a tier-scoped backfill worker loop.
// RunOnce is synchronous: each worker goroutine processes at most one job at a time.
type BackfillWorkerOptions struct {
	Name              string
	TierFilter        []string
	StaleRunningAfter time.Duration
	HeartbeatInterval time.Duration
}

// VODChatExporter uploads persisted VOD chat messages to cold storage.
type VODChatExporter interface {
	ExportVODChat(ctx context.Context, streamID string) error
}

// GoldLiteExporter uploads minute chat/emote aggregates from rollups.
type GoldLiteExporter interface {
	ExportGoldLite(ctx context.Context, streamID string, requireRollups bool) error
}

// BackfillWorker processes queued TT gap-fill jobs.
type BackfillWorker struct {
	db                     *pgxpool.Pool
	sync                   *SyncService
	exporter               SyncArchiveExporter
	vodChatExporter        VODChatExporter
	goldLiteExporter       GoldLiteExporter
	goldLiteRequireRollups bool
	interval               time.Duration
	goldSyncTimeout        time.Duration
	jobTracker             *jobtracker.Tracker
	name                   string
	tierFilter             []string
	staleRunningAfter      time.Duration
	heartbeatInterval      time.Duration
}

func NewBackfillWorker(db *pgxpool.Pool, sync *SyncService, exporter SyncArchiveExporter, interval time.Duration) *BackfillWorker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &BackfillWorker{db: db, sync: sync, exporter: exporter, interval: interval}
}

func (w *BackfillWorker) WithWorkerOptions(opts BackfillWorkerOptions) *BackfillWorker {
	if w == nil {
		return w
	}
	if opts.Name != "" {
		w.name = opts.Name
	}
	w.tierFilter = normalizeTierFilter(opts.TierFilter)
	if opts.StaleRunningAfter > 0 {
		w.staleRunningAfter = opts.StaleRunningAfter
	}
	if opts.HeartbeatInterval > 0 {
		w.heartbeatInterval = opts.HeartbeatInterval
	}
	return w
}

func (w *BackfillWorker) WithJobTracker(tracker *jobtracker.Tracker) *BackfillWorker {
	if w != nil {
		w.jobTracker = tracker
	}
	return w
}

func (w *BackfillWorker) WithGoldSyncTimeout(timeout time.Duration) *BackfillWorker {
	if w != nil {
		w.goldSyncTimeout = timeout
	}
	return w
}

func (w *BackfillWorker) WithVODChatExporter(exporter VODChatExporter) *BackfillWorker {
	if w != nil {
		w.vodChatExporter = exporter
	}
	return w
}

func (w *BackfillWorker) WithGoldLiteExporter(exporter GoldLiteExporter, requireRollups bool) *BackfillWorker {
	if w != nil {
		w.goldLiteExporter = exporter
		w.goldLiteRequireRollups = requireRollups
	}
	return w
}

func (w *BackfillWorker) workerName() string {
	if w == nil || w.name == "" {
		return "backfill"
	}
	return w.name
}

func (w *BackfillWorker) heartbeatEvery() time.Duration {
	if w == nil || w.heartbeatInterval <= 0 {
		return defaultBackfillHeartbeatInterval
	}
	return w.heartbeatInterval
}

func backfillSyncParams(tier string) (viewersOnly, forceChat bool) {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "gold", "gold_full":
		return false, true
	case "gold_lite":
		return true, false
	default:
		return true, false
	}
}

func backfillExportLabel(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "gold", "gold_full":
		return "backfill gold-full chat"
	case "gold_lite":
		return "backfill gold-lite aggregates"
	default:
		return "backfill viewers-only"
	}
}

func isGoldFullTier(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "gold", "gold_full":
		return true
	default:
		return false
	}
}

func isGoldWorkerTier(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "gold", "gold_full", "gold_lite":
		return true
	default:
		return false
	}
}

func (w *BackfillWorker) RunOnce(ctx context.Context) error {
	_, err := w.runOnce(ctx)
	return err
}

func (w *BackfillWorker) runOnce(ctx context.Context) (processed bool, err error) {
	if w == nil || w.db == nil || w.sync == nil {
		return false, nil
	}
	job, err := w.claimNext(ctx)
	if err != nil || job == nil {
		return false, err
	}
	processed = true
	jobFinished := false
	defer func() {
		if r := recover(); r != nil {
			metrics.RecordBackfillWorkerPanic(w.workerName())
			if !jobFinished {
				_, _ = w.db.Exec(ctx, backfillPanicRequeueSQL, job.ID)
				metrics.RefreshBackfillJobGauges(ctx, w.db, w.staleRunningAfter)
			}
			err = fmt.Errorf("backfill worker panic: %v", r)
		}
	}()

	stopHeartbeat := w.startJobHeartbeat(ctx, job.ID)
	defer stopHeartbeat()

	finishJob := func(outcome backfillOutcome) error {
		var finishErr error
		if outcome.requeue {
			_, finishErr = w.db.Exec(ctx, `
				UPDATE backfill_jobs
				SET status=$2, export_status=$3, error=NULLIF($4,''), attempt=$5, next_run_at=$6, stream_id=$7, updated_at=now()
				WHERE id=$1`, job.ID, outcome.status, outcome.exportStatus, outcome.errMsg, outcome.attempt, outcome.nextRunAt, job.StreamID)
		} else {
			_, finishErr = w.db.Exec(ctx, `
				UPDATE backfill_jobs
				SET status=$2, export_status=$3, error=NULLIF($4,''), attempt=$5, stream_id=$6, updated_at=now()
				WHERE id=$1`, job.ID, outcome.status, outcome.exportStatus, outcome.errMsg, outcome.attempt, job.StreamID)
		}
		jobFinished = true
		markGoldVODInventoryOutcome(ctx, w.db, *job, outcome)
		metrics.RefreshBackfillJobGauges(ctx, w.db, w.staleRunningAfter)
		w.updateArchiveItem(ctx, job.ID, outcome.status, outcome.errMsg)
		if outcome.status == "done" || outcome.status == "failed" {
			metrics.RecordBackfillJobCompleted(job.Tier, outcome.status)
		}
		return finishErr
	}

	jobStreamID, skipped, canonicalizeErr := w.canonicalizeClaimedBackfillJob(ctx, job)
	if skipped {
		jobFinished = true
		metrics.RefreshBackfillJobGauges(ctx, w.db, w.staleRunningAfter)
		w.updateArchiveItem(ctx, job.ID, "skipped", "[alias worker] duplicate active canonical backfill job exists")
		return true, canonicalizeErr
	}
	if canonicalizeErr != nil {
		now := time.Now()
		outcome := resolveBackfillOutcome(*job, canonicalizeErr, now)
		if strings.EqualFold(job.Tier, "silver") {
			outcome = resolveSilverBackfillOutcome(*job, canonicalizeErr, now)
		} else if isGoldWorkerTier(job.Tier) {
			outcome = resolveGoldBackfillOutcome(*job, canonicalizeErr, now)
		}
		return true, finishJob(outcome)
	}
	if isGoldWorkerTier(job.Tier) {
		if err := NewStore(w.db).EnsureSessionForStream(ctx, jobStreamID); err != nil {
			outcome := resolveGoldBackfillOutcome(*job, fmt.Errorf("ensure session before gold sync: %w", err), time.Now())
			return true, finishJob(outcome)
		}
		var ivrResult GoldIVRAttemptResult
		if w.sync != nil {
			ivrResult = w.sync.TryGoldIVRLiteBeforeGQL(ctx, jobStreamID, job.Login)
		}
		viewersOnly, forceChat := backfillSyncParams(job.Tier)
		syncCtx := ctx
		if isGoldFullTier(job.Tier) && w.goldSyncTimeout > 0 {
			var cancel context.CancelFunc
			syncCtx, cancel = context.WithTimeout(ctx, w.goldSyncTimeout)
			defer cancel()
		}
		syncCtx = WithGoldBackfillJobID(syncCtx, job.ID)
		_, syncErr := w.sync.SyncHistoricalStream(syncCtx, jobStreamID, job.Login, viewersOnly, forceChat, "")
		syncErr = w.applyGoldSegmentCompletionGate(ctx, job, jobStreamID, syncErr)
		if syncErr == nil && ivrResult.ShadowOnly && w.sync != nil {
			if _, recErr := w.sync.ReconcileGoldIVRShadowAfterGQL(ctx, jobStreamID, job.Login, ivrResult); recErr != nil {
				slog.Warn("gold ivr shadow reconciliation failed", "stream_id", jobStreamID, "err", recErr)
			}
		}
		var outcome backfillOutcome
		if syncErr != nil && strings.EqualFold(job.Tier, "silver") {
			outcome = resolveSilverBackfillOutcome(*job, syncErr, time.Now())
		} else if isGoldWorkerTier(job.Tier) {
			outcome = resolveGoldBackfillOutcome(*job, syncErr, time.Now())
		} else {
			outcome = resolveBackfillOutcome(*job, syncErr, time.Now())
		}
		if syncErr == nil {
			job.StreamID = jobStreamID
		}
		if syncErr == nil && w.exporter != nil {
			if err := w.exporter.ExportSync(ctx, jobStreamID, job.Login, backfillExportLabel(job.Tier)); err != nil {
				if isGoldFullTier(job.Tier) {
					outcome = resolveGoldBackfillOutcome(*job, err, time.Now())
					if !outcome.requeue {
						outcome.exportStatus = "failed"
					}
				} else {
					outcome.exportStatus = "failed"
					outcome.errMsg = err.Error()
				}
			} else if strings.EqualFold(job.Tier, "gold_lite") && w.goldLiteExporter != nil {
				if err := w.goldLiteExporter.ExportGoldLite(ctx, jobStreamID, w.goldLiteRequireRollups); err != nil {
					outcome.exportStatus = "failed"
					outcome.errMsg = err.Error()
				}
			} else if isGoldFullTier(job.Tier) && w.vodChatExporter != nil {
				if err := w.vodChatExporter.ExportVODChat(ctx, jobStreamID); err != nil {
					outcome = resolveGoldBackfillOutcome(*job, err, time.Now())
					if !outcome.requeue {
						outcome.exportStatus = "failed"
					}
				}
			}
		}
		return true, finishJob(outcome)
	}
	viewersOnly, forceChat := backfillSyncParams(job.Tier)
	syncCtx := ctx
	if isGoldFullTier(job.Tier) && w.goldSyncTimeout > 0 {
		var cancel context.CancelFunc
		syncCtx, cancel = context.WithTimeout(ctx, w.goldSyncTimeout)
		defer cancel()
	}
	_, syncErr := w.sync.SyncHistoricalStream(syncCtx, jobStreamID, job.Login, viewersOnly, forceChat, "")
	syncErr = w.applyGoldSegmentCompletionGate(ctx, job, jobStreamID, syncErr)
	var outcome backfillOutcome
	if syncErr != nil && strings.EqualFold(job.Tier, "silver") {
		outcome = resolveSilverBackfillOutcome(*job, syncErr, time.Now())
	} else if isGoldWorkerTier(job.Tier) {
		outcome = resolveGoldBackfillOutcome(*job, syncErr, time.Now())
	} else {
		outcome = resolveBackfillOutcome(*job, syncErr, time.Now())
	}
	if syncErr == nil {
		job.StreamID = jobStreamID
	}
	if syncErr == nil && w.exporter != nil {
		if err := w.exporter.ExportSync(ctx, jobStreamID, job.Login, backfillExportLabel(job.Tier)); err != nil {
			if isGoldFullTier(job.Tier) {
				outcome = resolveGoldBackfillOutcome(*job, err, time.Now())
				if !outcome.requeue {
					outcome.exportStatus = "failed"
				}
			} else {
				outcome.exportStatus = "failed"
				outcome.errMsg = err.Error()
			}
		} else if strings.EqualFold(job.Tier, "gold_lite") && w.goldLiteExporter != nil {
			if err := w.goldLiteExporter.ExportGoldLite(ctx, jobStreamID, w.goldLiteRequireRollups); err != nil {
				outcome.exportStatus = "failed"
				outcome.errMsg = err.Error()
			}
		} else if isGoldFullTier(job.Tier) && w.vodChatExporter != nil {
			if err := w.vodChatExporter.ExportVODChat(ctx, jobStreamID); err != nil {
				outcome = resolveGoldBackfillOutcome(*job, err, time.Now())
				if !outcome.requeue {
					outcome.exportStatus = "failed"
				}
			}
		}
	}
	return true, finishJob(outcome)
}

func (w *BackfillWorker) canonicalizeClaimedBackfillJob(ctx context.Context, job *BackfillJob) (string, bool, error) {
	if w == nil || w.db == nil || job == nil {
		return "", false, nil
	}
	jobStreamID := job.StreamID
	canonicalID, err := NewStore(w.db).ResolveCanonicalStreamID(ctx, job.StreamID)
	if err != nil {
		return jobStreamID, false, fmt.Errorf("resolve canonical stream id for backfill job %d: %w", job.ID, err)
	}
	if canonicalID == "" || canonicalID == job.StreamID {
		return jobStreamID, false, nil
	}
	tag, err := w.db.Exec(ctx, `
		UPDATE backfill_jobs
		SET stream_id=$2, updated_at=now()
		WHERE id=$1
		  AND status='running'
		  AND NOT EXISTS (
			SELECT 1 FROM backfill_jobs existing
			WHERE existing.id <> backfill_jobs.id
			  AND existing.stream_id = $2
			  AND existing.status IN ('queued', 'running')
		  )`, job.ID, canonicalID)
	if err != nil {
		return jobStreamID, false, fmt.Errorf("rekey backfill job %d %s -> %s: %w", job.ID, job.StreamID, canonicalID, err)
	}
	if tag.RowsAffected() == 0 {
		_, err = w.db.Exec(ctx, `
			UPDATE backfill_jobs
			SET status='skipped',
			    export_status='skipped',
			    error='[alias worker] duplicate active canonical backfill job exists',
			    updated_at=now()
			WHERE id=$1
			  AND status='running'`, job.ID)
		return jobStreamID, true, err
	}
	job.StreamID = canonicalID
	return canonicalID, false, nil
}

// ReclaimRunningOnStartup requeues every running backfill job once at worker startup.
func ReclaimRunningOnStartup(ctx context.Context, db *pgxpool.Pool) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db unavailable")
	}
	tag, err := db.Exec(ctx, reclaimRunningOnStartupSQL)
	if err != nil {
		return 0, err
	}
	requeued := int(tag.RowsAffected())
	metrics.RefreshBackfillJobGauges(ctx, db)
	return requeued, nil
}

func runBackfillStartupReclaimOnce(ctx context.Context, db *pgxpool.Pool, log backfillWorkerLog) {
	backfillStartupReclaimOnce.Do(func() {
		requeued, err := ReclaimRunningOnStartup(ctx, db)
		if err != nil && log != nil {
			log.Warn("startup backfill reclaim failed", "err", err)
			return
		}
		if requeued > 0 && log != nil {
			log.Warn("startup backfill jobs reclaimed", "requeued", requeued)
		}
	})
}

func (w *BackfillWorker) startJobHeartbeat(ctx context.Context, jobID int64) func() {
	if w == nil || w.db == nil || jobID <= 0 {
		return func() {}
	}
	beatCtx, cancel := context.WithCancel(ctx)
	interval := w.heartbeatEvery()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-beatCtx.Done():
				return
			case <-ticker.C:
				_, _ = w.db.Exec(beatCtx, `
					UPDATE backfill_jobs SET updated_at=now()
					WHERE id=$1 AND status='running'`, jobID)
			}
		}
	}()
	return cancel
}

func (w *BackfillWorker) updateArchiveItem(ctx context.Context, backfillJobID int64, status, errMsg string) {
	if w == nil || w.jobTracker == nil {
		return
	}
	_ = w.jobTracker.UpdateItemFromBackfill(ctx, backfillJobID, status, "", errMsg)
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
		if isSyncTimeoutError(syncErr) {
			return resolveRetriableBackfillOutcome(job, syncErr.Error(), now)
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

func resolveGoldBackfillOutcome(job BackfillJob, syncErr error, now time.Time) backfillOutcome {
	if syncErr == nil {
		return resolveBackfillOutcome(job, nil, now)
	}
	if isRetriableGoldFailure(syncErr.Error()) {
		return resolveRetriableBackfillOutcome(job, syncErr.Error(), now)
	}
	nextAttempt := job.Attempt + 1
	return backfillOutcome{
		status:       "failed",
		exportStatus: "failed",
		errMsg:       syncErr.Error(),
		attempt:      nextAttempt,
	}
}

func resolveRetriableBackfillOutcome(job BackfillJob, errMsg string, now time.Time) backfillOutcome {
	nextAttempt := job.Attempt + 1
	if nextAttempt < maxBackfillSyncAttempts {
		return backfillOutcome{
			status:       "queued",
			exportStatus: job.ExportStatus,
			errMsg:       errMsg,
			attempt:      nextAttempt,
			nextRunAt:    now.Add(backfillRetryDelay(nextAttempt)),
			requeue:      true,
		}
	}
	return backfillOutcome{
		status:       "failed",
		exportStatus: "failed",
		errMsg:       errMsg,
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
	query := ClaimNextSQL(w.tierFilter)
	if IsGoldWorkerTierFilter(w.tierFilter) || len(w.tierFilter) == 0 {
		err = tx.QueryRow(ctx, query).Scan(
			&job.ID, &job.Tier, &job.StreamID, &job.Login, &job.EgressSlot, &job.Attempt,
			&job.ExportStatus, &job.Status, &job.NextRunAt, &job.Error,
		)
	} else {
		err = tx.QueryRow(ctx, query, w.tierFilter).Scan(
			&job.ID, &job.Tier, &job.StreamID, &job.Login, &job.EgressSlot, &job.Attempt,
			&job.ExportStatus, &job.Status, &job.NextRunAt, &job.Error,
		)
	}
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
	if isGoldFullTier(job.Tier) {
		_, _ = tx.Exec(ctx, `
			UPDATE top500_vod_inventory
			SET gold_status='running', gold_backfill_job_id=$2, updated_at=now()
			WHERE stream_id=$1`, job.StreamID, job.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &job, nil
}

// ReclaimStaleRunningJobs fails or requeues running jobs older than after based on attempt count.
func ReclaimStaleRunningJobs(ctx context.Context, db *pgxpool.Pool, after time.Duration) (failed, requeued int, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("db unavailable")
	}
	if after <= 0 {
		after = defaultBackfillStaleRunningAfter
	}
	tag, err := db.Exec(ctx, `
		UPDATE backfill_jobs
		SET status='failed',
		    export_status='failed',
		    error=COALESCE(error,'') || ' [stale reclaim exceeded max attempts]',
		    updated_at=now()
		WHERE status='running' AND updated_at < now() - $1::interval
		  AND attempt >= $2`, after, maxBackfillSyncAttempts)
	if err != nil {
		return 0, 0, err
	}
	failed = int(tag.RowsAffected())
	tag, err = db.Exec(ctx, `
		UPDATE backfill_jobs
		SET status='queued', next_run_at=now(), attempt=attempt+1,
		    error=COALESCE(error,'') || ' [stale reclaim]', updated_at=now()
		WHERE status='running' AND updated_at < now() - $1::interval
		  AND attempt < $2`, after, maxBackfillSyncAttempts)
	if err != nil {
		return failed, 0, err
	}
	requeued = int(tag.RowsAffected())
	metrics.RefreshBackfillJobGauges(ctx, db, after)
	return failed, requeued, nil
}

type backfillWorkerLog interface {
	Warn(string, ...any)
	Error(string, ...any)
}

func StartBackfillWorker(ctx context.Context, worker *BackfillWorker, log backfillWorkerLog) {
	if worker == nil {
		return
	}
	name := worker.workerName()
	go func() {
		runBackfillStartupReclaimOnce(ctx, worker.db, log)
		run := func(trigger string) {
			defer func() {
				if r := recover(); r != nil {
					metrics.RecordBackfillWorkerPanic(name)
					if log != nil {
						log.Error("backfill worker panic", "worker", name, "trigger", trigger, "panic", r)
					}
				}
			}()
			runBackfillWorkerDrain(ctx, name, worker.runOnce, log)
		}
		ticker := time.NewTicker(worker.interval)
		defer ticker.Stop()
		metrics.RefreshBackfillJobGauges(ctx, worker.db, worker.staleRunningAfter)
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

func runBackfillWorkerDrain(ctx context.Context, name string, runOnce func(context.Context) (bool, error), log backfillWorkerLog) {
	if runOnce == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		metrics.RecordBackfillWorkerTick(name)
		processed, err := runOnce(ctx)
		if err != nil {
			if log != nil {
				log.Warn("backfill worker tick failed", "worker", name, "err", err)
			}
			return
		}
		if !processed {
			return
		}
	}
}

// StartStaleBackfillReclaimer periodically reclaims zombie running jobs.
func StartStaleBackfillReclaimer(ctx context.Context, db *pgxpool.Pool, after time.Duration, interval time.Duration, log backfillWorkerLog) {
	if db == nil {
		return
	}
	if after <= 0 {
		after = defaultBackfillStaleRunningAfter
	}
	if interval <= 0 {
		interval = defaultStaleReclaimerInterval
	}
	run := func(label string) {
		failed, requeued, err := ReclaimStaleRunningJobs(ctx, db, after)
		if err != nil && log != nil {
			log.Warn("stale backfill reclaim failed", "trigger", label, "err", err)
			return
		}
		if (failed > 0 || requeued > 0) && log != nil {
			log.Warn("stale backfill jobs reclaimed", "trigger", label, "failed", failed, "requeued", requeued)
		}
	}
	go func() {
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
