package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationSecondsRoundsUpAndDefaults(t *testing.T) {
	if got := durationSeconds(0, 120); got != 120 {
		t.Fatalf("durationSeconds zero = %d, want fallback", got)
	}
	if got := durationSeconds(1500*time.Millisecond, 120); got != 2 {
		t.Fatalf("durationSeconds rounded = %d, want 2", got)
	}
}

func TestGoldVODSegmentStoreClaimsRespectPerVODLeaseLimit(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-1", "stream-1", "xqc", 1200, 600, "")
	inserted, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 2)
	if err != nil {
		t.Fatalf("upsert segments: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("inserted = %d, want 2", inserted)
	}

	first, err := store.ClaimGoldVODSegment(ctx, "worker-a", time.Minute, 1)
	if err != nil {
		t.Fatalf("claim first: %v", err)
	}
	if first == nil || first.Attempt != 1 || first.LeaseOwner != "worker-a" {
		t.Fatalf("first claim = %+v", first)
	}
	blocked, err := store.ClaimGoldVODSegment(ctx, "worker-b", time.Minute, 1)
	if err != nil {
		t.Fatalf("claim blocked: %v", err)
	}
	if blocked != nil {
		t.Fatalf("second claim with per-VOD limit = %+v, want nil", blocked)
	}

	completed, err := store.CompleteGoldVODSegment(ctx, first.ID, "worker-a", 10, "cursor-a")
	if err != nil || !completed {
		t.Fatalf("complete = %v err=%v", completed, err)
	}
	second, err := store.ClaimGoldVODSegment(ctx, "worker-b", time.Minute, 1)
	if err != nil {
		t.Fatalf("claim second: %v", err)
	}
	if second == nil || second.StartOffsetSeconds != 600 {
		t.Fatalf("second claim = %+v, want trailing segment", second)
	}
}

func TestGoldVODSegmentStoreMovesToDeadLetterAfterRetries(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-2", "stream-2", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 1); err != nil {
		t.Fatalf("upsert segments: %v", err)
	}
	claim, err := store.ClaimGoldVODSegment(ctx, "worker-a", time.Minute, 1)
	if err != nil || claim == nil {
		t.Fatalf("claim = %+v err=%v", claim, err)
	}
	failed, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-a", "rate limited", time.Minute)
	if err != nil || !failed {
		t.Fatalf("fail = %v err=%v", failed, err)
	}
	summary, err := store.CorpusGoldSegmentSummary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.DeadLetter != 1 {
		t.Fatalf("summary = %+v, want one dead-letter segment", summary)
	}
}

func TestGoldVODSegmentStoreClaimByKeyAndComplete(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-3", "stream-3", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	key := plans[0].SegmentKey
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, key, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim by key = %+v err=%v", claim, err)
	}
	done, err := store.CompleteGoldVODSegment(ctx, claim.ID, "worker-a", 5, "350")
	if err != nil || !done {
		t.Fatalf("complete = %v err=%v", done, err)
	}
	status, found, err := store.GoldVODSegmentStatusByKey(ctx, key)
	if err != nil || !found || status != "done" {
		t.Fatalf("status = %q found=%v err=%v", status, found, err)
	}
}

func TestGoldVODSegmentStoreClaimByKeyReclaimsExpiredLease(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-4", "stream-4", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	key := plans[0].SegmentKey
	first, err := store.ClaimGoldVODSegmentByKey(ctx, key, "worker-a", time.Minute, 2)
	if err != nil || first == nil {
		t.Fatalf("first claim = %+v err=%v", first, err)
	}
	if _, err := store.db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET lease_expires_at = now() - interval '1 minute'
		WHERE id = $1`, first.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	second, err := store.ClaimGoldVODSegmentByKey(ctx, key, "worker-b", time.Minute, 2)
	if err != nil || second == nil || second.LeaseOwner != "worker-b" {
		t.Fatalf("reclaim = %+v err=%v", second, err)
	}
}

func TestCorpusGoldSegmentSummaryCountsRateLimitErrors(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-5", "stream-5", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim = %+v err=%v", claim, err)
	}
	if _, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-a", "gql video comments status 429 after 5 retries", time.Minute); err != nil {
		t.Fatalf("fail: %v", err)
	}
	summary, err := store.CorpusGoldSegmentSummary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.RateLimitedBuckets != 1 {
		t.Fatalf("rate limited buckets = %d, want 1", summary.RateLimitedBuckets)
	}
}

func applyGoldVODSegmentMigration(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	migrationPath := filepath.Join("..", "..", "migrations", "000049_gold_vod_segments.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read gold segment migration: %v", err)
	}
	if _, err := store.db.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply gold segment migration: %v", err)
	}
}
