package score

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"streamclone/internal/storygraph/store"
)

func TestComputeVolatilityInsufficientSamples(t *testing.T) {
	if got := ComputeVolatility(nil); got != nil {
		t.Fatalf("expected nil for empty input, got %v", *got)
	}
	if got := ComputeVolatility([]float64{42}); got != nil {
		t.Fatalf("expected nil for single sample, got %v", *got)
	}
}

func TestComputeVolatilityMeanAbsoluteDelta(t *testing.T) {
	trends := []float64{10, 30, 20}
	got := ComputeVolatility(trends)
	if got == nil {
		t.Fatal("expected volatility value")
	}
	want := 15.0 // |30-10| + |20-30| = 20+10, mean = 15
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("ComputeVolatility() = %v, want %v", *got, want)
	}
}

func TestComputeVolatilityFlatTrend(t *testing.T) {
	got := ComputeVolatility([]float64{50, 50, 50})
	if got == nil {
		t.Fatal("expected zero volatility")
	}
	if *got != 0 {
		t.Fatalf("expected 0 volatility for flat trend, got %v", *got)
	}
}

func TestComputeVolatilityMonotonicRise(t *testing.T) {
	got := ComputeVolatility([]float64{0, 25, 50, 75})
	if got == nil {
		t.Fatal("expected volatility value")
	}
	if *got != 25 {
		t.Fatalf("expected constant delta volatility 25, got %v", *got)
	}
}

func TestSuddenCommentRatioRequiresHistoryAndSharpJump(t *testing.T) {
	base := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	history := []store.SocialMetricSnapshot{
		{At: base.Add(-2 * time.Hour), Comments: intPtr(8)},
		{At: base.Add(-1 * time.Hour), Comments: intPtr(10)},
	}
	if got := suddenCommentRatio(history, 45); got < 3 {
		t.Fatalf("suddenCommentRatio = %v, want spike >= 3", got)
	}
	if got := suddenCommentRatio(history[:1], 45); got != 0 {
		t.Fatalf("single history sample should not flag, got %v", got)
	}
	if got := suddenCommentRatio(history, 21); got != 0 {
		t.Fatalf("small comment increase should not flag, got %v", got)
	}
}

func TestMergeScoreFactorsDeduplicatesExistingAndNewFactors(t *testing.T) {
	existing, _ := json.Marshal([]string{"source_weight", "sudden_comment_ratio:reddit"})
	got := mergeScoreFactors(existing, []string{"sudden_comment_ratio:reddit", "duplicate_author:clipper"})
	want := []string{"source_weight", "sudden_comment_ratio:reddit", "duplicate_author:clipper"}
	if len(got) != len(want) {
		t.Fatalf("factor count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("factor[%d] = %q, want %q in %+v", i, got[i], want[i], got)
		}
	}
}

func intPtr(v int) *int {
	return &v
}
