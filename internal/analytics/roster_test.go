package analytics

import (
	"testing"
	"time"
)

func TestTier0CoveragePct(t *testing.T) {
	start := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(60 * time.Minute)
	stream := &StreamRecord{
		StartedAt:    start,
		EndedAt:      &end,
		ViewerSource: ViewerSourceLive,
	}
	if pct := Tier0CoveragePct(stream, nil); pct != 100 {
		t.Fatalf("live source should be 100%%, got %v", pct)
	}
}

func TestTierForRank(t *testing.T) {
	if tierForRank(10) != "P1" {
		t.Fatalf("rank 10 expected P1")
	}
	if tierForRank(100) != "P2" {
		t.Fatalf("rank 100 expected P2")
	}
}
