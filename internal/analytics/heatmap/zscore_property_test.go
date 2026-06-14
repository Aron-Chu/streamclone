package heatmap

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// independentMean computes the arithmetic mean without reusing the production
// mean helper, so the property test validates zScoreSlice against an
// independent reference rather than the implementation under test.
func independentMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// independentPopStddev computes the population (divide-by-N) standard deviation
// independently of the production stddev helper.
func independentPopStddev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := independentMean(values)
	var sumSq float64
	for _, v := range values {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)))
}

// distinctCount returns the number of distinct float64 values in the slice.
func distinctCount(values []float64) int {
	seen := make(map[float64]struct{}, len(values))
	for _, v := range values {
		seen[v] = struct{}{}
	}
	return len(seen)
}

// TestPerStreamZScoreNormalization is a property-based test for per-stream
// z-score normalization (zScoreSlice). The engine z-score normalizes each
// signal channel independently across a stream's own windows using a population
// (divide-by-N) standard deviation, so for any slice with >=2 distinct values
// the normalized output has an arithmetic mean of approximately 0 and a
// population standard deviation of approximately 1. For a slice whose values are
// all identical (population stddev 0, the divide-by-zero guard) the output is
// all zeros.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
// Value ranges are kept well-conditioned (|v| <= 1e6, length <= 256) so the
// recomputed mean/stddev stay within a tight floating-point tolerance.
//
// Feature: moment-timeline, Property 6: Per-Stream Z-Score Normalization
//
// **Validates: Requirements 9.2**
func TestPerStreamZScoreNormalization(t *testing.T) {
	const meanTol = 1e-9
	const stddevTol = 1e-9

	// Empty input: zScoreSlice returns an empty (non-nil) slice. Asserted once
	// outside the generator loop so the property holds independent of draws.
	if got := zScoreSlice(nil); len(got) != 0 {
		t.Fatalf("zScoreSlice(nil) length = %d, want 0", len(got))
	}

	// Main property: slices with >=2 distinct values normalize to mean ~0 and
	// population stddev ~1.
	rapid.Check(t, func(t *rapid.T) {
		values := rapid.SliceOfN(
			rapid.Float64Range(-1e6, 1e6),
			2, 256,
		).Draw(t, "values")

		out := zScoreSlice(values)

		if len(out) != len(values) {
			t.Fatalf("zScoreSlice length = %d, want %d", len(out), len(values))
		}

		// Output must always be finite.
		for i, z := range out {
			if math.IsNaN(z) || math.IsInf(z, 0) {
				t.Fatalf("zScoreSlice[%d] not finite: %v", i, z)
			}
		}

		if distinctCount(values) >= 2 {
			gotMean := independentMean(out)
			gotStddev := independentPopStddev(out)
			if math.Abs(gotMean) > meanTol {
				t.Fatalf("normalized mean = %v, want within %v of 0 (input=%v)", gotMean, meanTol, values)
			}
			if math.Abs(gotStddev-1) > stddevTol {
				t.Fatalf("normalized population stddev = %v, want within %v of 1 (input=%v)", gotStddev, stddevTol, values)
			}
		} else {
			// All values identical (stddev 0): divide-by-zero guard yields zeros.
			for i, z := range out {
				if z != 0 {
					t.Fatalf("all-identical input expected all zeros, got out[%d]=%v", i, z)
				}
			}
		}
	})

	// Dedicated all-identical property: continuous random draws almost never
	// collide, so explicitly construct identical-valued slices to exercise the
	// divide-by-zero (stddev 0) branch, which must return all zeros.
	rapid.Check(t, func(t *rapid.T) {
		v := rapid.Float64Range(-1e6, 1e6).Draw(t, "value")
		n := rapid.IntRange(1, 256).Draw(t, "n")

		values := make([]float64, n)
		for i := range values {
			values[i] = v
		}

		out := zScoreSlice(values)
		if len(out) != n {
			t.Fatalf("zScoreSlice length = %d, want %d", len(out), n)
		}
		for i, z := range out {
			if z != 0 {
				t.Fatalf("identical-value slice expected all zeros, got out[%d]=%v (value=%v, n=%d)", i, z, v, n)
			}
		}
	})
}
