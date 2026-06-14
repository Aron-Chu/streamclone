package heatmap

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// drawValidWeights draws a Dirichlet-style normalized SignalWeights vector: six
// non-negative weights normalized to sum to exactly 1.0 (within floating-point
// error, far inside the ScoringConfig.Validate ±0.001 tolerance). When the draw
// is the degenerate all-zero vector (which cannot be normalized) it falls back
// to the v1 default weights so the returned config is always valid.
func drawValidWeights(t *rapid.T) SignalWeights {
	raw := make([]float64, 6)
	var sum float64
	for i := range raw {
		raw[i] = rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("w%d", i))
		sum += raw[i]
	}
	if sum == 0 {
		return DefaultScoringConfig().Weights
	}
	for i := range raw {
		raw[i] /= sum
	}
	return SignalWeights{
		ChatRate:          raw[0],
		EmoteRate:         raw[1],
		ViewerMomentum:    raw[2],
		ProviderSpike:     raw[3],
		TopEmoteDominance: raw[4],
		Novelty:           raw[5],
	}
}

// drawScoringConfig picks a valid ScoringConfig for the property: either the v1
// default, one of two hand-built valid configs (alternate but valid weight
// splits / smoothing knobs), or a Dirichlet-normalized random-weight config.
// All branches keep the score weights summing to 1.0 and the remaining engine
// knobs in their valid ranges, so ScoringConfig.Validate always passes.
func drawScoringConfig(t *rapid.T) ScoringConfig {
	choice := rapid.IntRange(0, 3).Draw(t, "configChoice")
	switch choice {
	case 0:
		return DefaultScoringConfig()
	case 1:
		// Hand-built valid config A: chat-heavy weighting, tighter smoothing.
		cfg := DefaultScoringConfig()
		cfg.Weights = SignalWeights{
			ChatRate:          0.50,
			EmoteRate:         0.10,
			ViewerMomentum:    0.10,
			ProviderSpike:     0.10,
			TopEmoteDominance: 0.10,
			Novelty:           0.10,
		}
		cfg.SmoothingAlpha = 0.8
		cfg.SmoothingSpan = 2
		cfg.SuppressionRadius = 1
		return cfg
	case 2:
		// Hand-built valid config B: even split across all six signals.
		cfg := DefaultScoringConfig()
		even := 1.0 / 6.0
		cfg.Weights = SignalWeights{
			ChatRate:          even,
			EmoteRate:         even,
			ViewerMomentum:    even,
			ProviderSpike:     even,
			TopEmoteDominance: even,
			Novelty:           even,
		}
		cfg.SuppressionThreshold = 40
		cfg.SuppressionRadius = 5
		return cfg
	default:
		cfg := DefaultScoringConfig()
		cfg.Weights = drawValidWeights(t)
		return cfg
	}
}

// drawEmotes generates an arbitrary per-window emote-count map keyed by the
// engine's "provider:id:name" convention, mixing 7TV, Twitch-native, FFZ, and
// no-provider keys with positive counts. May return an empty (or nil) map.
func drawEmotes(t *rapid.T, label string) map[string]int {
	n := rapid.IntRange(0, 6).Draw(t, label+".emoteCount")
	if n == 0 {
		return nil
	}
	providers := []string{providerSevenTV, providerTwitch, providerFFZ, ""}
	emotes := make(map[string]int, n)
	for i := 0; i < n; i++ {
		p := providers[rapid.IntRange(0, len(providers)-1).Draw(t, fmt.Sprintf("%s.provider%d", label, i))]
		count := rapid.IntRange(1, 5000).Draw(t, fmt.Sprintf("%s.count%d", label, i))
		var key string
		if p == "" {
			key = fmt.Sprintf("noprovider:%d:e%d", i, i)
		} else {
			key = fmt.Sprintf("%s:%d:e%d", p, i, i)
		}
		emotes[key] = count
	}
	return emotes
}

// drawRollups generates an arbitrary, offset-sorted MinuteRollup slice with
// varied chat/emote/viewer activity, varied emote maps, and an occasional
// missing window — mirroring the consolidated input the engine receives.
func drawRollups(t *rapid.T) []MinuteRollup {
	n := rapid.IntRange(1, 200).Draw(t, "rollupCount")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rollups := make([]MinuteRollup, n)
	for i := 0; i < n; i++ {
		missing := rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("missing%d", i)) < 0.15
		r := MinuteRollup{
			MinuteTS: base.Add(time.Duration(i) * time.Minute),
			Missing:  missing,
		}
		if !missing {
			r.ChatCount = rapid.IntRange(0, 50000).Draw(t, fmt.Sprintf("chat%d", i))
			r.TotalEmoteCount = rapid.IntRange(0, 50000).Draw(t, fmt.Sprintf("emote%d", i))
			r.SevenTVEmoteCount = rapid.IntRange(0, r.TotalEmoteCount+1).Draw(t, fmt.Sprintf("seventv%d", i))
			r.ViewerAvg = rapid.IntRange(0, 500000).Draw(t, fmt.Sprintf("viewerAvg%d", i))
			r.ViewerMax = r.ViewerAvg
			r.ViewerLatest = r.ViewerAvg
			r.ViewerSamples = rapid.IntRange(0, 60).Draw(t, fmt.Sprintf("viewerSamples%d", i))
			r.Emotes = drawEmotes(t, fmt.Sprintf("r%d", i))
		}
		rollups[i] = r
	}
	return rollups
}

// TestScoreOutputRange is a property-based test for the score output range
// (Property 5). For any arbitrary consolidated rollup slice and any valid
// ScoringConfig, every ReplayHeatmapPoint.Score produced by ComputeHeatmap must
// be an integer in [0,100]. Score is typed as int, so integrality is structural;
// the property asserts the [0,100] bound holds across the full pipeline
// (normalization → composite → smoothing → suppression → decimation) for varied
// inputs and varied valid weight sets.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: moment-timeline, Property 5: Score Output Range and Weight Validity
//
// **Validates: Requirements 9.1**
func TestScoreOutputRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := drawScoringConfig(t)

		// Every config the property feeds the engine must itself be valid
		// (weights sum to 1.0 ±0.001); guard the generator here so an invalid
		// draw is reported as a generator bug rather than a silent pass.
		if err := cfg.Validate(); err != nil {
			t.Fatalf("generated config is invalid: %v (weights=%+v)", err, cfg.Weights)
		}

		rollups := drawRollups(t)

		resp := ComputeHeatmap(rollups, cfg)

		for i, p := range resp.Points {
			if p.Score < 0 || p.Score > 100 {
				t.Fatalf("point %d (offset %d) score %d out of [0,100] (weights=%+v)",
					i, p.OffsetSeconds, p.Score, cfg.Weights)
			}
		}
	})
}
