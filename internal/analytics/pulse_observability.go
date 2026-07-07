package analytics

import (
	"strings"
	"time"

	"streamclone/internal/metrics"
)

const trackedFromStartToleranceSec = 120

type pulseGoLiveObservation struct {
	login           string
	detectedAt      time.Time
	streamStartedAt time.Time
	source          string
	priority        int
}

// goLive source classes exposed as low-cardinality Prometheus labels.
func normalizeGoLiveSourceClass(source string) string {
	switch strings.TrimSpace(source) {
	case "global_protected", "principal_always_track":
		return "always_track"
	case "top_roster":
		return "top_roster"
	case "manual", "extension":
		return "manual"
	default:
		if source == "" {
			return "protected"
		}
		return "protected"
	}
}

func isCapTierGoLiveSourceClass(sourceClass string) bool {
	switch sourceClass {
	case "top_roster", "always_track", "protected":
		return true
	default:
		return false
	}
}

func coverageStartOffsetSecondsForRollup(obs pulseGoLiveObservation, rollupMinute time.Time) int {
	anchor := obs.detectedAt
	if !obs.streamStartedAt.IsZero() {
		anchor = obs.streamStartedAt
	}
	offsetSec := int(rollupMinute.Sub(anchor).Seconds())
	if offsetSec < 0 {
		offsetSec = 0
	}
	return offsetSec
}

// NoteGoLiveDetected records offline→live for R1 metrics/logs. duplicate=true when the same stream was already tracked.
// streamStartedAt is optional Twitch stream start; when set, coverage-start metrics use it instead of detection time.
func (c *Collector) NoteGoLiveDetected(streamID, login, source string, priority int, duplicate bool, streamStartedAt time.Time) {
	if c == nil || strings.TrimSpace(streamID) == "" {
		return
	}
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	sourceClass := normalizeGoLiveSourceClass(source)
	if duplicate {
		metrics.PulseGoLiveDuplicateObservationTotal.Inc()
		return
	}
	now := c.nowUTC()
	c.goLiveByStream.Store(streamID, pulseGoLiveObservation{
		login:           login,
		detectedAt:      now,
		streamStartedAt: streamStartedAt.UTC(),
		source:          sourceClass,
		priority:        priority,
	})
	metrics.PulseGoLiveDetectedTotal.WithLabelValues(sourceClass).Inc()
	if c.log != nil {
		c.log.Info("pulse go-live detected",
			"login", login,
			"stream_id", streamID,
			"source", sourceClass,
			"priority", priority,
			"go_live_detected_at", now.Format(time.RFC3339Nano),
			"stream_started_at", streamStartedAt.UTC().Format(time.RFC3339Nano),
		)
	}
}

func (c *Collector) recordFirstRollupMetrics(streamID string, rollupMinute time.Time) {
	if c == nil || strings.TrimSpace(streamID) == "" {
		return
	}
	streamID = strings.TrimSpace(streamID)
	if _, loaded := c.firstRollupRecorded.LoadOrStore(streamID, struct{}{}); loaded {
		return
	}
	raw, ok := c.goLiveByStream.Load(streamID)
	if !ok {
		return
	}
	obs, ok := raw.(pulseGoLiveObservation)
	if !ok {
		return
	}
	firstAt := c.nowUTC()
	elapsed := firstAt.Sub(obs.detectedAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	metrics.PulseGoLiveToFirstRollupSeconds.Observe(elapsed)
	offsetSec := coverageStartOffsetSecondsForRollup(obs, rollupMinute)
	metrics.PulseCoverageStartOffsetSeconds.Observe(float64(offsetSec))
	trackedFromStart := offsetSec <= trackedFromStartToleranceSec
	if trackedFromStart {
		metrics.PulseTrackedFromStartTotal.Inc()
	} else if isCapTierGoLiveSourceClass(obs.source) {
		metrics.PulseLateCapStartTotal.WithLabelValues(obs.source).Inc()
	}
	if c.log != nil {
		c.log.Info("pulse first rollup recorded",
			"login", obs.login,
			"stream_id", streamID,
			"source", obs.source,
			"priority", obs.priority,
			"go_live_detected_at", obs.detectedAt.Format(time.RFC3339Nano),
			"stream_started_at", obs.streamStartedAt.UTC().Format(time.RFC3339Nano),
			"first_rollup_written_at", firstAt.Format(time.RFC3339Nano),
			"go_live_to_first_rollup_seconds", elapsed,
			"coverage_start_offset_seconds", offsetSec,
			"tracked_from_start", trackedFromStart,
		)
	}
}

// RefreshPulseMetricGauges updates low-cardinality Pulse gauges from in-memory state.
func RefreshPulseMetricGauges(collector *Collector, backfill *PulseBackfillManager) {
	if collector != nil {
		metrics.PulseActiveTrackedChannels.Set(float64(collector.TrackingSnapshot().Active))
	}
	if backfill != nil {
		metrics.PulseBackfillActiveJobs.Set(float64(backfill.Snapshot().Active))
	}
}
