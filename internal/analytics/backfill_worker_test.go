package analytics

import (
	"context"
	"errors"
	"fmt"
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
	if backfillExportLabel("gold") != "backfill gold chat" {
		t.Fatal("unexpected gold export label")
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
