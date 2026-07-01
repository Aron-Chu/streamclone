package analytics

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGoldVODSegmentUnresolvedSummaryBlocksCompletion(t *testing.T) {
	cases := []struct {
		name  string
		sum   GoldVODSegmentUnresolvedSummary
		block bool
	}{
		{"empty", GoldVODSegmentUnresolvedSummary{}, false},
		{"all done", GoldVODSegmentUnresolvedSummary{}, false},
		{"queued", GoldVODSegmentUnresolvedSummary{Queued: 1}, true},
		{"running", GoldVODSegmentUnresolvedSummary{Running: 2}, true},
		{"failed", GoldVODSegmentUnresolvedSummary{Failed: 1}, true},
		{"dead letter", GoldVODSegmentUnresolvedSummary{DeadLetter: 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sum.BlocksCompletion(); got != tc.block {
				t.Fatalf("BlocksCompletion() = %v, want %v", got, tc.block)
			}
		})
	}
}

func TestGoldVODSegmentsIncompleteErrorNilWhenComplete(t *testing.T) {
	if err := goldVODSegmentsIncompleteError(GoldVODSegmentUnresolvedSummary{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestGoldVODSegmentsIncompleteErrorMessage(t *testing.T) {
	err := goldVODSegmentsIncompleteError(GoldVODSegmentUnresolvedSummary{
		Queued:  1,
		Running: 2,
		Failed:  3,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gold vod segments incomplete") {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestSyncServiceGoldVODSegmentsBlockCompletionDisabled(t *testing.T) {
	svc := &SyncService{goldVODSegmentsEnabled: false, store: &Store{}}
	if err := svc.goldVODSegmentsBlockCompletion(t.Context(), 1, "stream-1"); err != nil {
		t.Fatalf("disabled flag should not block: %v", err)
	}
}

func TestBackfillWorkerApplyGoldSegmentCompletionGateSkipsWhenSyncFailed(t *testing.T) {
	w := &BackfillWorker{sync: &SyncService{goldVODSegmentsEnabled: true, store: &Store{}}}
	syncErr := errors.New("sync failed")
	got := w.applyGoldSegmentCompletionGate(t.Context(), &BackfillJob{Tier: "gold", ID: 1}, "stream-1", syncErr)
	if got != syncErr {
		t.Fatalf("got %v, want original sync err", got)
	}
}

func TestBackfillWorkerApplyGoldSegmentCompletionGateSkipsGoldLite(t *testing.T) {
	w := &BackfillWorker{sync: &SyncService{goldVODSegmentsEnabled: true, store: &Store{}}}
	got := w.applyGoldSegmentCompletionGate(t.Context(), &BackfillJob{Tier: "gold_lite", ID: 1}, "stream-1", nil)
	if got != nil {
		t.Fatalf("gold_lite should skip gate: %v", got)
	}
}

func TestResolveGoldBackfillOutcomeSegmentsIncompleteRequeues(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	job := BackfillJob{Tier: "gold", Attempt: 0, ExportStatus: "pending"}
	syncErr := errors.New("gold vod segments incomplete: queued=1 running=0 failed=0 dead_letter=0")
	got := resolveGoldBackfillOutcome(job, syncErr, now)
	if got.status != "queued" || !got.requeue {
		t.Fatalf("incomplete segments should requeue, got %+v", got)
	}
}

func TestResolveGoldBackfillOutcomeSegmentsIncompleteFailsAfterMaxAttempts(t *testing.T) {
	job := BackfillJob{Tier: "gold", Attempt: maxBackfillSyncAttempts - 1, ExportStatus: "pending"}
	syncErr := errors.New("gold vod segments incomplete: failed=2 running=1")
	got := resolveGoldBackfillOutcome(job, syncErr, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("final incomplete segments attempt should fail, got %+v", got)
	}
}

func TestGoldVODSegmentUnresolvedSummaryByJobID(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9001)
	plans := PlanGoldVODSegments("vod-complete-1", "stream-complete-1", "xqc", 1200, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim first segment: claim=%v err=%v", claim, err)
	}
	if _, err := store.CompleteGoldVODSegment(ctx, claim.ID, "worker-a", 3, "100"); err != nil {
		t.Fatalf("complete first: %v", err)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, "stream-complete-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !summary.BlocksCompletion() || summary.Queued != 1 {
		t.Fatalf("expected one queued tail segment, got %+v", summary)
	}

	claim2, err := store.ClaimGoldVODSegmentByKey(ctx, plans[1].SegmentKey, "worker-a", time.Minute, 2)
	if err != nil || claim2 == nil {
		t.Fatalf("claim second: %v", err)
	}
	if _, err := store.CompleteGoldVODSegment(ctx, claim2.ID, "worker-a", 0, "700"); err != nil {
		t.Fatalf("complete second: %v", err)
	}

	summary, err = store.GoldVODSegmentUnresolvedSummary(ctx, jobID, "stream-complete-1")
	if err != nil {
		t.Fatalf("summary after complete: %v", err)
	}
	if summary.BlocksCompletion() {
		t.Fatalf("all segments done should not block, got %+v", summary)
	}
}

func TestGoldVODSegmentUnresolvedSummaryFailedBlocks(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9002)
	plans := PlanGoldVODSegments("vod-failed-1", "stream-failed-1", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-a", "rate limited", time.Minute); err != nil {
		t.Fatalf("fail: %v", err)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, "stream-failed-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Failed != 1 || !summary.BlocksCompletion() {
		t.Fatalf("failed segment should block, got %+v", summary)
	}
	if err := goldVODSegmentsIncompleteError(summary); err == nil {
		t.Fatal("expected blocking error")
	}
}

func TestGoldVODSegmentUnresolvedSummaryDeadLetterBlocks(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9003)
	plans := PlanGoldVODSegments("vod-dl-1", "stream-dl-1", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-a", "gql video comments status 429 after 5 retries", time.Minute); err != nil {
		t.Fatalf("fail: %v", err)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, "stream-dl-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.DeadLetter != 1 || !summary.BlocksCompletion() {
		t.Fatalf("dead_letter should block, got %+v", summary)
	}
}

func TestGoldVODSegmentUnresolvedSummaryRunningBlocks(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9004)
	plans := PlanGoldVODSegments("vod-run-1", "stream-run-1", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, "worker-a", time.Minute, 2); err != nil {
		t.Fatalf("claim: %v", err)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, "stream-run-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Running != 1 || !summary.BlocksCompletion() {
		t.Fatalf("running lease should block, got %+v", summary)
	}
}
