package analytics

import (
	"testing"
	"time"
)

func TestSyncPhaseIsTerminal(t *testing.T) {
	if SyncPhaseFetchingComments.IsTerminal() {
		t.Fatal("fetching_comments should not be terminal")
	}
	if !SyncPhaseCompleted.IsTerminal() {
		t.Fatal("completed should be terminal")
	}
	if !SyncPhaseFailed.IsTerminal() {
		t.Fatal("failed should be terminal")
	}
}

func TestSyncStatusIsStale(t *testing.T) {
	now := time.Now().UTC()
	active := &SyncStatus{
		Phase:     SyncPhaseFetchingComments,
		UpdatedAt: now.Add(-30 * time.Second),
	}
	if syncStatusIsStale(active) {
		t.Fatal("recent status should not be stale")
	}
	stale := &SyncStatus{
		Phase:     SyncPhaseResolvingVOD,
		UpdatedAt: now.Add(-2 * time.Minute),
	}
	if !syncStatusIsStale(stale) {
		t.Fatal("old non-terminal status should be stale")
	}
	if syncStatusIsStale(&SyncStatus{Phase: SyncPhaseCompleted, UpdatedAt: now.Add(-2 * time.Minute)}) {
		t.Fatal("completed should not be stale")
	}
}

func TestSyncStatusShouldReportStaleDependsOnCurrentOwner(t *testing.T) {
	status := &SyncStatus{
		Phase:     SyncPhaseFetchingComments,
		UpdatedAt: time.Now().UTC().Add(-2 * time.Minute),
	}
	if !syncStatusShouldReportStale(status, false) {
		t.Fatal("stale status without a current owner should be reported stale")
	}
	if syncStatusShouldReportStale(status, true) {
		t.Fatal("stale status owned by this process should stay active")
	}
}

func TestSyncLockOwnedBy(t *testing.T) {
	if !syncLockOwnedBy("owner-a", "owner-a") {
		t.Fatal("matching owner should own lock")
	}
	if syncLockOwnedBy("owner-a", "owner-b") {
		t.Fatal("different owner should not own lock")
	}
	if syncLockOwnedBy("", "owner-a") {
		t.Fatal("empty lock value should not be owned")
	}
}

func TestSyncStatusCacheThrottlesFlush(t *testing.T) {
	cache := &syncStatusCache{}
	cache.put(SyncStatus{StreamID: "1"})
	if !cache.shouldFlush("1", false) {
		t.Fatal("first flush should be allowed")
	}
	cache.markFlushed("1")
	if cache.shouldFlush("1", false) {
		t.Fatal("immediate second flush should be throttled")
	}
	if !cache.shouldFlush("1", true) {
		t.Fatal("forced flush should always run")
	}
}

func TestSyncStatusKeys(t *testing.T) {
	if got := syncStatusKey("123"); got != "analytics:sync:123" {
		t.Fatalf("syncStatusKey = %q", got)
	}
	if got := syncLockKey("123"); got != "analytics:sync-lock:123" {
		t.Fatalf("syncLockKey = %q", got)
	}
}
