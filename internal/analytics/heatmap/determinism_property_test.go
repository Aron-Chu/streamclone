package heatmap

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genEmotes draws an emote map keyed by "provider:id:name" with multiple
// distinct keys so the test exercises map-iteration-order independence in
// top-emote ordering and signal accumulation. Go randomizes map iteration
// order between range loops within a single process, so two ComputeHeatmap
// calls over the same (deeply equal) rollups can iterate any contained map in
// different orders; a deterministic engine must still produce byte-identical
// output. Up to 6 keys are generated to make ties and ordering collisions
// likely.
func genEmotes(t *rapid.T, label string) map[string]int {
	providers := []string{providerSevenTV, providerTwitch, providerFFZ, "bttv", ""}
	n := rapid.IntRange(0, 6).Draw(t, label+"_emoteCount")
	if n == 0 {
		// Mix nil and empty maps across draws; both must omit TopEmotes.
		if rapid.Bool().Draw(t, label+"_nilEmotes") {
			return nil
		}
		return map[string]int{}
	}
	m := make(map[string]int, n)
	for i := 0; i < n; i++ {
		provider := rapid.SampledFrom(providers).Draw(t, label+"_provider")
		id := rapid.StringMatching(`[a-z0-9]{1,8}`).Draw(t, label+"_id")
		name := rapid.StringMatching(`[A-Za-z]{1,8}`).Draw(t, label+"_name")
		key := provider + ":" + id + ":" + name
		// Counts deliberately overlap a small range so multiple emotes tie,
		// forcing the deterministic tie-break (key ascending) to matter.
		m[key] = rapid.IntRange(1, 5).Draw(t, label+"_count")
	}
	return m
}

// genRollups draws an offset-sorted slice of arbitrary MinuteRollups with
// varied numeric fields, Missing flags, and multi-key Emotes maps. Minute
// timestamps are strictly increasing by one minute so the slice mirrors the
// consolidated, offset-sorted contract ComputeHeatmap expects.
func genRollups(t *rapid.T) []MinuteRollup {
	n := rapid.IntRange(0, 80).Draw(t, "rollupCount")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rollups := make([]MinuteRollup, n)
	for i := 0; i < n; i++ {
		missing := rapid.Float64Range(0, 1).Draw(t, "missingRoll") < 0.2
		r := MinuteRollup{
			MinuteTS: base.Add(time.Duration(i) * time.Minute),
			Missing:  missing,
		}
		if !missing {
			r.ViewerAvg = rapid.IntRange(0, 100000).Draw(t, "viewerAvg")
			r.ViewerMax = rapid.IntRange(0, 100000).Draw(t, "viewerMax")
			r.ViewerLatest = rapid.IntRange(0, 100000).Draw(t, "viewerLatest")
			r.ViewerSamples = rapid.IntRange(0, 60).Draw(t, "viewerSamples")
			r.ChatCount = rapid.IntRange(0, 5000).Draw(t, "chatCount")
			r.TotalEmoteCount = rapid.IntRange(0, 5000).Draw(t, "totalEmoteCount")
			r.SevenTVEmoteCount = rapid.IntRange(0, 5000).Draw(t, "seventvEmoteCount")
			r.Emotes = genEmotes(t, "roll")
		}
		rollups[i] = r
	}
	return rollups
}

// assertPointsIdentical fails the test if the two scored points are not
// bit-for-bit identical across every field, including the TopEmotes slice in
// order. This guards against map-iteration-order nondeterminism in score
// accumulation, reason selection, and top-emote ordering (Requirement 9.6).
func assertPointsIdentical(t *rapid.T, idx int, a, b ReplayHeatmapPoint) {
	if a.Score != b.Score {
		t.Fatalf("point %d Score differs: %d vs %d", idx, a.Score, b.Score)
	}
	if a.OffsetSeconds != b.OffsetSeconds {
		t.Fatalf("point %d OffsetSeconds differs: %d vs %d", idx, a.OffsetSeconds, b.OffsetSeconds)
	}
	if a.DurationSeconds != b.DurationSeconds {
		t.Fatalf("point %d DurationSeconds differs: %d vs %d", idx, a.DurationSeconds, b.DurationSeconds)
	}
	if a.Reason != b.Reason {
		t.Fatalf("point %d Reason differs: %q vs %q", idx, a.Reason, b.Reason)
	}
	if a.Confidence != b.Confidence {
		t.Fatalf("point %d Confidence differs: %v vs %v", idx, a.Confidence, b.Confidence)
	}
	if !a.MinuteTs.Equal(b.MinuteTs) {
		t.Fatalf("point %d MinuteTs differs: %v vs %v", idx, a.MinuteTs, b.MinuteTs)
	}
	if len(a.TopEmotes) != len(b.TopEmotes) {
		t.Fatalf("point %d TopEmotes length differs: %d vs %d", idx, len(a.TopEmotes), len(b.TopEmotes))
	}
	for j := range a.TopEmotes {
		ea, eb := a.TopEmotes[j], b.TopEmotes[j]
		if ea.ID != eb.ID || ea.Name != eb.Name || ea.ImageURL != eb.ImageURL ||
			ea.Count != eb.Count || ea.Provider != eb.Provider {
			t.Fatalf("point %d TopEmotes[%d] differs: %+v vs %+v", idx, j, ea, eb)
		}
	}
}

// assertResponsesIdentical fails unless the two HeatmapResponses are bit-for-bit
// identical: same stream-level Confidence, same point count, and identical
// fields for every point in order.
func assertResponsesIdentical(t *rapid.T, a, b HeatmapResponse) {
	if a.Confidence != b.Confidence {
		t.Fatalf("resp.Confidence differs: %v vs %v", a.Confidence, b.Confidence)
	}
	if len(a.Points) != len(b.Points) {
		t.Fatalf("point count differs: %d vs %d", len(a.Points), len(b.Points))
	}
	for i := range a.Points {
		assertPointsIdentical(t, i, a.Points[i], b.Points[i])
	}
}

// TestScoreDeterminism is a property-based test asserting that ComputeHeatmap is
// fully deterministic: computing the same consolidated rollups twice yields
// bit-for-bit identical scores, ordering, reasons, confidence, and top-emote
// lists. This is trust-critical (Requirement 9.6): the engine must never let
// Go's randomized map iteration order leak into score accumulation, reason
// selection, or top-emote ordering. Generators deliberately build multi-key
// Emotes maps with overlapping counts and a mix of Missing windows to stress
// every map-iteration path. Both the v1 DefaultScoringConfig and a second fixed
// config are exercised. rapid runs at least 100 iterations by default.
//
// Feature: moment-timeline, Property 10: Score Determinism
//
// **Validates: Requirements 9.6**
func TestScoreDeterminism(t *testing.T) {
	defaultCfg := DefaultScoringConfig()

	// A second fixed, valid config (weights still sum to 1.0) with different
	// smoothing/suppression/decimation knobs, so determinism is verified across
	// more than one config shape.
	altCfg := ScoringConfig{
		Version: "v1-alt",
		Weights: SignalWeights{
			ChatRate:          0.30,
			EmoteRate:         0.15,
			ViewerMomentum:    0.25,
			ProviderSpike:     0.10,
			TopEmoteDominance: 0.10,
			Novelty:           0.10,
		},
		DensityConfidenceWeight: 0.25,
		SmoothingSpan:           5,
		SmoothingAlpha:          0.4,
		SuppressionThreshold:    15,
		SuppressionRadius:       2,
		MaxPoints:               720,
		TopRetainPercent:        0.30,
	}
	if err := altCfg.Validate(); err != nil {
		t.Fatalf("altCfg invalid: %v", err)
	}

	rapid.Check(t, func(t *rapid.T) {
		rollups := genRollups(t)

		for _, cfg := range []ScoringConfig{defaultCfg, altCfg} {
			first := ComputeHeatmap(rollups, cfg)
			second := ComputeHeatmap(rollups, cfg)
			assertResponsesIdentical(t, first, second)
		}
	})
}
