package heatmap

import (
	"testing"

	"pgregory.net/rapid"
)

// TestNonMaxSuppressionLocality is a property-based test for suppressPeaks
// (Requirement 9.5). After non-max suppression with threshold T and radius R,
// no two retained scores at or above T may remain within radius R of each
// other: for any indices i < j with result[i] >= T and result[j] >= T, the gap
// j-i must be strictly greater than R. Additionally, scores strictly below the
// threshold are never altered by suppression.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100),
// drawing arbitrary score slices, thresholds, and radii in [1,10].
//
// Feature: moment-timeline, Property 9: Non-Max Suppression Locality
//
// **Validates: Requirements 9.5**
func TestNonMaxSuppressionLocality(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// A score slice spanning empty up to a length comfortably larger than
		// the max radius so locality interactions are exercised. Scores are
		// drawn around the threshold band so many land at/above T.
		scores := rapid.SliceOfN(rapid.Float64Range(0, 100), 0, 64).Draw(t, "scores")
		// Threshold is a positive peak cutoff: suppressed values become 0, so a
		// zero threshold would conflate zeroed and retained scores. Real peak
		// thresholds are > 0, so draw from [1,100].
		threshold := rapid.IntRange(1, 100).Draw(t, "threshold")
		radius := rapid.IntRange(1, 10).Draw(t, "radius")

		result := suppressPeaks(scores, threshold, radius)

		if len(result) != len(scores) {
			t.Fatalf("length changed: got %d, want %d", len(result), len(scores))
		}

		t64 := float64(threshold)

		// Locality: no two retained scores >= T within radius R of each other.
		lastKept := -1
		for i := range result {
			if result[i] >= t64 {
				if lastKept >= 0 && i-lastKept <= radius {
					t.Fatalf("two scores >= T=%d remain within radius R=%d: indices %d and %d (gap %d)",
						threshold, radius, lastKept, i, i-lastKept)
				}
				lastKept = i
			}
		}

		// Sub-threshold scores are preserved unchanged.
		for i := range scores {
			if scores[i] < t64 && result[i] != scores[i] {
				t.Fatalf("sub-threshold score at index %d altered: input=%v output=%v (T=%d)",
					i, scores[i], result[i], threshold)
			}
		}
	})
}
