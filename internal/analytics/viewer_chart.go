package analytics

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"sort"
	"time"
)

// minTTChartPointsForDuration is the minimum point count for SVG/injected TT charts.
func minTTChartPointsForDuration(durationMinutes int) int {
	if durationMinutes <= 0 {
		return 10
	}
	minPoints := durationMinutes / 5
	if minPoints < 10 {
		minPoints = 10
	}
	return minPoints
}

func chartMeetsTTPointDensity(pointCount, durationMinutes int) bool {
	if pointCount < 3 {
		return false
	}
	return pointCount >= minTTChartPointsForDuration(durationMinutes)
}

func applyTTViewerMedianSmooth(points []parsedViewerPoint, window int) []parsedViewerPoint {
	if window <= 1 || len(points) < 3 {
		return points
	}
	out := make([]parsedViewerPoint, len(points))
	copy(out, points)
	half := window / 2
	for i := range out {
		start := i - half
		if start < 0 {
			start = 0
		}
		end := i + half + 1
		if end > len(points) {
			end = len(points)
		}
		vals := make([]int, 0, end-start)
		for j := start; j < end; j++ {
			if points[j].Viewers > 0 {
				vals = append(vals, points[j].Viewers)
			}
		}
		if len(vals) == 0 {
			continue
		}
		sort.Ints(vals)
		out[i].Viewers = vals[len(vals)/2]
	}
	return out
}

func chartPeakViewers(points []parsedViewerPoint) int {
	return chartMaxViewers(points)
}

func chartAvgViewers(points []parsedViewerPoint) int {
	if len(points) == 0 {
		return 0
	}
	sum := 0
	count := 0
	for _, pt := range points {
		if pt.Viewers <= 0 {
			continue
		}
		sum += pt.Viewers
		count++
	}
	if count == 0 {
		return 0
	}
	return (sum + count/2) / count
}

// TTFixtureReport captures parser fidelity metrics for a TT HTML fixture.
type TTFixtureReport struct {
	Fixture           string  `json:"fixture"`
	PointCount        int     `json:"point_count"`
	DurationMinutes   int     `json:"duration_minutes"`
	PeakViewers       int     `json:"peak_viewers"`
	AvgViewers        int     `json:"avg_viewers"`
	ExpectedPeak      int     `json:"expected_peak,omitempty"`
	ExpectedAvg       int     `json:"expected_avg,omitempty"`
	PeakDeltaPct      float64 `json:"peak_delta_pct,omitempty"`
	AvgDeltaPct       float64 `json:"avg_delta_pct,omitempty"`
	LastOffsetSec     int     `json:"last_offset_sec"`
	DurationSec       int     `json:"duration_sec"`
	CoveragePct       float64 `json:"coverage_pct"`
	CompleteChart     bool    `json:"complete_chart"`
	FlatLine          bool    `json:"flat_line"`
	MeetsPointDensity bool    `json:"meets_point_density"`
	ParseError        string  `json:"parse_error,omitempty"`
}

func evaluateTTFixture(name string, html []byte, startedAt time.Time, expectedPeak, expectedAvg, expectedDurationMin int) TTFixtureReport {
	s := &SyncService{log: slog.Default()}
	parsed, err := s.parseTwitchTrackerHTML(string(html), startedAt)
	report := TTFixtureReport{
		Fixture:         name,
		PointCount:      len(parsed.ViewerPoints),
		DurationMinutes: parsed.DurationMinutes,
		PeakViewers:     parsed.PeakViewers,
		AvgViewers:      parsed.AvgViewers,
		ExpectedPeak:    expectedPeak,
		ExpectedAvg:     expectedAvg,
	}
	if err != nil {
		report.ParseError = err.Error()
	}
	if report.PeakViewers <= 0 {
		report.PeakViewers = chartPeakViewers(parsed.ViewerPoints)
	}
	if report.AvgViewers <= 0 {
		report.AvgViewers = chartAvgViewers(parsed.ViewerPoints)
	}
	durSec := parsed.DurationMinutes * 60
	if durSec <= 0 && len(parsed.ViewerPoints) > 0 {
		durSec = lastViewerOffsetSeconds(parsed.ViewerPoints)
	}
	report.DurationSec = durSec
	report.LastOffsetSec = lastViewerOffsetSeconds(parsed.ViewerPoints)
	if durSec > 0 {
		report.CoveragePct = roundPct(float64(report.LastOffsetSec) / float64(durSec) * 100)
	}
	report.CompleteChart = hasCompleteViewerChart(parsed.ViewerPoints, durSec)
	report.FlatLine = isFlatViewerChart(parsed.ViewerPoints)
	report.MeetsPointDensity = len(parsed.ViewerPoints) >= max(1, expectedDurationMin/10)
	if expectedPeak > 0 && report.PeakViewers > 0 {
		report.PeakDeltaPct = roundPct(math.Abs(float64(report.PeakViewers-expectedPeak)) / float64(expectedPeak) * 100)
	}
	if expectedAvg > 0 && report.AvgViewers > 0 {
		report.AvgDeltaPct = roundPct(math.Abs(float64(report.AvgViewers-expectedAvg)) / float64(expectedAvg) * 100)
	}
	return report
}

func isFlatViewerChart(points []parsedViewerPoint) bool {
	if len(points) < 2 {
		return true
	}
	minV, maxV := points[0].Viewers, points[0].Viewers
	for _, pt := range points[1:] {
		if pt.Viewers < minV {
			minV = pt.Viewers
		}
		if pt.Viewers > maxV {
			maxV = pt.Viewers
		}
	}
	return maxV <= minV
}

func roundPct(v float64) float64 {
	return math.Round(v*100) / 100
}

// TTVsLiveReport compares live minute rollups to TT-interpolated values.
type TTVsLiveReport struct {
	StreamID               string  `json:"stream_id"`
	LiveMinutesWithSamples int     `json:"live_minutes_with_samples"`
	TTPointCount           int     `json:"tt_point_count"`
	PeakDeltaPct           float64 `json:"peak_delta_pct"`
	MinuteMAE              float64 `json:"minute_mae"`
	CoverageOverlapPct     float64 `json:"coverage_overlap_pct"`
	Recommendation         string  `json:"recommendation"`
}

func compareLiveRollupsToTT(streamID string, rollups []MinuteRollup, ttPoints []parsedViewerPoint, durationSec int) TTVsLiveReport {
	report := TTVsLiveReport{StreamID: streamID, TTPointCount: len(ttPoints)}
	if durationSec <= 0 {
		durationSec = len(rollups) * 60
	}
	rollupPts := toRollupViewerPoints(ttPoints)
	livePeak, ttPeak := 0, chartPeakViewers(ttPoints)
	var absErrSum float64
	var overlap, compared int
	for i, rollup := range rollups {
		if rollup.Missing || rollup.ViewerSamples <= 0 {
			continue
		}
		liveVal := rollup.ViewerAvg
		if liveVal == 0 {
			liveVal = rollup.ViewerLatest
		}
		if liveVal == 0 {
			liveVal = rollup.ViewerMax
		}
		if liveVal <= 0 {
			continue
		}
		report.LiveMinutesWithSamples++
		if liveVal > livePeak {
			livePeak = liveVal
		}
		ttVal := InterpolateViewerCount(i, rollupPts)
		if ttVal <= 0 {
			continue
		}
		overlap++
		compared++
		absErrSum += math.Abs(float64(liveVal - ttVal))
	}
	if compared > 0 {
		report.MinuteMAE = roundPct(absErrSum / float64(compared))
	}
	if report.LiveMinutesWithSamples > 0 {
		report.CoverageOverlapPct = roundPct(float64(overlap) / float64(report.LiveMinutesWithSamples) * 100)
	}
	if livePeak > 0 && ttPeak > 0 {
		report.PeakDeltaPct = roundPct(math.Abs(float64(livePeak-ttPeak)) / float64(livePeak) * 100)
	}
	report.Recommendation = "merge_tt_gaps_only"
	return report
}

func writeTTJSONReport(path string, payload any) error {
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func countViewerSourceMinutes(rollups []MinuteRollup) (liveMinutes, ttMinutes int) {
	for _, rollup := range rollups {
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
		if rollup.ViewerSamples >= 2 {
			liveMinutes++
		} else if rollup.ViewerSamples == 1 {
			ttMinutes++
		}
	}
	return liveMinutes, ttMinutes
}
