package heatmap

import (
	"math"
	"time"
)

// defaultWindowSeconds is the scoring-window size in seconds. Rollups are
// per-minute, so each MinuteRollup maps to one 60-second scoring window
// (design: Signal Extraction). The replay-heatmap HTTP handler (task 11.5) may
// surface a different requested window via the `window` query parameter, but
// the pure scoring engine always works one window per consolidated rollup.
const defaultWindowSeconds = 60

type SignalComponent struct {
	RawScore      float64 `json:"rawScore"`
	WeightedScore float64 `json:"weightedScore"`
	Confidence    float64 `json:"confidence"`
}

type HeatmapEmote struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
	Count    int    `json:"count"`
	Provider string `json:"provider"`
}

type ReplayHeatmapPoint struct {
	OffsetSeconds   int            `json:"offsetSeconds"`
	DurationSeconds int            `json:"durationSeconds"`
	Score           int            `json:"score"`
	Confidence      float64        `json:"confidence"`
	Reason          string         `json:"reason"`
	TopEmotes       []HeatmapEmote `json:"topEmotes,omitempty"`
	VodID           *string        `json:"vodId,omitempty"`
	StreamID        string         `json:"streamId"`
	MinuteTs        time.Time      `json:"minuteTs"`
}

type ReplayHeatmapDetailPoint struct {
	ReplayHeatmapPoint
	Components map[string]SignalComponent `json:"components"`
}

type HeatmapResponse struct {
	StreamID       string               `json:"streamId"`
	WindowSeconds  int                  `json:"windowSeconds"`
	Confidence     float64              `json:"confidence"`
	ScoringVersion string               `json:"scoringVersion"`
	UpdatedAt      int64                `json:"updatedAt"`
	Points         []ReplayHeatmapPoint `json:"points"`
}

type HeatmapDetailResponse struct {
	StreamID       string                     `json:"streamId"`
	WindowSeconds  int                        `json:"windowSeconds"`
	Confidence     float64                    `json:"confidence"`
	ScoringVersion string                     `json:"scoringVersion"`
	UpdatedAt      int64                      `json:"updatedAt"`
	Points         []ReplayHeatmapDetailPoint `json:"points"`
}

// compositeScore combines the per-window normalized signal z-scores into a
// single 0–100 integer score using positive-surprise-only weighting
// (Requirement 9.1). Each signal's z-score is clamped to max(0, z) before
// weighting, so average (z≈0) and below-average windows contribute nothing and
// only above-average activity produces a non-zero score. This removes the
// "neutral = 50" ambiguity and lets missing/quiet windows naturally land at 0.
//
// When allMissing is true the function returns 0 immediately (Requirement 9.7):
// a window with no rollup data is scored 0, never interpolated from neighbors.
//
// The weighted raw value (max ~3.5 for extreme z-scores with weights summing to
// 1.0) is scaled by 30 and clamped to [0,100]. Accumulation walks the signal
// map by fixed, explicit keys in a constant order rather than ranging over the
// map, so the floating-point sum is order-independent and the result is fully
// deterministic across builds (Requirement 9.6).
func compositeScore(signals map[string]float64, weights SignalWeights, allMissing bool) int {
	if allMissing {
		return 0
	}

	raw := math.Max(0, signals[sigChatRate])*weights.ChatRate +
		math.Max(0, signals[sigEmoteRate])*weights.EmoteRate +
		math.Max(0, signals[sigViewerMomentum])*weights.ViewerMomentum +
		math.Max(0, signals[sigProviderSpike])*weights.ProviderSpike +
		math.Max(0, signals[sigTopEmoteDominance])*weights.TopEmoteDominance +
		math.Max(0, signals[sigNovelty])*weights.Novelty

	return int(math.Round(math.Min(100, math.Max(0, raw*30))))
}

// ComputeHeatmap is the pure, deterministic scoring engine entry point. It
// receives an already-consolidated, offset-sorted []MinuteRollup (one rollup per
// scoring window) and a ScoringConfig, and returns a HeatmapResponse with one
// ReplayHeatmapPoint per rollup. It performs no DB access, no analytics-package
// access, no randomness, and no wall-clock reads, so the same inputs always
// produce identical output (Requirement 9.6).
//
// Pipeline (design: ComputeHeatmap data flow):
//  1. extractSignals  — per-window log-transformed, z-score-normalized signals
//  2. compositeScore  — positive-surprise weighted 0–100 score per window
//  3. ewmaSmooth      — forward-only EWMA over the composite scores (causal)
//  4. suppressPeaks   — non-max suppression so one event yields one peak
//  5. build points    — assemble ReplayHeatmapPoint slice
//
// Missing windows (rollup.Missing) are forced to score 0 both before
// suppression (so they cannot bleed neighbor energy in through smoothing or act
// as a false peak) and in the final point (Requirement 9.7).
//
// Per-window Confidence and the stream-level Confidence are populated here
// (task 9.5): each point carries its overall window confidence and resp.Confidence
// is the median of those values (Requirements 11.6, 11.7). StreamID and UpdatedAt
// are intentionally left zero — they require request-scoped context (stream id,
// rollup updated_at) and are populated by the HTTP handler (task 11.5). Reason is
// selected per window from the signal
// z-scores (selectReason, Requirement 10.1/10.2); missing windows keep the
// chat_spike fallback. TopEmotes are attached per non-missing window from the
// rollup's emote counts (task 9.2). Finally, decimation (task 9.8, Requirement
// 12) is applied to resp.Points: zero-score windows are omitted, the top
// percentile is retained, and the remainder is uniformly sampled down to
// config.MaxPoints. resp.Confidence is computed over ALL windows before this
// step so omitting zero-score points does not skew it.
func ComputeHeatmap(rollups []MinuteRollup, config ScoringConfig) HeatmapResponse {
	resp := HeatmapResponse{
		WindowSeconds:  defaultWindowSeconds,
		ScoringVersion: config.Version,
		Points:         []ReplayHeatmapPoint{},
	}

	if len(rollups) == 0 {
		return resp
	}

	points, windowConfs := computeWindowPoints(rollups, config)

	// Stream-level confidence is the median of the per-window overall
	// confidences (Requirement 11.7). It is computed over ALL windows BEFORE
	// decimation so dropping zero-score points (below) does not skew it.
	resp.Confidence = streamConfidence(windowConfs)

	// Decimate to the configured point cap as the final step (Requirement 12):
	// zero-score points are omitted, the top percentile is always retained, and
	// the remainder is uniformly sampled down to MaxPoints.
	resp.Points = decimate(points, config.MaxPoints, config.TopRetainPercent)
	return resp
}

// computeWindowPoints runs the full scoring pipeline and returns one
// ReplayHeatmapPoint and one WindowConfidence per input rollup, in rollup
// (offset-ascending) order, WITHOUT decimation. It is the shared core used by
// both ComputeHeatmap and ComputeHeatmapDetail so the two responses score and
// rank windows identically before their respective decimation steps.
//
// Pipeline: extractSignals → compositeScore → ewmaSmooth → suppressPeaks →
// build points. Missing windows are forced to score 0 both before suppression
// (so smoothing carry-over cannot fabricate a peak) and in the final point
// (Requirement 9.7).
func computeWindowPoints(rollups []MinuteRollup, config ScoringConfig) ([]ReplayHeatmapPoint, []WindowConfidence) {
	n := len(rollups)
	normalized, raw := extractSignals(rollups)

	rawScores := make([]float64, n)
	for i := range rollups {
		rawScores[i] = float64(compositeScore(normalized[i], config.Weights, raw[i].missing))
	}

	smoothed := ewmaSmooth(rawScores, config.SmoothingSpan, config.SmoothingAlpha)

	// Force missing windows to 0 before suppression so smoothing carry-over
	// cannot turn a data-less window into a peak (Requirement 9.7).
	for i := range smoothed {
		if raw[i].missing {
			smoothed[i] = 0
		}
	}

	suppressed := suppressPeaks(smoothed, config.SuppressionThreshold, config.SuppressionRadius)

	chatCoverage := streamChatCoverage(rollups)
	dictLoaded := emoteDictLoadedFor(rollups)

	points := make([]ReplayHeatmapPoint, n)
	windowConfs := make([]WindowConfidence, n)
	for i := range rollups {
		score := int(math.Round(suppressed[i]))
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		if raw[i].missing {
			score = 0
		}

		reason := ReasonChatSpike
		var emotes []HeatmapEmote
		if !raw[i].missing {
			reason = selectReason(normalized[i], raw[i])
			// Attach the window's top 1–3 emotes by count (Requirement 10.3).
			// Missing windows have no rollup data, so TopEmotes stays nil.
			emotes = topEmotes(rollups[i].Emotes, maxTopEmotes)
		}

		conf := windowConfidence(rollups[i], config, chatCoverage, dictLoaded, windowDensityLow(rollups[i]))
		windowConfs[i] = conf

		points[i] = ReplayHeatmapPoint{
			OffsetSeconds:   i * defaultWindowSeconds,
			DurationSeconds: defaultWindowSeconds,
			Score:           score,
			Confidence:      conf.Overall,
			Reason:          reason,
			TopEmotes:       emotes,
			MinuteTs:        rollups[i].MinuteTS,
		}
	}

	return points, windowConfs
}

// ComputeHeatmapDetail is the detail-response variant of ComputeHeatmap. It
// returns the same scored points plus the per-signal component breakdown
// (components map) required by the `?detail=true` path of the heatmap HTTP
// handler (task 11.5, Requirements 28.2, 28.3). It is pure and deterministic for
// the same reasons as ComputeHeatmap and shares its scoring/confidence pipeline,
// so the base point fields (score, reason, confidence, top emotes) and the
// stream-level confidence are identical between the compact and detail
// responses.
//
// Each detail point embeds the compact ReplayHeatmapPoint and attaches a
// components map keyed by signal name; components for signals without data
// (missing window or zeroed per-signal confidence) are zeroed per Requirement
// 28.3.
func ComputeHeatmapDetail(rollups []MinuteRollup, config ScoringConfig) HeatmapDetailResponse {
	detail := HeatmapDetailResponse{
		WindowSeconds:  defaultWindowSeconds,
		ScoringVersion: config.Version,
		Points:         []ReplayHeatmapDetailPoint{},
	}

	n := len(rollups)
	if n == 0 {
		return detail
	}

	// Share the exact scoring pipeline with ComputeHeatmap so base point fields
	// and the stream-level confidence match the compact response.
	points, windowConfs := computeWindowPoints(rollups, config)
	detail.Confidence = streamConfidence(windowConfs)

	normalized, raw := extractSignals(rollups)

	// Build full (undecimated) detail points, one per rollup, in offset order.
	fullDetail := make([]ReplayHeatmapDetailPoint, n)
	for i := range rollups {
		fullDetail[i] = ReplayHeatmapDetailPoint{
			ReplayHeatmapPoint: points[i],
			Components:         signalComponents(normalized[i], windowConfs[i], config.Weights, raw[i].missing),
		}
	}

	// Apply the same decimation as the compact response, then keep only the
	// detail points whose embedded point survived (matched by OffsetSeconds,
	// which is unique per scoring window). fullDetail is already offset-sorted,
	// so iterating it preserves the offset-ascending order decimate produces.
	kept := decimate(points, config.MaxPoints, config.TopRetainPercent)
	keptOffsets := make(map[int]struct{}, len(kept))
	for _, p := range kept {
		keptOffsets[p.OffsetSeconds] = struct{}{}
	}

	out := make([]ReplayHeatmapDetailPoint, 0, len(kept))
	for i := range fullDetail {
		if _, ok := keptOffsets[fullDetail[i].OffsetSeconds]; ok {
			out = append(out, fullDetail[i])
		}
	}
	detail.Points = out
	return detail
}
