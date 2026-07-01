package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultGoldVODSegmentStaleGrace   = 3 * time.Minute
	defaultGoldVODSegmentReclaimEvery = 3 * time.Minute
)

type goldVODSegmentReclaimLog interface {
	Warn(string, ...any)
}

// ReclaimStaleGoldVODSegments fails or dead-letters running segment leases that
// expired without heartbeat renewal.
func ReclaimStaleGoldVODSegments(ctx context.Context, db *pgxpool.Pool, after time.Duration) (failed, deadLetter int, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("db unavailable")
	}
	if after <= 0 {
		after = defaultGoldVODSegmentStaleGrace
	}
	tag, err := db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET status = 'dead_letter',
		    lease_owner = '',
		    lease_expires_at = NULL,
		    heartbeat_at = NULL,
		    error = left(COALESCE(error, '') || ' [stale reclaim exceeded max attempts]', 500),
		    updated_at = now()
		WHERE status = 'running'
		  AND COALESCE(lease_expires_at, '-infinity'::timestamptz) < now() - $1::interval
		  AND attempt >= max_attempts`, after)
	if err != nil {
		return 0, 0, err
	}
	deadLetter = int(tag.RowsAffected())
	tag, err = db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET status = 'failed',
		    next_run_at = now(),
		    lease_owner = '',
		    lease_expires_at = NULL,
		    heartbeat_at = NULL,
		    error = left(COALESCE(error, '') || ' [stale reclaim]', 500),
		    updated_at = now()
		WHERE status = 'running'
		  AND COALESCE(lease_expires_at, '-infinity'::timestamptz) < now() - $1::interval
		  AND attempt < max_attempts`, after)
	if err != nil {
		return 0, deadLetter, err
	}
	failed = int(tag.RowsAffected())
	return failed, deadLetter, nil
}

// StartStaleGoldVODSegmentReclaimer periodically reclaims zombie running segments.
func StartStaleGoldVODSegmentReclaimer(ctx context.Context, db *pgxpool.Pool, after, interval time.Duration, log goldVODSegmentReclaimLog) {
	if db == nil {
		return
	}
	if after <= 0 {
		after = defaultGoldVODSegmentStaleGrace
	}
	if interval <= 0 {
		interval = defaultGoldVODSegmentReclaimEvery
	}
	run := func(trigger string) {
		failed, deadLetter, err := ReclaimStaleGoldVODSegments(ctx, db, after)
		if err != nil && log != nil {
			log.Warn("stale gold vod segment reclaim failed", "trigger", trigger, "err", err)
			return
		}
		if (failed > 0 || deadLetter > 0) && log != nil {
			log.Warn("stale gold vod segments reclaimed", "trigger", trigger, "failed", failed, "dead_letter", deadLetter)
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
