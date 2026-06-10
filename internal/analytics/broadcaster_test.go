package analytics

import "testing"

func TestNormalizeBroadcasterID(t *testing.T) {
	if got := NormalizeBroadcasterID("12345"); got != "12345" {
		t.Fatalf("expected id, got %q", got)
	}
	if got := NormalizeBroadcasterID("pending"); got != "" {
		t.Fatalf("expected empty for pending, got %q", got)
	}
	if got := NormalizeBroadcasterID(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
