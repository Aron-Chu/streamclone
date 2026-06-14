package heatmap

import (
	"testing"
	"time"
)

// TestComputeHeatmapSmoke is a minimal wiring check for task 8.7. The dedicated
// score-range (8.8), missing-window (8.9), determinism (8.10), and fixture
// (8.11) suites live separately.
func TestComputeHeatmapSmoke(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{
		{MinuteTS: base, ChatCount: 5, TotalEmoteCount: 2, ViewerAvg: 100},
		{MinuteTS: base.Add(time.Minute), ChatCount: 400, TotalEmoteCount: 300, SevenTVEmoteCount: 250, ViewerAvg: 900, Emotes: map[string]int{"seventv:1:KEKW": 250}},
		{MinuteTS: base.Add(2 * time.Minute), Missing: true},
		{MinuteTS: base.Add(3 * time.Minute), ChatCount: 6, TotalEmoteCount: 1, ViewerAvg: 110},
	}

	resp := ComputeHeatmap(rollups, DefaultScoringConfig())

	// After decimation (task 9.8), zero-score windows are omitted, so the point
	// count is the number of surviving non-zero windows, not len(rollups). With
	// only a few rollups (< MaxPoints) no sampling occurs — every non-zero
	// window is retained, offset-sorted.
	if len(resp.Points) > len(rollups) {
		t.Fatalf("points length = %d, must not exceed rollups %d", len(resp.Points), len(rollups))
	}
	if len(resp.Points) == 0 {
		t.Fatal("expected at least one non-zero scored point")
	}
	if resp.WindowSeconds != defaultWindowSeconds {
		t.Errorf("WindowSeconds = %d, want %d", resp.WindowSeconds, defaultWindowSeconds)
	}
	if resp.ScoringVersion != "v1" {
		t.Errorf("ScoringVersion = %q, want %q", resp.ScoringVersion, "v1")
	}

	lastOffset := -1
	for i, p := range resp.Points {
		if p.Score <= 0 || p.Score > 100 {
			t.Errorf("point %d score %d out of (0,100] (zero-score points must be omitted)", i, p.Score)
		}
		if p.OffsetSeconds%defaultWindowSeconds != 0 {
			t.Errorf("point %d offset = %d, want multiple of %d", i, p.OffsetSeconds, defaultWindowSeconds)
		}
		if p.OffsetSeconds <= lastOffset {
			t.Errorf("point %d offset = %d not strictly after previous %d (must be offset-sorted)", i, p.OffsetSeconds, lastOffset)
		}
		lastOffset = p.OffsetSeconds
		if p.DurationSeconds != defaultWindowSeconds {
			t.Errorf("point %d duration = %d, want %d", i, p.DurationSeconds, defaultWindowSeconds)
		}
		if !IsValidReason(p.Reason) {
			t.Errorf("point %d reason %q invalid", i, p.Reason)
		}
	}

	// The missing window (index 2, offset 120) scores 0 and must be omitted
	// from the decimated response (Requirements 9.7, 12.3).
	for _, p := range resp.Points {
		if p.OffsetSeconds == 2*defaultWindowSeconds {
			t.Errorf("missing/zero-score window at offset %d must be omitted", p.OffsetSeconds)
		}
	}

	// Determinism: recompute yields identical points (Requirement 9.6).
	resp2 := ComputeHeatmap(rollups, DefaultScoringConfig())
	if len(resp.Points) != len(resp2.Points) {
		t.Fatalf("non-deterministic point count: %d vs %d", len(resp.Points), len(resp2.Points))
	}
	for i := range resp.Points {
		a, b := resp.Points[i], resp2.Points[i]
		if a.Score != b.Score || a.OffsetSeconds != b.OffsetSeconds ||
			a.DurationSeconds != b.DurationSeconds || a.Reason != b.Reason ||
			!a.MinuteTs.Equal(b.MinuteTs) {
			t.Errorf("non-deterministic point %d: %+v vs %+v", i, a, b)
		}
	}
}

func TestCompositeScoreAllMissingReturnsZero(t *testing.T) {
	signals := map[string]float64{
		sigChatRate:          5,
		sigEmoteRate:         5,
		sigViewerMomentum:    5,
		sigProviderSpike:     5,
		sigTopEmoteDominance: 5,
		sigNovelty:           5,
	}
	if got := compositeScore(signals, DefaultScoringConfig().Weights, true); got != 0 {
		t.Errorf("allMissing compositeScore = %d, want 0", got)
	}
}

func TestCompositeScoreClampsNegativeZToZero(t *testing.T) {
	// All negative z-scores → positive-surprise clamp → raw 0 → score 0.
	signals := map[string]float64{
		sigChatRate:          -2,
		sigEmoteRate:         -1,
		sigViewerMomentum:    -3,
		sigProviderSpike:     -0.5,
		sigTopEmoteDominance: -1,
		sigNovelty:           -1,
	}
	if got := compositeScore(signals, DefaultScoringConfig().Weights, false); got != 0 {
		t.Errorf("below-average compositeScore = %d, want 0", got)
	}
}
