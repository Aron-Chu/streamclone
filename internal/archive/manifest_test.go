package archive

import "testing"

func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"":          StatusPending,
		"pending":   StatusPending,
		"confirmed": StatusConfirmed,
		"failed":    StatusFailed,
		"unknown":   StatusPending,
	}
	for input, want := range cases {
		if got := normalizeStatus(input); got != want {
			t.Fatalf("normalizeStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBlockIfMissing(t *testing.T) {
	if err := BlockIfMissing(ArtifactAnalyticsStream, 0); err != nil {
		t.Fatalf("BlockIfMissing with zero missing returned %v", err)
	}
	err := BlockIfMissing(ArtifactAnalyticsStream, 2)
	if err == nil {
		t.Fatal("expected missing artifacts to block")
	}
	blocked, ok := err.(RetentionBlockedError)
	if !ok {
		t.Fatalf("error type = %T, want RetentionBlockedError", err)
	}
	if blocked.ArtifactType != ArtifactAnalyticsStream || blocked.Missing != 2 {
		t.Fatalf("blocked error = %+v", blocked)
	}
}
