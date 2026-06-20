package sync_test

import (
	"testing"

	emotesync "streamclone/internal/emote/sync"
)

func TestProviderSnapshotNeedsRefresh(t *testing.T) {
	if !emotesync.ProviderSnapshotNeedsRefresh(false, "", "", "set-1", "abc", 0, 10) {
		t.Fatal("expected refresh when local snapshot missing")
	}
	if emotesync.ProviderSnapshotNeedsRefresh(true, "set-1", "abc", "set-1", "abc", 10, 10) {
		t.Fatal("expected matching snapshot to be fresh")
	}
	if !emotesync.ProviderSnapshotNeedsRefresh(true, "set-1", "abc", "set-1", "def", 10, 10) {
		t.Fatal("expected hash mismatch to refresh")
	}
}

func TestHashSevenTVEmoteIDsStable(t *testing.T) {
	left := emotesync.HashSevenTVEmoteIDs([]string{"b", "a", "c"})
	right := emotesync.HashSevenTVEmoteIDs([]string{"c", "b", "a"})
	if left == "" || left != right {
		t.Fatalf("hash mismatch: %q vs %q", left, right)
	}
}
