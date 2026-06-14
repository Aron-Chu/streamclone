package heatmap

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// TestEWMAForwardOnlyCausality is a property-based test for the forward-only
// EWMA smoothing recurrence (Requirement 9.4). EWMA is applied as a strictly
// forward pass so it is causal: smoothed[i] depends only on scores[0..i] and
// never on later windows. The defining consequence is that mutating the score
// at index k must leave every earlier smoothed value smoothed[j] (j < k)
// bit-for-bit identical, while smoothed[k] and later values may change.
//
// The test also asserts the boundary sanity property smoothed[0] == scores[0]:
// the first element is carried through unchanged regardless of inputs.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100),
// generating an arbitrary scores slice, an index k, and a replacement value.
//
// Feature: moment-timeline, Property 8: EWMA Forward-Only Causality
//
// **Validates: Requirements 9.4**
func TestEWMAForwardOnlyCausality(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a non-empty scores slice spanning the practical score range
		// (and beyond) so the property is exercised across realistic and
		// extreme inputs.
		n := rapid.IntRange(1, 256).Draw(t, "n")
		scores := make([]float64, n)
		for i := range scores {
			scores[i] = rapid.Float64Range(-1e6, 1e6).Draw(t, "score")
		}

		// Smoothing parameters. alpha governs the recurrence; span is accepted
		// for config parity and does not affect the result, so draw arbitrary
		// values for both to keep the property general.
		alpha := rapid.Float64Range(0, 1).Draw(t, "alpha")
		span := rapid.IntRange(1, 10).Draw(t, "span")

		// Pick an index k to mutate and a replacement value.
		k := rapid.IntRange(0, n-1).Draw(t, "k")
		replacement := rapid.Float64Range(-1e6, 1e6).Draw(t, "replacement")

		original := ewmaSmooth(scores, span, alpha)

		// Boundary sanity: the first smoothed value equals the first input.
		if original[0] != scores[0] {
			t.Fatalf("smoothed[0] = %v, want scores[0] = %v", original[0], scores[0])
		}

		// Build the mutated slice (copy first so the original input is intact).
		mutated := make([]float64, n)
		copy(mutated, scores)
		mutated[k] = replacement
		smoothedMutated := ewmaSmooth(mutated, span, alpha)

		// Forward-only causality: every smoothed value before index k must be
		// bit-for-bit identical, since smoothed[j] (j < k) depends only on
		// scores[0..j], none of which changed.
		for j := 0; j < k; j++ {
			if original[j] != smoothedMutated[j] {
				t.Fatalf("causality violated at j=%d (k=%d): original=%v mutated=%v",
					j, k, original[j], smoothedMutated[j])
			}
		}

		// Sanity on the smoothed outputs: forward pass must stay finite for
		// finite inputs and produce the same length.
		if len(smoothedMutated) != n {
			t.Fatalf("length changed: got %d want %d", len(smoothedMutated), n)
		}
		for i, v := range original {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("original smoothed[%d] not finite: %v", i, v)
			}
		}
	})
}
