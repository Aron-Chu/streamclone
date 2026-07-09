package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	IngestActiveCollectors = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_active_collectors",
		Help: "IRC channels with active collectors",
	})
	IngestDesiredCollectors = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_desired_collectors",
		Help: "IRC channels desired by tier scheduler",
	})
	IngestAdmitLagSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_admit_lag_seconds",
		Help: "Seconds between desired and active collector admission",
	})
	IngestIRCJoinRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_irc_join_rate",
		Help: "IRC JOIN rate per minute (rolling)",
	})
	IngestIRCPartRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_irc_part_rate",
		Help: "IRC PART rate per minute (rolling)",
	})
	IngestMessagesDroppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_messages_dropped_total",
		Help: "IRC messages rejected at enqueue due to backpressure",
	}, []string{"tier"})
	IngestIRCQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_irc_queue_depth",
		Help: "Depth of global IRC line ingress queue",
	})
	IngestShardQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ingest_shard_queue_depth",
		Help: "Depth of per-shard aggregate inbox",
	}, []string{"shard"})
	IngestFlushQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ingest_flush_queue_depth",
		Help: "Depth of pending rollup flush queue",
	})
	IngestIRCQueueAgeSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ingest_irc_queue_age_seconds",
		Help:    "Age of IRC messages waiting in ingress queue",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	})
	IngestShardQueueAgeSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ingest_shard_queue_age_seconds",
		Help:    "Age of messages waiting in shard inbox",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	}, []string{"shard"})
	IngestFlushQueueAgeSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ingest_flush_queue_age_seconds",
		Help:    "Age of rollups waiting in flush queue",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 16),
	})
	IngestPostgresWriteErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_postgres_write_errors_total",
		Help: "Postgres rollup write errors from IRC ingest flusher",
	})
	IngestRedisWriteErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_redis_write_errors_total",
		Help: "Redis write errors from IRC ingest writer",
	})
	IngestShadowCompareMismatchTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_shadow_compare_mismatch_total",
		Help: "Shadow compare rollup mismatches vs legacy path",
	})
	IngestShadowCompareMatchTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_shadow_compare_match_total",
		Help: "Shadow compare rollup matches vs legacy path",
	})
)
