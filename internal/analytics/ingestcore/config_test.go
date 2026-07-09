package ingestcore

import "testing"

func TestNormalizeCandidateScanTopN(t *testing.T) {
	if got := normalizeCandidateScanTopN(0, 500); got != 500 {
		t.Fatalf("normalizeCandidateScanTopN(0, 500) = %d, want 500", got)
	}
	if got := normalizeCandidateScanTopN(400, 500); got != 400 {
		t.Fatalf("normalizeCandidateScanTopN(400, 500) = %d, want 400", got)
	}
}
