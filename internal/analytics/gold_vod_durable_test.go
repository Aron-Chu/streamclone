package analytics

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGoldVODFetchLedgerNilWhenDisabled(t *testing.T) {
	svc := &SyncService{goldVODSegmentsEnabled: false, store: &Store{}}
	if got := svc.goldVODFetchLedger(context.Background(), "stream-1", "xqc", "vod-1"); got != nil {
		t.Fatalf("ledger = %#v, want nil when disabled", got)
	}
}

func TestWithGoldBackfillJobID(t *testing.T) {
	ctx := WithGoldBackfillJobID(context.Background(), 42)
	got := goldBackfillJobIDFromContext(ctx)
	if got == nil || *got != 42 {
		t.Fatalf("job id = %#v, want 42", got)
	}
}

func TestSanitizeGoldSegmentErrorTruncates(t *testing.T) {
	long := errors.New(string(make([]byte, 600)))
	got := sanitizeGoldSegmentError(long)
	if len(got) != 500 {
		t.Fatalf("len = %d, want 500", len(got))
	}
}

func TestGoldVODFetchLedgerUpsertPlansIdempotent(t *testing.T) {
	plans := PlanGoldVODSegments("vod-1", "stream-1", "xqc", 1200, 600, "")
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(plans))
	}
	if plans[0].SegmentKey == plans[1].SegmentKey {
		t.Fatal("expected distinct segment keys")
	}
}

func TestDefaultGoldVODSegmentOwner(t *testing.T) {
	owner := defaultGoldVODSegmentOwner()
	if owner == "" {
		t.Fatal("expected non-empty owner")
	}
}

func TestGoldVODSegmentRetryLaterIsDistinct(t *testing.T) {
	if errors.Is(errGoldSegmentRetryLater, errGoldSegmentSkip) {
		t.Fatal("retry later should differ from skip")
	}
	if errGoldSegmentRetryLater.Error() == "" {
		t.Fatal("expected message")
	}
	_ = time.Second
}

func TestGoldVODFetchLedgerHotSplitRetiresParent(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9101)
	vodID := "vod-hot-1"
	streamID := "stream-hot-1"
	plans := PlanGoldVODSegments(vodID, streamID, "xqc", 3600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	owner := "worker-hot-a"
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, owner, time.Minute, 4)
	if err != nil || claim == nil {
		t.Fatalf("claim parent: %+v err=%v", claim, err)
	}

	svc := &SyncService{
		goldVODSegmentsEnabled: true,
		store:                  store,
		goldRetryMax:           3,
		goldLeaseTTL:           time.Minute,
		goldMaxSegmentsPerVOD:  4,
		goldVODSegmentOwner:    owner,
	}
	ledger := svc.goldVODFetchLedger(WithGoldBackfillJobID(ctx, jobID), streamID, "xqc", vodID)
	parentSeg := gqlSegmentProgress{StartSec: 0, EndSec: 1199, OffsetSec: 0}
	ledger.mu.Lock()
	ledger.activeClaimID[ledger.segmentKey(parentSeg)] = claim.ID
	ledger.mu.Unlock()

	splitAt := 600
	tail := gqlSegmentProgress{StartSec: splitAt, EndSec: 1199, OffsetSec: splitAt}
	if err := ledger.onHotSplit(parentSeg, splitAt, tail); err != nil {
		t.Fatalf("onHotSplit: %v", err)
	}

	status, found, err := store.GoldVODSegmentStatusByKey(ctx, plans[0].SegmentKey)
	if err != nil {
		t.Fatalf("parent status: %v", err)
	}
	if !found || status != "skipped" {
		t.Fatalf("parent status = %q found=%v, want skipped", status, found)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, streamID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Failed != 0 || summary.Running != 0 {
		t.Fatalf("parent should not block completion, got %+v", summary)
	}
	if summary.Queued < 1 {
		t.Fatalf("expected child segments queued, got %+v", summary)
	}
}

func TestGoldVODFetchLedgerHotSplitChildUpsertFailPreservesParent(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9102)
	vodID := "vod-hot-2"
	streamID := "stream-hot-2"
	plans := PlanGoldVODSegments(vodID, streamID, "xqc", 3600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	owner := "worker-hot-b"
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, owner, time.Minute, 4)
	if err != nil || claim == nil {
		t.Fatalf("claim parent: %+v err=%v", claim, err)
	}

	cancelled, cancel := context.WithCancel(WithGoldBackfillJobID(ctx, jobID))
	cancel()
	svc := &SyncService{
		goldVODSegmentsEnabled: true,
		store:                  store,
		goldRetryMax:           3,
		goldLeaseTTL:           time.Minute,
		goldMaxSegmentsPerVOD:  4,
		goldVODSegmentOwner:    owner,
	}
	ledger := svc.goldVODFetchLedger(cancelled, streamID, "xqc", vodID)
	parentSeg := gqlSegmentProgress{StartSec: 0, EndSec: 1199, OffsetSec: 0}
	ledger.mu.Lock()
	ledger.activeClaimID[ledger.segmentKey(parentSeg)] = claim.ID
	ledger.mu.Unlock()

	if err := ledger.onHotSplit(parentSeg, 600, gqlSegmentProgress{StartSec: 600, EndSec: 1199, OffsetSec: 600}); err == nil {
		t.Fatal("expected child upsert failure")
	}

	status, found, err := store.GoldVODSegmentStatusByKey(ctx, plans[0].SegmentKey)
	if err != nil {
		t.Fatalf("parent status: %v", err)
	}
	if !found || status != "running" {
		t.Fatalf("parent status = %q found=%v, want running", status, found)
	}
}

func TestGoldVODFetchLedgerHotSplitSkipFailKeepsParentBlocking(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	jobID := int64(9103)
	vodID := "vod-hot-3"
	streamID := "stream-hot-3"
	plans := PlanGoldVODSegments(vodID, streamID, "xqc", 3600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, &jobID, 3); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	owner := "worker-hot-c"
	claim, err := store.ClaimGoldVODSegmentByKey(ctx, plans[0].SegmentKey, owner, time.Minute, 4)
	if err != nil || claim == nil {
		t.Fatalf("claim parent: %+v err=%v", claim, err)
	}

	svc := &SyncService{
		goldVODSegmentsEnabled: true,
		store:                  store,
		goldRetryMax:           3,
		goldLeaseTTL:           time.Minute,
		goldMaxSegmentsPerVOD:  4,
		goldVODSegmentOwner:    "worker-hot-wrong",
	}
	ledger := svc.goldVODFetchLedger(WithGoldBackfillJobID(ctx, jobID), streamID, "xqc", vodID)
	parentSeg := gqlSegmentProgress{StartSec: 0, EndSec: 1199, OffsetSec: 0}
	ledger.mu.Lock()
	ledger.activeClaimID[ledger.segmentKey(parentSeg)] = claim.ID
	ledger.mu.Unlock()

	if err := ledger.onHotSplit(parentSeg, 600, gqlSegmentProgress{StartSec: 600, EndSec: 1199, OffsetSec: 600}); err == nil {
		t.Fatal("expected parent skip failure")
	}

	status, found, err := store.GoldVODSegmentStatusByKey(ctx, plans[0].SegmentKey)
	if err != nil {
		t.Fatalf("parent status: %v", err)
	}
	if !found || status != "running" {
		t.Fatalf("parent status = %q found=%v, want running", status, found)
	}

	summary, err := store.GoldVODSegmentUnresolvedSummary(ctx, jobID, streamID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !summary.BlocksCompletion() || summary.Running != 1 {
		t.Fatalf("parent running lease should block, got %+v", summary)
	}
}
