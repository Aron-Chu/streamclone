package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PulseActiveTrackedChannels = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_active_tracked_channels",
		Help: "Currently active Pulse tracked channels.",
	})
	PulseBackfillActiveJobs = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pulse_backfill_active_jobs",
		Help: "Active Pulse backfill jobs (not queued backlog depth).",
	})
	PulseGoLiveDetectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pulse_golive_detected_total",
		Help: "Protected/top-roster offline-to-live detections by source class.",
	}, []string{"source"})
	PulseGoLiveDuplicateObservationTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pulse_golive_duplicate_observation_total",
		Help: "Duplicate go-live observations for the same stream ID.",
	})
	PulseGoLiveToFirstRollupSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_golive_to_first_rollup_seconds",
		Help:    "Seconds from go-live detection to first minute rollup write.",
		Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 1800},
	})
	PulseTrackedFromStartTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pulse_tracked_from_start_total",
		Help: "Streams whose first rollup coverage start offset is within 120 seconds.",
	})
	PulseCoverageStartOffsetSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pulse_coverage_start_offset_seconds",
		Help:    "Coverage start offset in seconds recorded on first rollup for a stream.",
		Buckets: []float64{0, 30, 60, 120, 300, 600, 1800, 3600, 7200},
	})
)
