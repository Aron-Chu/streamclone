package analytics

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
)

func TestPostgresSilverBudgetCounterReaderNilDBFailClosed(t *testing.T) {
	var reader *PostgresSilverBudgetCounterReader
	snap, err := reader.ReadSnapshot(context.Background(), "shroud", "stream-1")
	if err != nil {
		t.Fatalf("ReadSnapshot err = %v", err)
	}
	if snap.Available {
		t.Fatal("nil reader must fail closed")
	}
}

func TestPostgresSilverBudgetCounterReaderRedisRequiredWhenBackoffEnabled(t *testing.T) {
	reader := NewPostgresSilverBudgetCounterReader(nil, nil, true)
	snap, err := reader.ReadSnapshot(context.Background(), "shroud", "stream-1")
	if err != nil || snap.Available {
		t.Fatalf("snap=%+v err=%v, want unavailable without db/redis", snap, err)
	}
}

func TestPostgresSilverBudgetCounterReaderRedisPingFailClosed(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 for redis integration test")
	}
	opt, err := redis.ParseURL("redis://localhost:59999/0")
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, pool := setupSilverGateBudgetStore(t)
	reader := NewPostgresSilverBudgetCounterReader(pool, rdb, true)
	snap, err := reader.ReadSnapshot(ctx, "freshuser", "fresh-stream")
	if err != nil || snap.Available {
		t.Fatalf("snap=%+v err=%v, want unavailable when redis ping fails", snap, err)
	}
}

func TestEvaluateSilverGateWithRealCounterSnapshotUnavailable(t *testing.T) {
	candidate := SilverGateCandidate{
		Login:      "shroud",
		ChannelID:  "141981764",
		StreamID:   "123",
		SampledAt:  time.Now().UTC(),
		StaleAfter: time.Now().UTC().Add(15 * time.Minute),
	}
	result := EvaluateSilverGate(candidate, SilverBudgetSnapshot{Available: false}, SilverGateConfig{})
	if result.Decision != SilverGateSkipCounterUnavailable {
		t.Fatalf("decision = %q, want skip_counter_unavailable", result.Decision)
	}
}

func TestEvaluateSilverGateWithRealCounterSnapshotStale(t *testing.T) {
	candidate := SilverGateCandidate{
		Login:      "shroud",
		ChannelID:  "141981764",
		StreamID:   "123",
		SampledAt:  time.Now().UTC(),
		StaleAfter: time.Now().UTC().Add(15 * time.Minute),
	}
	result := EvaluateSilverGate(candidate, SilverBudgetSnapshot{Available: true, Stale: true}, SilverGateConfig{})
	if result.Decision != SilverGateSkipCounterStale {
		t.Fatalf("decision = %q, want skip_counter_stale", result.Decision)
	}
}

func TestPostgresSilverBudgetCounterReaderIntegration(t *testing.T) {
	ctx, pool := setupSilverGateBudgetStore(t)
	reader := NewPostgresSilverBudgetCounterReader(pool, nil, false)

	if _, err := pool.Exec(ctx, `
INSERT INTO backfill_jobs (tier, stream_id, login, status, created_at, updated_at)
VALUES
  ('silver', 'running-stream', 'runner', 'running', now(), now()),
  ('silver', 'queued-stream', 'queued', 'queued', now(), now()),
  ('silver', 'done-stream', 'doneuser', 'done', now() - interval '2 days', now() - interval '2 days'),
  ('silver', 'dup-stream', 'dupuser', 'queued', now(), now()),
  ('gold', 'gold-stream', 'golduser', 'queued', now(), now())`); err != nil {
		t.Fatalf("seed jobs: %v", err)
	}

	snap, err := reader.ReadSnapshot(ctx, "freshuser", "fresh-stream")
	if err != nil {
		t.Fatalf("ReadSnapshot err = %v", err)
	}
	if !snap.Available {
		t.Fatal("expected available snapshot")
	}
	if snap.SilverRunningNow != 1 {
		t.Fatalf("SilverRunningNow = %d, want 1", snap.SilverRunningNow)
	}
	if snap.SilverQueueDepth != 2 {
		t.Fatalf("SilverQueueDepth = %d, want 2 (running+queued silver)", snap.SilverQueueDepth)
	}

	dupSnap, err := reader.ReadSnapshot(ctx, "dupuser", "dup-stream")
	if err != nil || !dupSnap.DuplicateQueuedOrRunning {
		t.Fatalf("dup snap = %+v err=%v", dupSnap, err)
	}

	doneSnap, err := reader.ReadSnapshot(ctx, "doneuser", "done-stream")
	if err != nil || !doneSnap.AlreadyDone {
		t.Fatalf("done snap = %+v err=%v", doneSnap, err)
	}

	result := EvaluateSilverGate(SilverGateCandidate{
		Login:      "freshuser",
		ChannelID:  "100",
		StreamID:   "fresh-stream",
		SampledAt:  time.Now().UTC(),
		StaleAfter: time.Now().UTC().Add(15 * time.Minute),
	}, snap, SilverGateConfig{})
	if !result.AllowEnqueue {
		t.Fatalf("expected allow_enqueue with healthy counters, got %q", result.Decision)
	}
}

func TestPostgresSilverBudgetCounterReaderUsesLoginColumn(t *testing.T) {
	ctx, pool := setupSilverGateBudgetStore(t)
	reader := NewPostgresSilverBudgetCounterReader(pool, nil, false)

	if _, err := pool.Exec(ctx, `
INSERT INTO backfill_jobs (tier, stream_id, login, status, created_at, updated_at)
VALUES ('silver', 'login-col-stream', 'Summit1g', 'queued', now(), now())`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	snap, err := reader.ReadSnapshot(ctx, "summit1g", "login-col-stream")
	if err != nil || !snap.Available {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
	if !snap.DuplicateQueuedOrRunning {
		t.Fatal("expected duplicate when login column matches normalized login and stream_id matches")
	}

	other, err := reader.ReadSnapshot(ctx, "summit1g", "other-stream")
	if err != nil || !other.Available || other.DuplicateQueuedOrRunning {
		t.Fatalf("other-stream snap=%+v err=%v, want no duplicate for different stream", other, err)
	}
}

func TestPostgresSilverBudgetCounterReaderGlobalBackoffFromRedis(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 for redis integration test")
	}
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/15"
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	rdb := redis.NewClient(opt)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	key := ttBackoffKeyGlobalPrefix + TTScrapeReasonCloudflareChallenge
	if err := rdb.Set(ctx, key, "1", time.Minute).Err(); err != nil {
		t.Fatalf("set backoff key: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key) })

	_, pool := setupSilverGateBudgetStore(t)
	reader := NewPostgresSilverBudgetCounterReader(pool, rdb, true)
	snap, err := reader.ReadSnapshot(ctx, "freshuser", "fresh-stream")
	if err != nil || !snap.Available {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
	if !snap.GlobalTTBackoffActive {
		t.Fatal("expected global TT backoff active")
	}
	result := EvaluateSilverGate(SilverGateCandidate{
		Login: "freshuser", ChannelID: "1", StreamID: "fresh-stream",
		SampledAt: time.Now().UTC(), StaleAfter: time.Now().UTC().Add(time.Hour),
	}, snap, SilverGateConfig{})
	if result.Decision != SilverGateSkipGlobalBackoff {
		t.Fatalf("decision = %q, want skip_global_backoff", result.Decision)
	}
}

func TestNewTop500SilverGateFromAppUsesRealCounterReaderWhenStorePresent(t *testing.T) {
	ctx, pool := setupSilverGateBudgetStore(t)
	store := NewStore(pool)
	gate := NewTop500SilverGateFromApp(configWithSilverGateEnabled(), nil, store, nil)
	if gate == nil || !gate.UsesRealCounterReader() {
		t.Fatal("expected real counter reader when store is wired")
	}
	summary, err := gate.RunTick(ctx)
	if err != nil {
		t.Fatalf("RunTick err = %v", err)
	}
	if summary.CandidatesEvaluated > 0 {
		// store has no top500 rows — listed 0 is fine
	}
}

func configWithSilverGateEnabled() config.Config {
	return config.Config{
		Top500SilverGateEnabled:      true,
		Top500SilverGateDryRun:       true,
		Top500SilverGateWriteEnabled: false,
	}
}

func setupSilverGateBudgetStore(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run silver gate budget reader integration tests")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:test@localhost:15432/emotes?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pgxpool.New: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("silver_gate_budget_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("test pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `
CREATE TABLE backfill_jobs (
    id              BIGSERIAL PRIMARY KEY,
    tier            TEXT NOT NULL DEFAULT 'silver',
    stream_id       TEXT NOT NULL,
    login           TEXT NOT NULL,
    egress_slot     INT NOT NULL DEFAULT 0,
    attempt         INT NOT NULL DEFAULT 0,
    export_status   TEXT NOT NULL DEFAULT 'pending',
    status          TEXT NOT NULL DEFAULT 'queued',
    next_run_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatalf("create backfill_jobs: %v", err)
	}
	return ctx, pool
}
