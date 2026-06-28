package heatmap

import (
	"regexp"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestPropSchemaConformance is a property-based test for heatmap response schema
// conformance. It validates that ComputeHeatmap output always conforms to the
// ReplayHeatmapPoint contract for any valid rollup input.
//
// Feature: moment-timeline, Property 25: Heatmap Response Schema Conformance
//
// **Validates: Requirements 8.1, 28.1, 28.2, 28.3, 28.4**
func TestPropSchemaConformance(t *testing.T) {
	emoteURLPattern := regexp.MustCompile(`^(/emotes/[a-zA-Z0-9_-]+/1x\.webp|https://static-cdn\.jtvnw\.net/emoticons/v2/[^/]+/default/dark/2\.0|https://cdn\.7tv\.app/emote/[^/]+/4x\.webp|https://cdn\.frankerfacez\.com/emoticon/[^/]+/4|https://cdn\.betterttv\.net/emote/[^/]+/3x)$`)

	genEmoteKey := rapid.Custom(func(t *rapid.T) string {
		provider := rapid.SampledFrom([]string{"seventv", "twitch", "ffz", "bttv"}).Draw(t, "provider")
		id := rapid.StringMatching(`[a-f0-9]{8,24}`).Draw(t, "id")
		name := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_]{1,15}`).Draw(t, "name")
		return provider + ":" + id + ":" + name
	})

	genRollup := rapid.Custom(func(t *rapid.T) MinuteRollup {
		missing := rapid.Float64Range(0, 1).Draw(t, "missingChance") < 0.1 // ~10% missing
		if missing {
			return MinuteRollup{
				MinuteTS: time.Unix(rapid.Int64Range(1700000000, 1800000000).Draw(t, "ts"), 0),
				Missing:  true,
			}
		}

		numEmotes := rapid.IntRange(0, 8).Draw(t, "numEmotes")
		emotes := make(map[string]int, numEmotes)
		totalEmote := 0
		seventvCount := 0
		for i := 0; i < numEmotes; i++ {
			key := genEmoteKey.Draw(t, "emoteKey")
			count := rapid.IntRange(1, 200).Draw(t, "emoteCount")
			emotes[key] = count
			totalEmote += count
			if len(key) > 7 && key[:7] == "seventv" {
				seventvCount += count
			}
		}

		return MinuteRollup{
			MinuteTS:          time.Unix(rapid.Int64Range(1700000000, 1800000000).Draw(t, "ts"), 0),
			ViewerAvg:         rapid.IntRange(0, 100000).Draw(t, "viewerAvg"),
			ViewerMax:         rapid.IntRange(0, 100000).Draw(t, "viewerMax"),
			ViewerLatest:      rapid.IntRange(0, 100000).Draw(t, "viewerLatest"),
			ViewerSamples:     rapid.IntRange(0, 10).Draw(t, "viewerSamples"),
			ChatCount:         rapid.IntRange(0, 5000).Draw(t, "chatCount"),
			TotalEmoteCount:   totalEmote,
			SevenTVEmoteCount: seventvCount,
			Emotes:            emotes,
			Missing:           false,
		}
	})

	t.Run("envelope_fields", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			// ScoringVersion must be non-empty (Requirement 28.4)
			if resp.ScoringVersion == "" {
				t.Fatal("ScoringVersion is empty")
			}

			// WindowSeconds must equal the default window
			if resp.WindowSeconds != defaultWindowSeconds {
				t.Fatalf("WindowSeconds = %d, want %d", resp.WindowSeconds, defaultWindowSeconds)
			}

			// Overall confidence must be in [0,1] (Requirement 28.2)
			if resp.Confidence < 0 || resp.Confidence > 1 {
				t.Fatalf("stream Confidence = %f, want [0,1]", resp.Confidence)
			}

			// Points must be non-nil (may be empty slice)
			if resp.Points == nil {
				t.Fatal("Points is nil, expected non-nil slice")
			}
		})
	})

	t.Run("point_score_range", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if pt.Score < 0 || pt.Score > 100 {
					t.Fatalf("point[%d].Score = %d, want [0,100]", i, pt.Score)
				}
			}
		})
	})

	t.Run("point_confidence_range", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if pt.Confidence < 0 || pt.Confidence > 1 {
					t.Fatalf("point[%d].Confidence = %f, want [0,1]", i, pt.Confidence)
				}
			}
		})
	})

	t.Run("point_valid_reason", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if !IsValidReason(pt.Reason) {
					t.Fatalf("point[%d].Reason = %q, not a valid reason label", i, pt.Reason)
				}
			}
		})
	})

	t.Run("point_duration_positive", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if pt.DurationSeconds <= 0 {
					t.Fatalf("point[%d].DurationSeconds = %d, want > 0", i, pt.DurationSeconds)
				}
			}
		})
	})

	t.Run("point_offset_non_negative", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if pt.OffsetSeconds < 0 {
					t.Fatalf("point[%d].OffsetSeconds = %d, want >= 0", i, pt.OffsetSeconds)
				}
			}
		})
	})

	t.Run("points_sorted_by_offset", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(2, 50).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i := 1; i < len(resp.Points); i++ {
				if resp.Points[i].OffsetSeconds < resp.Points[i-1].OffsetSeconds {
					t.Fatalf("points not sorted by OffsetSeconds: [%d]=%d > [%d]=%d",
						i-1, resp.Points[i-1].OffsetSeconds, i, resp.Points[i].OffsetSeconds)
				}
			}
		})
	})

	t.Run("top_emotes_format", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(3, 30).Draw(t, "numRollups")
			rollups := make([]MinuteRollup, n)
			for i := range rollups {
				rollups[i] = genRollup.Draw(t, "rollup")
			}

			cfg := DefaultScoringConfig()
			resp := ComputeHeatmap(rollups, cfg)

			for i, pt := range resp.Points {
				if len(pt.TopEmotes) > 3 {
					t.Fatalf("point[%d].TopEmotes has %d entries, want 0-3", i, len(pt.TopEmotes))
				}
				for j, emote := range pt.TopEmotes {
					if emote.ID == "" {
						t.Fatalf("point[%d].TopEmotes[%d].ID is empty", i, j)
					}
					if !emoteURLPattern.MatchString(emote.ImageURL) {
						t.Fatalf("point[%d].TopEmotes[%d].ImageURL = %q, want resolved emote CDN/local URL", i, j, emote.ImageURL)
					}
				}
			}
		})
	})
}
