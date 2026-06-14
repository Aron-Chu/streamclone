package heatmap

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// TestLogTransformSafety is a property-based test for the ln(count+1) log
// transform applied to count signals before z-score normalization. For any
// arbitrary non-negative count the transform must be finite (never NaN/Inf),
// non-negative, map 0 -> 0, and preserve ordering (monotonically increasing).
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100),
// drawing arbitrary non-negative counts across the full practical range.
//
// Feature: moment-timeline, Property 7: Log Transform Safety
//
// **Validates: Requirements 9.3**
func TestLogTransformSafety(t *testing.T) {
	// logTransform(0) == 0 (a fixed point, asserted once outside the generator
	// loop so the property holds independent of drawn values).
	if got := logTransform(0); got != 0 {
		t.Fatalf("logTransform(0) = %v, want exactly 0", got)
	}

	rapid.Check(t, func(t *rapid.T) {
		// Arbitrary non-negative counts. Counts in this engine derive from
		// chat/emote tallies, so model them as non-negative float64 spanning
		// from 0 up to a large bound well beyond any realistic per-window count.
		a := rapid.Float64Range(0, 1e12).Draw(t, "a")
		b := rapid.Float64Range(0, 1e12).Draw(t, "b")

		fa := logTransform(a)
		fb := logTransform(b)

		// Finite: never NaN or +/-Inf.
		if math.IsNaN(fa) || math.IsInf(fa, 0) {
			t.Fatalf("logTransform(%v) not finite: %v", a, fa)
		}
		if math.IsNaN(fb) || math.IsInf(fb, 0) {
			t.Fatalf("logTransform(%v) not finite: %v", b, fb)
		}

		// Non-negative for non-negative input.
		if fa < 0 {
			t.Fatalf("logTransform(%v) negative: %v", a, fa)
		}
		if fb < 0 {
			t.Fatalf("logTransform(%v) negative: %v", b, fb)
		}

		// Order-preserving: a < b => logTransform(a) <= logTransform(b).
		if a < b && fa > fb {
			t.Fatalf("not order-preserving: a=%v b=%v but logTransform(a)=%v > logTransform(b)=%v", a, b, fa, fb)
		}
	})
}
