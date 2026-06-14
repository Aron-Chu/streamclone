package heatmap

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: moment-timeline, Property 14: Chat Confidence Cap
//
// **Validates: Requirements 11.1**
//
// When stream-level chat coverage is below 35% AND the window has no chat
// rollup (ChatCount == 0), the chat-signal confidence must be capped at 0.3.
// Conversely, when either condition is false the cap does not apply.
func TestPropConfidenceChatCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 0.3499).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)
		rollup.ChatCount = 0

		conf := windowConfidence(rollup, cfg, coverage, true, false)

		if conf.Chat > chatConfidenceCap {
			t.Fatalf("chat confidence %v > cap %v with coverage=%v ChatCount=0",
				conf.Chat, chatConfidenceCap, coverage)
		}
		if conf.Chat != chatConfidenceCap {
			t.Fatalf("chat confidence %v != expected cap %v", conf.Chat, chatConfidenceCap)
		}
	})
}

// TestPropConfidenceChatNoCap verifies the chat confidence remains 1.0 when
// either chat coverage >= 35% or the window has chat data.
func TestPropConfidenceChatNoCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)

		highCoverage := rapid.Float64Range(0.35, 1.0).Draw(t, "highCoverage")
		rollup := drawConfMinuteRollup(t)
		rollup.ChatCount = rapid.IntRange(0, 1000).Draw(t, "chatCount")

		conf := windowConfidence(rollup, cfg, highCoverage, true, false)
		if conf.Chat != 1.0 {
			t.Fatalf("chat confidence %v != 1.0 with coverage=%v >= threshold",
				conf.Chat, highCoverage)
		}

		lowCoverage := rapid.Float64Range(0, 0.3499).Draw(t, "lowCoverage")
		rollup.ChatCount = rapid.IntRange(1, 1000).Draw(t, "positiveChatCount")
		conf2 := windowConfidence(rollup, cfg, lowCoverage, true, false)
		if conf2.Chat != 1.0 {
			t.Fatalf("chat confidence %v != 1.0 with ChatCount=%d > 0",
				conf2.Chat, rollup.ChatCount)
		}
	})
}

// Feature: moment-timeline, Property 15: Viewer Confidence Cap
//
// **Validates: Requirements 11.2**
//
// When a window has zero viewer samples, the viewer-signal confidence must be
// capped at 0.4.
func TestPropConfidenceViewerCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)
		rollup.ViewerSamples = 0

		conf := windowConfidence(rollup, cfg, coverage, true, false)

		if conf.Viewer > viewerConfidenceCap {
			t.Fatalf("viewer confidence %v > cap %v with ViewerSamples=0",
				conf.Viewer, viewerConfidenceCap)
		}
		if conf.Viewer != viewerConfidenceCap {
			t.Fatalf("viewer confidence %v != expected cap %v", conf.Viewer, viewerConfidenceCap)
		}
	})
}

// TestPropConfidenceViewerNoCap verifies the viewer confidence remains 1.0
// when the window has at least one viewer sample.
func TestPropConfidenceViewerNoCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)
		rollup.ViewerSamples = rapid.IntRange(1, 100).Draw(t, "viewerSamples")

		conf := windowConfidence(rollup, cfg, coverage, true, false)

		if conf.Viewer != 1.0 {
			t.Fatalf("viewer confidence %v != 1.0 with ViewerSamples=%d",
				conf.Viewer, rollup.ViewerSamples)
		}
	})
}

// Feature: moment-timeline, Property 16: Density Confidence Cap
//
// **Validates: Requirements 11.3**
//
// When window density is low (Missing==true), the density confidence must be
// capped at 0.5.
func TestPropConfidenceDensityCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)

		conf := windowConfidence(rollup, cfg, coverage, true, true)

		if conf.Density > densityConfidenceCap {
			t.Fatalf("density confidence %v > cap %v with low density",
				conf.Density, densityConfidenceCap)
		}
		if conf.Density != densityConfidenceCap {
			t.Fatalf("density confidence %v != expected cap %v", conf.Density, densityConfidenceCap)
		}
	})
}

// TestPropConfidenceDensityNoCap verifies the density confidence remains 1.0
// when density is not low.
func TestPropConfidenceDensityNoCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)

		conf := windowConfidence(rollup, cfg, coverage, true, false)

		if conf.Density != 1.0 {
			t.Fatalf("density confidence %v != 1.0 with normal density", conf.Density)
		}
	})
}

// Feature: moment-timeline, Property 17: Emote Dictionary Absent Zeroes Emote Signals
//
// **Validates: Requirements 11.4**
//
// When the emote dictionary is not loaded, the emote signal confidence must be
// exactly 0.0.
func TestPropConfidenceEmoteDictAbsent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)

		conf := windowConfidence(rollup, cfg, coverage, false, false)

		if conf.Emote != emoteAbsentConfidence {
			t.Fatalf("emote confidence %v != %v when dict not loaded",
				conf.Emote, emoteAbsentConfidence)
		}
	})
}

// TestPropConfidenceEmoteDictLoaded verifies the emote confidence is 1.0 when
// the emote dictionary is loaded.
func TestPropConfidenceEmoteDictLoaded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawConfScoringConfig(t)
		coverage := rapid.Float64Range(0, 1.0).Draw(t, "coverage")
		rollup := drawConfMinuteRollup(t)

		conf := windowConfidence(rollup, cfg, coverage, true, false)

		if conf.Emote != 1.0 {
			t.Fatalf("emote confidence %v != 1.0 when dict loaded", conf.Emote)
		}
	})
}

// --- Generators ---

func drawConfScoringConfig(t *rapid.T) ScoringConfig {
	cfg := DefaultScoringConfig()
	cfg.DensityConfidenceWeight = rapid.Float64Range(0.01, 1.0).Draw(t, "densityWeight")
	return cfg
}

func drawConfMinuteRollup(t *rapid.T) MinuteRollup {
	return MinuteRollup{
		ViewerAvg:         rapid.IntRange(0, 100000).Draw(t, "viewerAvg"),
		ViewerMax:         rapid.IntRange(0, 100000).Draw(t, "viewerMax"),
		ViewerLatest:      rapid.IntRange(0, 100000).Draw(t, "viewerLatest"),
		ViewerSamples:     rapid.IntRange(0, 60).Draw(t, "viewerSamples"),
		ChatCount:         rapid.IntRange(0, 5000).Draw(t, "chatCount"),
		TotalEmoteCount:   rapid.IntRange(0, 3000).Draw(t, "totalEmoteCount"),
		SevenTVEmoteCount: rapid.IntRange(0, 2000).Draw(t, "seventvEmoteCount"),
	}
}
