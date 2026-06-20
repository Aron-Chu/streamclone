package ingest

import "testing"

func TestRisingScore(t *testing.T) {
	score := risingScore(50, 10, false, 2, 5000)
	if score <= 0 {
		t.Fatalf("expected positive score for strong momentum, got %v", score)
	}
	entrant := risingScore(10, 0, true, 0, 1000)
	flat := risingScore(0, 0, false, 0, 1000)
	if entrant <= flat {
		t.Fatalf("new entrant with viewers should beat flat score: entrant=%v flat=%v", entrant, flat)
	}
}
