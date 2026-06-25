package analytics

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	SilverGatePerChannelCooldown = 24 * time.Hour
	SilverGateFailureBackoff     = 72 * time.Hour
)

// PostgresSilverBudgetCounterReader loads silver budget counters from Postgres and Redis (read-only).
type PostgresSilverBudgetCounterReader struct {
	db               *pgxpool.Pool
	rdb              *redis.Client
	ttBackoffEnabled bool
	now              func() time.Time
}

// NewPostgresSilverBudgetCounterReader wires a read-only hosted counter reader for the silver gate.
func NewPostgresSilverBudgetCounterReader(db *pgxpool.Pool, rdb *redis.Client, ttBackoffEnabled bool) *PostgresSilverBudgetCounterReader {
	return &PostgresSilverBudgetCounterReader{
		db:               db,
		rdb:              rdb,
		ttBackoffEnabled: ttBackoffEnabled,
		now:              time.Now,
	}
}

// ReadSnapshot loads global and per-channel silver counters without mutating queue state.
func (r *PostgresSilverBudgetCounterReader) ReadSnapshot(ctx context.Context, login, streamID string) (SilverBudgetSnapshot, error) {
	unavailable := SilverBudgetSnapshot{Available: false}
	if r == nil || r.db == nil {
		return unavailable, nil
	}
	if r.ttBackoffEnabled && r.rdb == nil {
		return unavailable, nil
	}

	readAt := r.now()
	snap := SilverBudgetSnapshot{Available: true}

	if err := r.readGlobalSilverCounts(ctx, &snap); err != nil {
		return unavailable, nil
	}
	if err := r.readChannelSilverState(ctx, normalizeLogin(login), strings.TrimSpace(streamID), readAt, &snap); err != nil {
		return unavailable, nil
	}
	if r.ttBackoffEnabled {
		active, err := silverGateGlobalTTBackoffActive(ctx, r.rdb)
		if err != nil {
			return unavailable, nil
		}
		snap.GlobalTTBackoffActive = active
	}

	return snap, nil
}

func silverGateGlobalTTBackoffActive(ctx context.Context, rdb *redis.Client) (bool, error) {
	if rdb == nil {
		return false, errors.New("redis unavailable")
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return false, err
	}
	for _, reason := range ttScrapeGlobalBackoffReasons() {
		key := ttBackoffKeyGlobalPrefix + reason
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			return false, err
		}
		if ttl != -2 {
			return true, nil
		}
	}
	return false, nil
}

func (r *PostgresSilverBudgetCounterReader) readGlobalSilverCounts(ctx context.Context, snap *SilverBudgetSnapshot) error {
	const q = `
SELECT
  COALESCE(COUNT(*) FILTER (WHERE status = 'running'), 0)::int,
  COALESCE(COUNT(*) FILTER (WHERE status IN ('queued', 'running')), 0)::int,
  COALESCE(COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')), 0)::int
FROM backfill_jobs
WHERE tier = 'silver'`
	row := r.db.QueryRow(ctx, q)
	if err := row.Scan(&snap.SilverRunningNow, &snap.SilverQueueDepth, &snap.SilverEnqueuedToday); err != nil {
		return err
	}
	return nil
}

func (r *PostgresSilverBudgetCounterReader) readChannelSilverState(ctx context.Context, login, streamID string, readAt time.Time, snap *SilverBudgetSnapshot) error {
	if login == "" {
		return nil
	}

	const lastAttemptQ = `
SELECT MAX(created_at)
FROM backfill_jobs
WHERE tier = 'silver' AND login = $1`
	var lastAttempt *time.Time
	if err := r.db.QueryRow(ctx, lastAttemptQ, login).Scan(&lastAttempt); err != nil {
		return err
	}
	if lastAttempt != nil {
		snap.LastAttemptAt = lastAttempt.UTC()
		if readAt.Sub(*lastAttempt) < SilverGatePerChannelCooldown {
			snap.InChannelCooldown = true
		}
	}

	const lastFailureQ = `
SELECT MAX(updated_at)
FROM backfill_jobs
WHERE tier = 'silver' AND login = $1 AND status = 'failed'`
	var lastFailure *time.Time
	if err := r.db.QueryRow(ctx, lastFailureQ, login).Scan(&lastFailure); err != nil {
		return err
	}
	if lastFailure != nil && readAt.Sub(*lastFailure) < SilverGateFailureBackoff {
		snap.InFailureBackoff = true
	}

	if streamID == "" {
		return nil
	}

	const streamStatusQ = `
SELECT status
FROM backfill_jobs
WHERE tier = 'silver' AND login = $1 AND stream_id = $2
ORDER BY updated_at DESC
LIMIT 1`
	var status string
	err := r.db.QueryRow(ctx, streamStatusQ, login, streamID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch status {
	case "done", "skipped":
		snap.AlreadyDone = true
	case "queued", "running":
		snap.DuplicateQueuedOrRunning = true
	}
	return nil
}
