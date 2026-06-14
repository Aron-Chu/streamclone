package analytics

import (
	"testing"

	"pgregory.net/rapid"
)

// TestRapidToolingAvailable is a setup smoke test confirming the
// pgregory.net/rapid property-testing dependency is wired in and usable from
// the internal/analytics package. Moment-timeline property tests build on this.
//
// Feature: moment-timeline, Property 0: rapid tooling availability
func TestRapidToolingAvailable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 1_000_000).Draw(t, "n")
		if n+1 <= n {
			t.Fatalf("expected n+1 > n for non-negative n, got n=%d", n)
		}
	})
}
