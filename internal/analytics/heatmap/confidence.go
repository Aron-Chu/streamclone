package heatmap

import (
	"math"
	"sort"
)

// WindowConfidence holds the per-signal and overall confidence for a single
// scoring window (Requirement 11.5). The four per-signal dimensions (Chat,
// Viewer, Emote, Density) are each capped independently per Requirements
// 11.1–11.4, and Overall is the weighted average of the available (non-zero)
// per-signal confidences weighted by the scoring-config signal weights
// (Requirement 11.6), never a product. When no degradation applies all fields
// are 1.0 (Requirement 11.8).
//
// Density is a cross-cutting data-quality signal with no single score component,
// so it does not appear in the per-signal component breakdown surfaced in the
// detail response; it contributes to Overall via cfg.DensityConfidenceWeight.
type WindowConfidence struct {
	Chat    float64 `json:"chat"`
	Viewer  float64 `json:"viewer"`
	Emote   float64 `json:"emote"`
	Density float64 `json:"density"`
	Overall float64 `json:"overall"`
}

// Confidence caps and thresholds (Requirements 11.1–11.4).
const (
	chatCoverageThreshold = 0.35 // streams below this chat coverage cap chat confidence
	chatConfidenceCap     = 0.3  // Requirement 11.1
	viewerConfidenceCap   = 0.4  // Requirement 11.2
	densityConfidenceCap  = 0.5  // Requirement 11.3
	emoteAbsentConfidence  = 0.0 // Requirement 11.4
)

// windowConfidence computes the per-signal and overall confidence for one
// scoring window (design: Confidence Computation; Requirements 11.1–11.6, 11.8).
//
// Each per-signal confidence starts at 1.0 and is capped when a degradation
// condition applies:
//   - Chat → 0.3 when stream-level chat coverage is below 35% AND this window
//     has no chat rollup (ChatCount == 0) (Requirement 11.1).
//   - Viewer → 0.4 when the window has zero viewer samples (Requirement 11.2).
//   - Emote → 0.0 when the channel emote dictionary is not loaded
//     (Requirement 11.4); the 0.0 confidence excludes emote signals from the
//     overall average and zeroes their components in the detail response.
//   - Density → 0.5 when rollup density is low for this window (Requirement 11.3).
//
// Overall is the weighted average of the capped per-signal confidences, weighted
// by the scoring-config signal weights (Requirement 11.6), NOT a product. The
// emote dimension carries the combined weight of every emote/provider-derived
// score signal (EmoteRate + ProviderSpike + TopEmoteDominance + Novelty), and
// density carries the dedicated DensityConfidenceWeight. Signals whose capped
// confidence is exactly 0.0 are excluded from both the numerator and the
// denominator so a missing emote dictionary cannot erase strong chat/viewer
// confidence. The result is clamped to [0.0, 1.0]. When every signal is excluded
// (no weight remains) Overall is 0.0.
func windowConfidence(rollup MinuteRollup, cfg ScoringConfig, streamChatCoverage float64, emoteDictLoaded bool, rollupDensityLow bool) WindowConfidence {
	conf := WindowConfidence{Chat: 1.0, Viewer: 1.0, Emote: 1.0, Density: 1.0}

	if streamChatCoverage < chatCoverageThreshold && rollup.ChatCount == 0 {
		conf.Chat = chatConfidenceCap
	}
	if rollup.ViewerSamples == 0 {
		conf.Viewer = viewerConfidenceCap
	}
	if !emoteDictLoaded {
		conf.Emote = emoteAbsentConfidence
	}
	if rollupDensityLow {
		conf.Density = densityConfidenceCap
	}

	w := cfg.Weights
	type entry struct{ conf, weight float64 }
	signals := []entry{
		{conf.Chat, w.ChatRate},
		{conf.Viewer, w.ViewerMomentum},
		{conf.Emote, w.EmoteRate + w.ProviderSpike + w.TopEmoteDominance + w.Novelty},
		{conf.Density, cfg.DensityConfidenceWeight},
	}
	var sumW, sumCW float64
	for _, s := range signals {
		if s.conf > 0 {
			sumW += s.weight
			sumCW += s.conf * s.weight
		}
	}
	if sumW > 0 {
		conf.Overall = math.Min(1.0, math.Max(0.0, sumCW/sumW))
	} else {
		conf.Overall = 0.0
	}
	return conf
}

// streamChatCoverage returns the fraction of windows that carry a chat rollup
// (ChatCount > 0), the stream-level input compared against the 35% threshold in
// windowConfidence (Requirement 11.1). An empty slice yields 0.
func streamChatCoverage(rollups []MinuteRollup) float64 {
	if len(rollups) == 0 {
		return 0
	}
	withChat := 0
	for _, r := range rollups {
		if r.ChatCount > 0 {
			withChat++
		}
	}
	return float64(withChat) / float64(len(rollups))
}

// emoteDictLoadedFor infers whether the channel emote dictionary was loaded for
// the stream. The pure scoring engine has no channel-dictionary handle, so it
// treats the dictionary as loaded when any window carries emote activity
// (TotalEmoteCount, SevenTVEmoteCount, or any keyed emote). When no window has
// any emote data the dictionary is treated as absent, which zeroes the
// emote-signal confidence and emote score components (Requirement 11.4).
func emoteDictLoadedFor(rollups []MinuteRollup) bool {
	for _, r := range rollups {
		if r.TotalEmoteCount > 0 || r.SevenTVEmoteCount > 0 || len(r.Emotes) > 0 {
			return true
		}
	}
	return false
}

// windowDensityLow reports whether a window has low rollup density for the
// density-confidence cap (Requirement 11.3). For per-minute rollups one rollup
// maps to one scoring window, so a window falls below the "one rollup per two
// windows" density floor exactly when it is missing/sparse — i.e. it has no
// backing rollup row.
func windowDensityLow(r MinuteRollup) bool {
	return r.Missing
}

// streamConfidence returns the stream-level confidence as the median of the
// per-window overall confidence values (Requirement 11.7). For an even number of
// windows it averages the two middle values. An empty slice yields 0.0.
func streamConfidence(windows []WindowConfidence) float64 {
	n := len(windows)
	if n == 0 {
		return 0
	}
	vals := make([]float64, n)
	for i, w := range windows {
		vals[i] = w.Overall
	}
	sort.Float64s(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// signalComponents builds the per-signal component breakdown for one window for
// the detail response (Requirements 28.2, 28.3). It returns one entry per score
// signal (chatRate, emoteRate, viewerMomentum, providerSpike, topEmoteDominance,
// novelty), each carrying:
//   - RawScore: the un-clamped signal z-score (normalized[key]).
//   - WeightedScore: max(0, z) * weight, matching the positive-surprise-only
//     weighting compositeScore applies.
//   - Confidence: the per-signal confidence, mapped from WindowConfidence —
//     chatRate→Chat, viewerMomentum→Viewer, and every emote/provider signal
//     →Emote.
//
// A component is fully zeroed (RawScore 0, WeightedScore 0, Confidence 0.0) when
// the window is missing (no rollup data at all) or when the mapped per-signal
// confidence is 0.0 (e.g. the emote dictionary is not loaded), per Requirement
// 28.3.
func signalComponents(normalized map[string]float64, conf WindowConfidence, weights SignalWeights, missing bool) map[string]SignalComponent {
	defs := []struct {
		key    string
		weight float64
		conf   float64
	}{
		{sigChatRate, weights.ChatRate, conf.Chat},
		{sigEmoteRate, weights.EmoteRate, conf.Emote},
		{sigViewerMomentum, weights.ViewerMomentum, conf.Viewer},
		{sigProviderSpike, weights.ProviderSpike, conf.Emote},
		{sigTopEmoteDominance, weights.TopEmoteDominance, conf.Emote},
		{sigNovelty, weights.Novelty, conf.Emote},
	}
	out := make(map[string]SignalComponent, len(defs))
	for _, d := range defs {
		if missing || d.conf <= 0 {
			out[d.key] = SignalComponent{}
			continue
		}
		raw := normalized[d.key]
		out[d.key] = SignalComponent{
			RawScore:      raw,
			WeightedScore: math.Max(0, raw) * d.weight,
			Confidence:    d.conf,
		}
	}
	return out
}
