package analytics

import "testing"

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

func TestSyncStatusKeys(t *testing.T) {
	if got := syncStatusKey("123"); got != "analytics:sync:123" {
		t.Fatalf("syncStatusKey = %q", got)
	}
	if got := syncLockKey("123"); got != "analytics:sync-lock:123" {
		t.Fatalf("syncLockKey = %q", got)
	}
}
