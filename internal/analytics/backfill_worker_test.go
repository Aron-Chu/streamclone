package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsSyncTimeoutError(t *testing.T) {
	if isSyncTimeoutError(nil) {
		t.Fatal("nil error should not be timeout")
	}
	if !isSyncTimeoutError(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded should match")
	}
	if !isSyncTimeoutError(fmt.Errorf("scraper: %w", context.DeadlineExceeded)) {
		t.Fatal("wrapped DeadlineExceeded should match")
	}
	if !isSyncTimeoutError(errors.New("context deadline exceeded after 45s")) {
		t.Fatal("deadline exceeded message should match")
	}
	if isSyncTimeoutError(errors.New("tracker access protected")) {
		t.Fatal("non-timeout error should not match")
	}
}

func TestBackfillRetryDelay(t *testing.T) {
	if got := backfillRetryDelay(1); got != 60*time.Second {
		t.Fatalf("attempt 1 delay = %v, want 60s", got)
	}
	if got := backfillRetryDelay(2); got != 120*time.Second {
		t.Fatalf("attempt 2 delay = %v, want 120s", got)
	}
	if got := backfillRetryDelay(5); got != 5*time.Minute {
		t.Fatalf("attempt 5 delay = %v, want capped at 5m", got)
	}
}

func TestResolveBackfillOutcomeSuccess(t *testing.T) {
	job := BackfillJob{Attempt: 0, ExportStatus: "pending"}
	got := resolveBackfillOutcome(job, nil, time.Now())
	if got.status != "done" || got.exportStatus != "confirmed" || got.requeue {
		t.Fatalf("success outcome = %+v", got)
	}
	if got.attempt != 1 {
		t.Fatalf("attempt = %d, want 1", got.attempt)
	}
}

func TestResolveBackfillOutcomeTimeoutRequeues(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	job := BackfillJob{Attempt: 0, ExportStatus: "pending"}
	syncErr := fmt.Errorf("failed to scrape TwitchTracker: %w", context.DeadlineExceeded)
	got := resolveBackfillOutcome(job, syncErr, now)
	if got.status != "queued" || !got.requeue {
		t.Fatalf("timeout should requeue, got %+v", got)
	}
	if got.exportStatus != "pending" {
		t.Fatalf("export_status = %q, want pending", got.exportStatus)
	}
	if got.attempt != 1 {
		t.Fatalf("attempt = %d, want 1", got.attempt)
	}
	wantNext := now.Add(60 * time.Second)
	if !got.nextRunAt.Equal(wantNext) {
		t.Fatalf("next_run_at = %v, want %v", got.nextRunAt, wantNext)
	}
}

func TestResolveBackfillOutcomeTimeoutFailsAfterMaxAttempts(t *testing.T) {
	job := BackfillJob{Attempt: maxBackfillSyncAttempts - 1, ExportStatus: "pending"}
	syncErr := errors.New("context deadline exceeded")
	got := resolveBackfillOutcome(job, syncErr, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("final timeout should fail, got %+v", got)
	}
	if got.exportStatus != "failed" {
		t.Fatalf("export_status = %q, want failed", got.exportStatus)
	}
	if got.attempt != maxBackfillSyncAttempts {
		t.Fatalf("attempt = %d, want %d", got.attempt, maxBackfillSyncAttempts)
	}
}

func TestResolveBackfillOutcomeNonTimeoutFailsImmediately(t *testing.T) {
	job := BackfillJob{Attempt: 0, ExportStatus: "pending"}
	syncErr := errors.New("tracker access protected (cloudflare challenge/block)")
	got := resolveBackfillOutcome(job, syncErr, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("non-timeout should fail immediately, got %+v", got)
	}
	if got.attempt != 1 {
		t.Fatalf("attempt = %d, want 1", got.attempt)
	}
}

func TestBackfillSyncParamsGold(t *testing.T) {
	viewersOnly, forceChat := backfillSyncParams("gold")
	if viewersOnly || !forceChat {
		t.Fatalf("gold params = viewersOnly=%v forceChat=%v", viewersOnly, forceChat)
	}
}

func TestBackfillSyncParamsSilver(t *testing.T) {
	viewersOnly, forceChat := backfillSyncParams("silver")
	if !viewersOnly || forceChat {
		t.Fatalf("silver params = viewersOnly=%v forceChat=%v", viewersOnly, forceChat)
	}
}

func TestBackfillExportLabel(t *testing.T) {
	if backfillExportLabel("gold") != "backfill gold-full chat" {
		t.Fatal("unexpected gold export label")
	}
	if backfillExportLabel("gold_lite") != "backfill gold-lite aggregates" {
		t.Fatal("unexpected gold_lite export label")
	}
	if backfillExportLabel("silver") != "backfill viewers-only" {
		t.Fatal("unexpected silver export label")
	}
}

type mockVODChatExporter struct{}

func (mockVODChatExporter) ExportVODChat(context.Context, string) error { return nil }

func TestNewBackfillWorkerVODChatExporter(t *testing.T) {
	exp := mockVODChatExporter{}
	w := NewBackfillWorker(nil, nil, nil, time.Second).WithVODChatExporter(exp)
	if w.vodChatExporter == nil {
		t.Fatal("expected vod chat exporter")
	}
}

func TestResolveBackfillOutcomeGoldSuccess(t *testing.T) {
	job := BackfillJob{Tier: "gold", Attempt: 0, ExportStatus: "pending"}
	got := resolveBackfillOutcome(job, nil, time.Now())
	if got.status != "done" || got.exportStatus != "confirmed" {
		t.Fatalf("gold success outcome = %+v", got)
	}
}

func TestIsGoldWorkerTierFilter(t *testing.T) {
	if !IsGoldWorkerTierFilter([]string{"gold", "gold_full", "gold_lite"}) {
		t.Fatal("expected gold tier filter")
	}
	if IsGoldWorkerTierFilter([]string{"silver"}) {
		t.Fatal("silver should not be gold filter")
	}
	if IsGoldWorkerTierFilter([]string{"gold", "silver"}) {
		t.Fatal("mixed tiers should not be gold-only filter")
	}
}

func TestClaimNextSQLGoldReadiness(t *testing.T) {
	sql := ClaimNextSQL([]string{"gold", "gold_full", "gold_lite"})
	if !strings.Contains(sql, "EXISTS") {
		t.Fatal("gold claim SQL should require silver readiness")
	}
	if !strings.Contains(sql, "export_status = 'confirmed'") {
		t.Fatal("gold claim SQL should require confirmed silver export")
	}
	if !strings.Contains(sql, "WITH RECURSIVE canonical_path") {
		t.Fatal("gold claim SQL should resolve silver aliases transitively")
	}
}

func TestClaimNextSQLSilverTier(t *testing.T) {
	sql := ClaimNextSQL([]string{"silver"})
	if !strings.Contains(sql, "tier = ANY") {
		t.Fatal("silver claim SQL should filter by tier")
	}
	if strings.Contains(sql, "EXISTS") {
		t.Fatal("silver claim SQL should not use readiness join")
	}
}

func TestBackfillWorkerOptions(t *testing.T) {
	w := NewBackfillWorker(nil, nil, nil, time.Second).
		WithWorkerOptions(BackfillWorkerOptions{
			Name:              "silver",
			TierFilter:        []string{"silver"},
			StaleRunningAfter: 2 * time.Hour,
			HeartbeatInterval: time.Minute,
		})
	if w.workerName() != "silver" {
		t.Fatalf("worker name = %q", w.workerName())
	}
	if len(w.tierFilter) != 1 || w.tierFilter[0] != "silver" {
		t.Fatalf("tier filter = %v", w.tierFilter)
	}
	if w.staleRunningAfter != 2*time.Hour || w.heartbeatInterval != time.Minute {
		t.Fatalf("stale lease = %v heartbeat = %v", w.staleRunningAfter, w.heartbeatInterval)
	}
}

func TestRunBackfillWorkerDrainContinuesUntilEmpty(t *testing.T) {
	calls := 0
	runBackfillWorkerDrain(context.Background(), "test", func(context.Context) (bool, error) {
		calls++
		return calls < 4, nil
	}, nil)
	if calls != 4 {
		t.Fatalf("calls = %d, want 4 (three jobs plus empty claim)", calls)
	}
}

func TestRunBackfillWorkerDrainStopsOnError(t *testing.T) {
	calls := 0
	runBackfillWorkerDrain(context.Background(), "test", func(context.Context) (bool, error) {
		calls++
		if calls == 2 {
			return false, errors.New("boom")
		}
		return true, nil
	}, nil)
	if calls != 2 {
		t.Fatalf("calls = %d, want stop after error on second call", calls)
	}
}

func TestReclaimRunningOnStartupSQL(t *testing.T) {
	sql := strings.ToLower(reclaimRunningOnStartupSQL)
	if !strings.Contains(sql, "status='running'") {
		t.Fatal("startup reclaim SQL should target running jobs")
	}
	if !strings.Contains(sql, "[startup reclaim]") {
		t.Fatal("startup reclaim SQL should tag error message")
	}
	if !strings.Contains(sql, "status='queued'") {
		t.Fatal("startup reclaim SQL should requeue jobs")
	}
}

func TestBackfillPanicRequeueSQL(t *testing.T) {
	sql := strings.ToLower(backfillPanicRequeueSQL)
	if !strings.Contains(sql, "[panic reclaim]") {
		t.Fatal("panic requeue SQL should tag error message")
	}
	if !strings.Contains(sql, "status='running'") {
		t.Fatal("panic requeue SQL should only affect running jobs")
	}
}

func TestReclaimRunningOnStartupNilDB(t *testing.T) {
	_, err := ReclaimRunningOnStartup(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil db err = %v", err)
	}
}
