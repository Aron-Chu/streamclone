package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Top500MetadataSamplerEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_sampler_enabled",
		Help: "Whether the Top 100 metadata sampler is enabled in the current process.",
	})
	Top500MetadataDryRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_dry_run",
		Help: "Whether the Top 100 metadata sampler is running in dry-run mode.",
	})
	Top500MetadataWriteEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_write_enabled",
		Help: "Whether the Top 100 metadata sampler write path is enabled.",
	})
	Top500MetadataRosterSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_roster_size",
		Help: "Number of Top 100 metadata roster rows considered during the latest sampler tick.",
	})
	Top500MetadataTopNConfigured = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_top_n_configured",
		Help: "Configured Top N limit for the Top 100 metadata sampler after safety caps.",
	})
	Top500MetadataChannelsPlannedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_channels_planned_total",
		Help: "Top 100 metadata channels planned by result and sampler mode.",
	}, []string{"result", "mode"})
	Top500MetadataChannelsSampledTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_channels_sampled_total",
		Help: "Top 100 metadata channels sampled by result and sampler mode.",
	}, []string{"result", "mode"})
	Top500MetadataProviderCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_provider_calls_total",
		Help: "Top 100 metadata provider calls by operation, result, and source.",
	}, []string{"operation", "result", "source"})
	Top500MetadataProviderErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_provider_errors_total",
		Help: "Top 100 metadata provider errors by operation, reason, and source.",
	}, []string{"operation", "reason", "source"})
	Top500MetadataProviderRateLimitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_provider_rate_limits_total",
		Help: "Top 100 metadata provider rate-limit classifications by operation and source.",
	}, []string{"operation", "source"})
	Top500MetadataFreshnessSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_freshness_seconds",
		Help: "Latest observed Top 100 metadata freshness age in seconds.",
	})
	Top500MetadataSnapshotWritesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_snapshot_writes_total",
		Help: "Top 100 metadata snapshot writes by result and sampler mode.",
	}, []string{"result", "mode"})
	Top500MetadataCurrentUpsertsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_current_upserts_total",
		Help: "Top 100 metadata current row upserts by result and sampler mode.",
	}, []string{"result", "mode"})
	Top500MetadataWriteBatchSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_write_batch_size",
		Help: "Latest Top 100 metadata write batch size by result, mode, and operation.",
	}, []string{"result", "mode", "operation"})
	Top500MetadataWriteLatencySeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_write_latency_seconds",
		Help: "Latest Top 100 metadata write latency in seconds by result, mode, and operation.",
	}, []string{"result", "mode", "operation"})
	Top500MetadataSamplesDegradedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_samples_degraded_total",
		Help: "Top 100 metadata sampler degraded or skipped states by reason and mode.",
	}, []string{"reason", "mode"})
	Top500MetadataRollbackState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_metadata_rollback_state",
		Help: "Whether a Top 100 metadata rollback-like state is currently observable by reason and mode.",
	}, []string{"reason", "mode"})
	Top500MetadataLockUnavailableTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top500_metadata_lock_unavailable_total",
		Help: "Top 100 metadata sampler lock-unavailable events by reason and mode.",
	}, []string{"reason", "mode"})
)
