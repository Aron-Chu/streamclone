package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Top500SilverGateEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_silver_gate_enabled",
		Help: "Whether the Top500 selective silver gate is enabled in the current process.",
	})
	Top500SilverGateDryRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_silver_gate_dry_run",
		Help: "Whether the Top500 selective silver gate is running in dry-run mode.",
	})
	Top500SilverGateWriteEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_silver_gate_write_enabled",
		Help: "Whether the Top500 selective silver gate write path is enabled.",
	})
	Top500SilverGateDecisionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_silver_gate_decisions_total",
		Help: "Top500 selective silver gate decisions by result, reason, lane, and operation.",
	}, []string{"result", "reason", "lane", "operation"})
	Top500SilverGateCandidatesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_silver_gate_candidates_total",
		Help: "Top500 selective silver gate candidate observations by lane and operation.",
	}, []string{"lane", "operation"})
	Top500SilverGateEnqueueAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_silver_gate_enqueue_attempts_total",
		Help: "Top500 selective silver gate enqueue attempts by result, lane, and operation.",
	}, []string{"result", "lane", "operation"})
	Top500SilverGateEnqueueErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_silver_gate_enqueue_errors_total",
		Help: "Top500 selective silver gate enqueue errors by reason, lane, and operation.",
	}, []string{"reason", "lane", "operation"})
	Top500SilverGateIdempotencySkipsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_silver_gate_idempotency_skips_total",
		Help: "Top500 selective silver gate idempotency skips by reason and lane.",
	}, []string{"reason", "lane"})
	Top500SilverGateDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "streamclone_silver_gate_duration_seconds",
		Help:    "Top500 selective silver gate evaluation duration by operation and lane.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation", "lane"})
)
