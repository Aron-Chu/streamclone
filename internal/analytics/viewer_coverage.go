package analytics

import (
	"strings"
	"time"
)

const liveCollectorRecentWindow = 48 * time.Hour

// hasGoodViewerCoverageFromRollups mirrors frontend analyzeViewerCoverage heuristics
// using persisted minute rollups. Returns true when viewer data looks complete enough
// to skip a TwitchTracker rescrape during chat-only resync.
func hasGoodViewerCoverageFromRollups(rollups []MinuteRollup, stream *StreamRecord) bool {
	if stream == nil || stream.ViewerSamples <= 0 {
		return false
	}

	type indexedPoint struct {
		idx   int
		value int
	}
	indexed := make([]indexedPoint, 0, len(rollups))
	for i, rollup := range rollups {
		if rollup.Missing {
			continue
		}
		val := rollup.ViewerAvg
		if val == 0 {
			val = rollup.ViewerLatest
		}
		if val == 0 {
			val = rollup.ViewerMax
		}
		if val <= 0 {
			continue
		}
		indexed = append(indexed, indexedPoint{idx: i, value: val})
	}
	if len(indexed) < 3 {
		return false
	}

	values := make([]int, len(indexed))
	for i, pt := range indexed {
		values[i] = pt.value
	}
	minV, maxV := values[0], values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if minV == maxV {
		return false
	}

	tailCount := max(4, len(indexed)*4/10)
	headCount := max(4, len(indexed)*2/10)
	if len(indexed) >= 12 {
		tailValues := values[len(values)-tailCount:]
		headValues := values[:headCount]
		tailFlat := len(tailValues) >= 4 && minSlice(tailValues) == maxSlice(tailValues)
		headVaried := len(headValues) >= 4 && minSlice(headValues) != maxSlice(headValues)
		if tailFlat && headVaried {
			return false
		}
	}

	if len(rollups) >= 10 {
		spanMinutes := indexed[len(indexed)-1].idx - indexed[0].idx + 1
		if float64(spanMinutes)/float64(len(rollups)) < 0.7 {
			return false
		}
	}

	durationSeconds := streamDurationSeconds(stream, rollups)
	points := rollupsToViewerPoints(rollups)
	return hasCompleteViewerChart(points, durationSeconds)
}

func streamDurationSeconds(stream *StreamRecord, rollups []MinuteRollup) int {
	if stream != nil && stream.EndedAt != nil && !stream.StartedAt.IsZero() {
		if d := int(stream.EndedAt.Sub(stream.StartedAt).Seconds()); d > 0 {
			return d
		}
	}
	if len(rollups) >= 2 {
		first := rollups[0].MinuteTS
		last := rollups[len(rollups)-1].MinuteTS
		if d := int(last.Sub(first).Seconds()) + 60; d > 0 {
			return d
		}
	}
	return 0
}

func rollupsToViewerPoints(rollups []MinuteRollup) []parsedViewerPoint {
	var chartStart time.Time
	points := make([]parsedViewerPoint, 0, len(rollups))
	for _, rollup := range rollups {
		if rollup.ViewerSamples == 0 && rollup.ViewerAvg == 0 && rollup.ViewerMax == 0 && rollup.ViewerLatest == 0 {
			continue
		}
		if chartStart.IsZero() {
			chartStart = rollup.MinuteTS.UTC()
		}
		val := rollup.ViewerAvg
		if val == 0 {
			val = rollup.ViewerLatest
		}
		if val == 0 {
			val = rollup.ViewerMax
		}
		offsetSec := int(rollup.MinuteTS.UTC().Sub(chartStart).Seconds())
		points = append(points, parsedViewerPoint{OffsetSeconds: offsetSec, Viewers: val})
	}
	return points
}

func minSlice(values []int) int {
	minV := values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
	}
	return minV
}

func maxSlice(values []int) int {
	maxV := values[0]
	for _, v := range values[1:] {
		if v > maxV {
			maxV = v
		}
	}
	return maxV
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// hasAnyViewerRollups returns true when persisted rollups include viewer minute data.
func hasAnyViewerRollups(rollups []MinuteRollup) bool {
	for _, rollup := range rollups {
		if rollup.Missing {
			continue
		}
		if rollup.ViewerSamples > 0 || rollup.ViewerAvg > 0 || rollup.ViewerMax > 0 || rollup.ViewerLatest > 0 {
			return true
		}
	}
	return false
}

// hasLiveCollectorViewerCoverage is true when the IRC/Helix live collector already
// produced a usable viewer timeline (recent session with enough samples).
func hasLiveCollectorViewerCoverage(stream *StreamRecord, rollups []MinuteRollup) bool {
	if stream == nil || stream.ViewerSamples < 3 {
		return false
	}
	if !stream.LastSeenAt.IsZero() && time.Since(stream.LastSeenAt) > liveCollectorRecentWindow {
		return false
	}
	return hasGoodViewerCoverageFromRollups(rollups, stream)
}

// inferViewerSource reports a lightweight hint for stream detail.
func inferViewerSource(stream *StreamRecord, rollups []MinuteRollup) string {
	if stream != nil {
		if source := normalizeViewerSource(stream.ViewerSource); source != ViewerSourceUnknown {
			return source
		}
	}
	liveMinutes, ttMinutes := countViewerSourceMinutes(rollups)
	hasLive := liveMinutes > 0 || hasLiveCollectorViewerCoverage(stream, rollups)
	hasTT := ttMinutes > 0 || hasGoodViewerCoverageFromRollups(rollups, stream)
	if hasLive && hasTT && liveMinutes > 0 && ttMinutes > 0 {
		return "merged"
	}
	if hasLiveCollectorViewerCoverage(stream, rollups) {
		return "live"
	}
	if hasGoodViewerCoverageFromRollups(rollups, stream) {
		return "tt"
	}
	if hasAnyViewerRollups(rollups) {
		return "partial"
	}
	if stream != nil && strings.TrimSpace(stream.VodSource) != "" {
		return "partial"
	}
	return ""
}

func persistedViewerSource(stream *StreamRecord, rollups []MinuteRollup) string {
	if stream != nil {
		if source := normalizeViewerSource(stream.ViewerSource); source != ViewerSourceUnknown {
			return source
		}
	}
	return inferViewerSource(stream, rollups)
}
