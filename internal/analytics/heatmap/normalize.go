package heatmap

import (
	"math"
	"strings"
	"time"
)

// MinuteRollup is the pure input contract for the scoring engine. It mirrors the
// fields the engine needs from analytics.MinuteRollup so that the heatmap package
// stays pure and never imports the analytics package. The rollup-consolidation
// bridge in package analytics (task 11.4) copies fields into this struct before
// calling ComputeHeatmap. Downstream tasks (composite scoring, reason labels,
// confidence) reuse this type.
//
// Field semantics match analytics.MinuteRollup:
//   - Emotes is keyed by "provider:id:name" (see analytics.splitEmoteKey); the
//     provider prefix is the substring before the first ':'.
//   - SevenTVEmoteCount is the dedicated 7TV use count for the window.
//   - Missing marks a window that has no rollup data; the score for such a
//     window is forced to 0 (Requirement 9.7) rather than interpolated.
type MinuteRollup struct {
	MinuteTS          time.Time
	ViewerAvg         int
	ViewerMax         int
	ViewerLatest      int
	ViewerSamples     int
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
	Missing           bool
}

// Signal component keys. These names are the canonical map keys used by the
// composite score, reason-label selection, and the per-signal component
// breakdown surfaced in the detail response.
const (
	sigChatRate          = "chatRate"
	sigEmoteRate         = "emoteRate"
	sigViewerMomentum    = "viewerMomentum"
	sigProviderSpike     = "providerSpike"
	sigTopEmoteDominance = "topEmoteDominance"
	sigNovelty           = "novelty"
)

// Emote key provider prefixes (see analytics.splitEmoteKey: "provider:id:name").
const (
	providerSevenTV = "seventv"
	providerTwitch  = "twitch"
	providerFFZ     = "ffz"
)

// logTransform applies the natural-log transform ln(value+1) used on count
// signals before z-score normalization (Requirement 9.3). It is finite and
// non-negative for any value >= 0, maps 0 -> 0, and is monotonically
// increasing, so it preserves ordering between counts.
func logTransform(value float64) float64 {
	return math.Log(value + 1)
}

// mean returns the arithmetic mean of values, or 0 for an empty slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// stddev returns the population standard deviation of values, or 0 for an empty
// slice. Population (divide-by-N) is used so that the z-scores produced by
// zScore have a population standard deviation of exactly 1 for >=2 distinct
// values (Requirement 9.2, Property 6).
func stddev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	// Detect zero spread directly from the min/max range. When every value is
	// identical there is no variance, so the standard deviation is exactly 0 and
	// the divide-by-zero guard in zScore/zScoreSlice must trigger. Deriving this
	// from min==max is exact and deterministic, unlike a sum-of-squares path that
	// accumulates floating-point rounding: for many identical non-trivial values
	// (e.g. 1.0625 repeated) sequential summation can yield a tiny non-zero stddev
	// (~1e-16) that would defeat the guard and collapse each z-score to +/-1
	// instead of 0 (Requirement 9.2, Property 6).
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if min == max {
		return 0
	}
	// Compute the variance in a deviation space scaled by the largest absolute
	// deviation. The standard deviation is unchanged by this scaling
	// (sd = scale * sqrt(mean((d/scale)^2))) but the squared terms stay in
	// [0,1], avoiding subnormal underflow that destroys precision for tiny
	// near-subnormal inputs (e.g. [2e-161, 0]) and avoiding overflow for large
	// magnitudes. This keeps the >=2-distinct-values guarantee (mean~0, stddev~1)
	// holding to within 1e-9 across the full input range (Requirement 9.2).
	m := mean(values)
	scale := math.Max(math.Abs(max-m), math.Abs(min-m))
	if scale == 0 {
		return 0
	}
	var sumSq float64
	for _, v := range values {
		e := (v - m) / scale
		sumSq += e * e
	}
	return scale * math.Sqrt(sumSq/float64(len(values)))
}

// zScore returns the z-score of values[idx] against the distribution of values.
// When the standard deviation is 0 (all values identical, including a
// single-element or empty distribution) it returns 0 to avoid divide-by-zero
// (Requirement 9.2). idx out of range returns 0.
func zScore(values []float64, idx int) float64 {
	if idx < 0 || idx >= len(values) {
		return 0
	}
	sd := stddev(values)
	if sd == 0 {
		return 0
	}
	return (values[idx] - mean(values)) / sd
}

// zScoreSlice returns the per-stream z-score normalization of values. Divide by
// zero (stddev 0) yields all zeros. The returned slice has the same length as
// the input.
func zScoreSlice(values []float64) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	m := mean(values)
	sd := stddev(values)
	if sd == 0 {
		return out
	}
	for i, v := range values {
		out[i] = (v - m) / sd
	}
	return out
}

// rawWindowSignals holds the per-window signal values extracted from a single
// rollup before cross-window z-score normalization. Provider rates are kept
// separate so providerSpike can be computed as the max of the individually
// normalized provider rates (design: Signal Extraction).
type rawWindowSignals struct {
	chatRate          float64 // ln(chatCount+1)
	emoteRate         float64 // ln(totalEmoteCount+1)
	viewerMomentum    float64 // viewerAvg[t] - viewerAvg[t-1]
	sevenTVRate       float64 // ln(seventvEmoteCount+1)
	twitchRate        float64 // ln(twitch provider emote count+1)
	ffzRate           float64 // ln(ffz provider emote count+1)
	topEmoteDominance float64 // topEmoteCount / max(totalEmoteCount,1), in [0,1]
	novelty           float64 // 1 - (topEmoteCount if seen before)/max(total,1), in [0,1]
	missing           bool
}

// emoteKeyProvider returns the provider prefix of a rollup emote key
// ("provider:id:name"). Keys without a ':' are treated as having no provider.
func emoteKeyProvider(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return ""
}

// providerEmoteCounts sums emote uses per provider from the rollup Emotes map.
// 7TV is sourced from the dedicated SevenTVEmoteCount field rather than this
// map (design: Signal Extraction), so only Twitch-native and FFZ counts are
// derived here.
func providerEmoteCounts(emotes map[string]int) (twitch, ffz int) {
	for key, count := range emotes {
		switch emoteKeyProvider(key) {
		case providerTwitch:
			twitch += count
		case providerFFZ:
			ffz += count
		}
	}
	return twitch, ffz
}

// topEmoteCount returns the highest single-emote use count in the window, or 0
// when there are no emotes.
func topEmoteCount(emotes map[string]int) int {
	top := 0
	for _, count := range emotes {
		if count > top {
			top = count
		}
	}
	return top
}

// topEmoteKey returns the emote key with the highest use count in the window.
// Ties are broken by lexically smallest key so selection is deterministic
// (Requirement 9.6). Returns "" for an empty map.
func topEmoteKey(emotes map[string]int) string {
	best := ""
	bestCount := -1
	for key, count := range emotes {
		if count > bestCount || (count == bestCount && key < best) {
			best = key
			bestCount = count
		}
	}
	return best
}

// extractRawSignals derives the per-window raw signals for an already
// consolidated, offset-sorted rollup slice. Each rollup minute maps to one
// scoring window aligned to the rollup minute boundary (default 60s window).
// viewerMomentum and novelty are sequential (depend on prior windows), so this
// must run over the ordered slice. The returned slice has one entry per rollup.
func extractRawSignals(rollups []MinuteRollup) []rawWindowSignals {
	out := make([]rawWindowSignals, len(rollups))
	seenEmotes := make(map[string]struct{})
	prevViewerAvg := 0
	havePrevViewer := false

	for i, r := range rollups {
		var s rawWindowSignals
		s.missing = r.Missing

		s.chatRate = logTransform(float64(r.ChatCount))
		s.emoteRate = logTransform(float64(r.TotalEmoteCount))

		if havePrevViewer {
			s.viewerMomentum = float64(r.ViewerAvg - prevViewerAvg)
		} else {
			s.viewerMomentum = 0
		}

		twitch, ffz := providerEmoteCounts(r.Emotes)
		s.sevenTVRate = logTransform(float64(r.SevenTVEmoteCount))
		s.twitchRate = logTransform(float64(twitch))
		s.ffzRate = logTransform(float64(ffz))

		total := float64(r.TotalEmoteCount)
		denom := math.Max(total, 1)
		topKey := topEmoteKey(r.Emotes)
		topCount := 0
		if topKey != "" {
			topCount = r.Emotes[topKey]
		}
		s.topEmoteDominance = float64(topCount) / denom

		appearedBefore := 0.0
		if topKey != "" {
			if _, seen := seenEmotes[topKey]; seen {
				appearedBefore = float64(topCount)
			}
		}
		s.novelty = 1.0 - appearedBefore/denom

		for key := range r.Emotes {
			seenEmotes[key] = struct{}{}
		}

		// Only advance the viewer baseline from windows that carry viewer data,
		// so missing windows do not poison the momentum delta.
		if !r.Missing {
			prevViewerAvg = r.ViewerAvg
			havePrevViewer = true
		}

		out[i] = s
	}
	return out
}

// normalizeSignals converts raw per-window signals into per-window normalized
// signal maps keyed by signal-component name. Each signal channel is z-score
// normalized independently across the stream's own windows (Requirement 9.2).
// providerSpike is the max of the individually normalized 7TV / Twitch / FFZ
// rates for the window (design: Signal Extraction). The returned slice has one
// map per window, ready for composite scoring and reason-label selection.
func normalizeSignals(raw []rawWindowSignals) []map[string]float64 {
	n := len(raw)
	out := make([]map[string]float64, n)
	if n == 0 {
		return out
	}

	chat := make([]float64, n)
	emote := make([]float64, n)
	viewer := make([]float64, n)
	sevenTV := make([]float64, n)
	twitch := make([]float64, n)
	ffz := make([]float64, n)
	dominance := make([]float64, n)
	novelty := make([]float64, n)

	for i, s := range raw {
		chat[i] = s.chatRate
		emote[i] = s.emoteRate
		viewer[i] = s.viewerMomentum
		sevenTV[i] = s.sevenTVRate
		twitch[i] = s.twitchRate
		ffz[i] = s.ffzRate
		dominance[i] = s.topEmoteDominance
		novelty[i] = s.novelty
	}

	chatZ := zScoreSlice(chat)
	emoteZ := zScoreSlice(emote)
	viewerZ := zScoreSlice(viewer)
	sevenTVZ := zScoreSlice(sevenTV)
	twitchZ := zScoreSlice(twitch)
	ffzZ := zScoreSlice(ffz)
	dominanceZ := zScoreSlice(dominance)
	noveltyZ := zScoreSlice(novelty)

	for i := range raw {
		providerSpike := math.Max(sevenTVZ[i], math.Max(twitchZ[i], ffzZ[i]))
		out[i] = map[string]float64{
			sigChatRate:          chatZ[i],
			sigEmoteRate:         emoteZ[i],
			sigViewerMomentum:    viewerZ[i],
			sigProviderSpike:     providerSpike,
			sigTopEmoteDominance: dominanceZ[i],
			sigNovelty:           noveltyZ[i],
		}
	}
	return out
}

// extractSignals is the convenience pipeline used by ComputeHeatmap: it extracts
// raw per-window signals from the ordered rollup slice and returns the
// z-score-normalized per-window signal maps. The raw signals are returned
// alongside so callers (confidence, reason labels, missing-window detection) can
// inspect pre-normalization values such as the per-window missing flag.
func extractSignals(rollups []MinuteRollup) (normalized []map[string]float64, raw []rawWindowSignals) {
	raw = extractRawSignals(rollups)
	normalized = normalizeSignals(raw)
	return normalized, raw
}

// ewmaSmooth applies a forward-only exponentially weighted moving average to
// scores (Requirement 9.4). The first element is carried through unchanged and
// each subsequent element blends the current value with the running smoothed
// value: smoothed[i] = alpha*scores[i] + (1-alpha)*smoothed[i-1].
//
// The pass is strictly forward so the result is causal: smoothed[i] depends
// only on scores[0..i], never on later windows. A peak at minute 5 therefore
// cannot retroactively boost minute 3 (Property 8). Consequently, changing the
// score at index k leaves every smoothed[j] for j < k unchanged.
//
// span is accepted for interface and configuration parity with the smoothing
// parameters; the recurrence is governed by alpha. An empty input returns an
// empty (non-nil) slice.
func ewmaSmooth(scores []float64, span int, alpha float64) []float64 {
	_ = span
	smoothed := make([]float64, len(scores))
	if len(scores) == 0 {
		return smoothed
	}
	smoothed[0] = scores[0]
	for i := 1; i < len(scores); i++ {
		smoothed[i] = alpha*scores[i] + (1-alpha)*smoothed[i-1]
	}
	return smoothed
}
