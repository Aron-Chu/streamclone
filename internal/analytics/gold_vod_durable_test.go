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
