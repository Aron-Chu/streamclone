package analytics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"streamclone/internal/analytics/heatmap"
)

const coverageStartToleranceSec = 120

// Pulse coverage states exposed to the Pulse extension.
const (
	CoverageStateFullStreamTracked = "full_stream_tracked"
	CoverageStatePartialTracking   = "partial_tracking"
	CoverageStateMissingRanges     = "missing_ranges_detected"
	CoverageStateWaitingForVOD     = "waiting_for_vod"
	CoverageStateVODUnavailable    = "vod_unavailable"
	CoverageStateBackfillRunning   = "backfill_running"
	CoverageStateBackfillFailed    = "backfill_failed"
)

type ExtensionCoverageRange struct {
	FromOffsetSeconds int `json:"fromOffsetSeconds"`
	ToOffsetSeconds   int `json:"toOffsetSeconds"`
}

type ExtensionCoverage struct {
	State                      string                   `json:"state"`
	CoverageStartOffsetSeconds int                      `json:"coverageStartOffsetSeconds"`
	CoverageEndOffsetSeconds   int                      `json:"coverageEndOffsetSeconds"`
	HasFullStreamCoverage      bool                     `json:"hasFullStreamCoverage"`
	TrackedFromStart           bool                     `json:"trackedFromStart"`
	HasGaps                    bool                     `json:"hasGaps"`
	MissingRanges              []ExtensionCoverageRange `json:"missingRanges,omitempty"`
	CanBackfill                bool                     `json:"canBackfill"`
	BackfillReason             string                   `json:"backfillReason,omitempty"`
	VODStatus                  string                   `json:"vodStatus,omitempty"`
	ManualRetryAllowed         bool                     `json:"manualRetryAllowed,omitempty"`
	CopyKey                    string                   `json:"copyKey,omitempty"`
	Message                    string                   `json:"message"`
}

func computePulseCoverage(
	rollups []heatmap.MinuteRollup,
	streamStart time.Time,
	currentOffset int,
	isLive bool,
	vodID string,
	backfillRunning bool,
	backfillFailed bool,
) ExtensionCoverage {
	offsets := completedChatOffsets(rollups, streamStart)
	coverageStart := 0
	coverageEnd := 0
	if len(offsets) > 0 {
		coverageStart = offsets[0]
		coverageEnd = offsets[len(offsets)-1]
	}

	missingRanges := detectMissingRanges(offsets, coverageStart)
	hasGaps := len(missingRanges) > 0
	hasFull := len(offsets) > 0 && coverageStart <= coverageStartToleranceSec && !hasGaps

	state := CoverageStatePartialTracking
	message := "Showing moments since tracking began"
	canBackfill := false
	backfillReason := ""

	switch {
	case backfillRunning:
		state = CoverageStateBackfillRunning
		message = "Loading missed chat replay…"
	case backfillFailed:
		state = CoverageStateBackfillFailed
		message = "Could not load missed moments"
	case hasFull:
		state = CoverageStateFullStreamTracked
		message = "Full stream tracked"
	case hasGaps:
		state = CoverageStateMissingRanges
		message = formatMissingPrefixMessage(missingRanges)
	case coverageStart > coverageStartToleranceSec:
		state = CoverageStatePartialTracking
		message = fmt.Sprintf("Showing moments since %s", formatCoverageOffset(coverageStart))
	default:
		state = CoverageStatePartialTracking
	}

	if !backfillRunning && !backfillFailed && hasGaps {
		if strings.TrimSpace(vodID) != "" {
			canBackfill = true
			backfillReason = "vod_available"
		} else if isLive {
			state = CoverageStateWaitingForVOD
			message = "VOD chat not available yet — archive publishes after the stream ends"
			backfillReason = "waiting_vod"
		} else {
			state = CoverageStateVODUnavailable
			message = "VOD chat replay is unavailable for this stream"
			backfillReason = "unavailable"
		}
	} else if !backfillRunning && !hasFull && coverageStart > coverageStartToleranceSec {
		if strings.TrimSpace(vodID) != "" {
			canBackfill = true
			backfillReason = "vod_available"
			if state == CoverageStatePartialTracking {
				state = CoverageStateMissingRanges
				if len(missingRanges) == 0 {
					missingRanges = []ExtensionCoverageRange{{
						FromOffsetSeconds: 0,
						ToOffsetSeconds:   coverageStart - 60,
					}}
					hasGaps = true
				}
				message = formatMissingPrefixMessage(missingRanges)
			}
		} else if isLive {
			state = CoverageStateWaitingForVOD
			message = "VOD chat not available yet"
			backfillReason = "waiting_vod"
		} else {
			state = CoverageStateVODUnavailable
			message = "VOD chat replay is unavailable for this stream"
			backfillReason = "unavailable"
		}
	}

	if isLive && currentOffset > 0 && coverageEnd > 0 && !hasFull && !hasGaps && coverageStart <= coverageStartToleranceSec {
		// Live edge still collecting — not a "missed prefix" case.
		if state == CoverageStatePartialTracking && len(offsets) > 0 {
			message = "Collecting live moments"
		}
	}

	return decoratePulseCoverage(ExtensionCoverage{
		State:                      state,
		CoverageStartOffsetSeconds: coverageStart,
		CoverageEndOffsetSeconds:   coverageEnd,
		HasFullStreamCoverage:      hasFull,
		TrackedFromStart:           hasFull,
		HasGaps:                    hasGaps,
		MissingRanges:              missingRanges,
		CanBackfill:                canBackfill,
		BackfillReason:             backfillReason,
		Message:                    message,
	}, vodID)
}

func decoratePulseCoverage(c ExtensionCoverage, vodID string) ExtensionCoverage {
	c.TrackedFromStart = c.HasFullStreamCoverage || c.CoverageStartOffsetSeconds <= coverageStartToleranceSec && c.CoverageEndOffsetSeconds > 0
	if strings.TrimSpace(vodID) != "" {
		c.VODStatus = "available"
	} else {
		switch c.State {
		case CoverageStateWaitingForVOD:
			c.VODStatus = "waiting"
		case CoverageStateVODUnavailable:
			c.VODStatus = "unavailable"
			c.ManualRetryAllowed = true
		}
	}
	if c.CopyKey == "" {
		c.CopyKey = c.State
	}
	return c
}

// enrichExtensionCoverage aligns nested coverage with rollup-based stream start when
// computePulseCoverage missed a late-tracking prefix (sparse rollups).
func enrichExtensionCoverage(c ExtensionCoverage, rollupStart int, vodID string, isLive bool) ExtensionCoverage {
	if rollupStart <= coverageStartToleranceSec {
		return c
	}
	if c.HasFullStreamCoverage {
		return c
	}
	if rollupStart > c.CoverageStartOffsetSeconds {
		c.CoverageStartOffsetSeconds = rollupStart
	}
	if c.CanBackfill {
		return c
	}
	if strings.TrimSpace(vodID) != "" {
		c.CanBackfill = true
		c.BackfillReason = "vod_available"
		c.HasGaps = true
		if len(c.MissingRanges) == 0 {
			c.MissingRanges = []ExtensionCoverageRange{{
				FromOffsetSeconds: 0,
				ToOffsetSeconds:   rollupStart - 60,
			}}
		}
		if c.State == CoverageStatePartialTracking || c.State == "" {
			c.State = CoverageStateMissingRanges
		}
		c.Message = formatMissingPrefixMessage(c.MissingRanges)
		return decoratePulseCoverage(c, vodID)
	}
	if isLive && c.State != CoverageStateBackfillRunning && c.State != CoverageStateBackfillFailed {
		c.State = CoverageStateWaitingForVOD
		c.Message = "VOD chat not available yet — archive publishes after the stream ends"
		c.BackfillReason = "waiting_vod"
	}
	return decoratePulseCoverage(c, vodID)
}

func completedChatOffsets(rollups []heatmap.MinuteRollup, streamStart time.Time) []int {
	if streamStart.IsZero() || len(rollups) == 0 {
		return nil
	}
	base := streamStart.UTC().Truncate(time.Minute)
	seen := make(map[int]struct{}, len(rollups))
	var offsets []int
	for _, r := range rollups {
		if r.Missing {
			continue
		}
		if r.ChatCount == 0 && r.TotalEmoteCount == 0 && r.SevenTVEmoteCount == 0 {
			continue
		}
		offset := int(r.MinuteTS.Sub(base).Seconds())
		if offset < 0 {
			offset = 0
		}
		if _, ok := seen[offset]; ok {
			continue
		}
		seen[offset] = struct{}{}
		offsets = append(offsets, offset)
	}
	sort.Ints(offsets)
	return offsets
}

func detectMissingRanges(offsets []int, coverageStart int) []ExtensionCoverageRange {
	var out []ExtensionCoverageRange
	if len(offsets) == 0 {
		return out
	}
	if coverageStart > coverageStartToleranceSec {
		out = append(out, ExtensionCoverageRange{
			FromOffsetSeconds: 0,
			ToOffsetSeconds:   coverageStart - 60,
		})
	}
	const gapThreshold = 180
	for i := 1; i < len(offsets); i++ {
		gap := offsets[i] - offsets[i-1]
		if gap <= gapThreshold {
			continue
		}
		from := offsets[i-1] + 60
		to := offsets[i] - 60
		if to <= from {
			continue
		}
		out = append(out, ExtensionCoverageRange{
			FromOffsetSeconds: from,
			ToOffsetSeconds:   to,
		})
	}
	return out
}

func formatMissingPrefixMessage(ranges []ExtensionCoverageRange) string {
	if len(ranges) == 0 {
		return "Missing chat coverage"
	}
	first := ranges[0]
	if first.FromOffsetSeconds == 0 && len(ranges) == 1 {
		return fmt.Sprintf("Missing first %s", formatCoverageDuration(first.ToOffsetSeconds))
	}
	return fmt.Sprintf("Missing %d coverage gap(s)", len(ranges))
}

func formatCoverageDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatCoverageOffset(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func rangeFullyCovered(offsets []int, fromOffset, toOffset int) bool {
	if toOffset <= fromOffset {
		return true
	}
	if len(offsets) == 0 {
		return false
	}
	covered := completedOffsetsSet(offsets)
	for minute := fromOffset / 60; minute <= toOffset/60; minute++ {
		off := minute * 60
		if off < fromOffset {
			continue
		}
		if off > toOffset {
			break
		}
		if _, ok := covered[off]; !ok {
			return false
		}
	}
	return true
}

func completedOffsetsSet(offsets []int) map[int]struct{} {
	out := make(map[int]struct{}, len(offsets))
	for _, off := range offsets {
		out[off] = struct{}{}
	}
	return out
}

func mergeMissingRanges(ranges []ExtensionCoverageRange, fromOffset, toOffset int) []ExtensionCoverageRange {
	if toOffset <= fromOffset {
		return nil
	}
	if len(ranges) == 0 {
		return []ExtensionCoverageRange{{FromOffsetSeconds: fromOffset, ToOffsetSeconds: toOffset}}
	}
	var out []ExtensionCoverageRange
	for _, r := range ranges {
		if r.ToOffsetSeconds <= fromOffset || r.FromOffsetSeconds >= toOffset {
			continue
		}
		from := r.FromOffsetSeconds
		to := r.ToOffsetSeconds
		if from < fromOffset {
			from = fromOffset
		}
		if to > toOffset {
			to = toOffset
		}
		if to > from {
			out = append(out, ExtensionCoverageRange{FromOffsetSeconds: from, ToOffsetSeconds: to})
		}
	}
	if len(out) == 0 {
		return []ExtensionCoverageRange{{FromOffsetSeconds: fromOffset, ToOffsetSeconds: toOffset}}
	}
	return out
}
