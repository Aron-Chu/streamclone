package heatmap

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestMissingWindowScoresZero is a property-based test for Property 11: a
// scoring window with no rollup data (MinuteRollup.Missing == true) is always
// scored 0 and is never interpolated from neighboring windows.
//
// The test asserts the property at the most robust level — through the exported
// ComputeHeatmap engine. ComputeHeatmap forces missing windows to score 0 both
// before and after non-max suppression (Requirement 9.7), and its final
// decimation step omits zero-score points (Requirement 12.3). Therefore a
// missing window at rollup index i (offset i*defaultWindowSeconds) must never
// surface in resp.Points: a missing window can neither produce a non-zero score
// itself nor borrow energy from its neighbors through smoothing/suppression.
//
// The generator interleaves missing windows with random data windows so that
// missing windows are surrounded by arbitrary (often very loud) neighbors,
// exercising the no-interpolation guarantee. Data windows are allowed to be
// loud, quiet, or zero — the property only constrains missing-window offsets,
// not which data windows survive decimation.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: moment-timeline, Property 11: Missing Window Scores Zero
//
// **Validates: Requirements 9.7**
func TestMissingWindowScoresZero(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := DefaultScoringConfig()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 200).Draw(t, "n")

		rollups := make([]MinuteRollup, n)
		missingOffsets := make(map[int]struct{})

		for i := 0; i < n; i++ {
			r := MinuteRollup{MinuteTS: base.Add(time.Duration(i) * time.Minute)}
			if rapid.Bool().Draw(t, "missing") {
				r.Missing = true
				missingOffsets[i*defaultWindowSeconds] = struct{}{}
			} else {
				// Arbitrary data window. Allow large counts so missing windows
				// are frequently sandwiched between very loud neighbors,
				// stressing the no-interpolation guarantee.
				r.ChatCount = rapid.IntRange(0, 5000).Draw(t, "chat")
				r.TotalEmoteCount = rapid.IntRange(0, 5000).Draw(t, "emote")
				r.SevenTVEmoteCount = rapid.IntRange(0, r.TotalEmoteCount).Draw(t, "seventv")
				r.ViewerAvg = rapid.IntRange(0, 100000).Draw(t, "viewerAvg")
				r.ViewerMax = r.ViewerAvg
				r.ViewerLatest = r.ViewerAvg
				r.ViewerSamples = rapid.IntRange(0, 60).Draw(t, "samples")
			}
			rollups[i] = r
		}

		resp := ComputeHeatmap(rollups, cfg)

		// No point in the response may correspond to a missing window's offset.
		// A surviving point at a missing offset would mean the window was
		// either scored non-zero or interpolated from neighbors — both violate
		// Requirement 9.7.
		for _, p := range resp.Points {
			if _, ok := missingOffsets[p.OffsetSeconds]; ok {
				t.Fatalf("missing window at offset %d surfaced in response with score %d (must be 0 and omitted)",
					p.OffsetSeconds, p.Score)
			}
		}
	})

	// Direct package-private check: compositeScore with allMissing == true must
	// return 0 regardless of the signal z-scores, confirming the engine scores a
	// fully-missing window 0 before any smoothing/suppression/decimation
	// (Requirement 9.7).
	rapid.Check(t, func(t *rapid.T) {
		signals := map[string]float64{
			sigChatRate:          rapid.Float64Range(-10, 10).Draw(t, "chat"),
			sigEmoteRate:         rapid.Float64Range(-10, 10).Draw(t, "emote"),
			sigViewerMomentum:    rapid.Float64Range(-10, 10).Draw(t, "viewer"),
			sigProviderSpike:     rapid.Float64Range(-10, 10).Draw(t, "provider"),
			sigTopEmoteDominance: rapid.Float64Range(-10, 10).Draw(t, "dominance"),
			sigNovelty:           rapid.Float64Range(-10, 10).Draw(t, "novelty"),
		}
		if got := compositeScore(signals, cfg.Weights, true); got != 0 {
			t.Fatalf("compositeScore(allMissing=true) = %d, want 0 (signals=%v)", got, signals)
		}
	})
}
