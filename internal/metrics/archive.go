package metrics

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ArchiveExportsConfirmedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "archive_exports_confirmed_total",
		Help: "Confirmed archive export uploads by artifact type.",
	}, []string{"artifact_type"})
	ArchiveExportsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "archive_exports_failed_total",
		Help: "Failed archive export uploads by artifact type.",
	}, []string{"artifact_type"})
	BackfillJobsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "backfill_jobs_gauge",
		Help: "Current backfill job counts by tier and status.",
	}, []string{"tier", "status"})
	BronzeChannelsIndexedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bronze_channels_indexed_total",
		Help: "Bronze channels with a successful Helix VOD index export.",
	})
	BronzeChannelsTarget = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bronze_channels_target",
		Help: "Target bronze channel count for the current roster pass.",
	})
	Tier0CoveragePctHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tier0_coverage_pct",
		Help:    "Tier-0 viewer minute coverage percent observed on stream export.",
		Buckets: []float64{0, 20, 40, 60, 70, 80, 85, 90, 95, 100},
	})
)

func RecordArchiveExportConfirmed(artifactType string) {
	if artifactType = prometheusLabel(artifactType); artifactType != "" {
		ArchiveExportsConfirmedTotal.WithLabelValues(artifactType).Inc()
	}
}

func RecordArchiveExportFailed(artifactType string) {
	if artifactType = prometheusLabel(artifactType); artifactType != "" {
		ArchiveExportsFailedTotal.WithLabelValues(artifactType).Inc()
	}
}

func RecordTier0CoveragePct(pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	Tier0CoveragePctHistogram.Observe(pct)
}

func SetBronzeChannelsTarget(count int) {
	if count < 0 {
		count = 0
	}
	BronzeChannelsTarget.Set(float64(count))
}

func IncBronzeChannelsIndexed() {
	BronzeChannelsIndexedTotal.Inc()
}

func RefreshBackfillJobGauges(ctx context.Context, db *pgxpool.Pool) {
	if db == nil {
		return
	}
	rows, err := db.Query(ctx, `
		SELECT tier, status, COUNT(*)::float8
		FROM backfill_jobs
		GROUP BY tier, status`)
	if err != nil {
		return
	}
	defer rows.Close()
	BackfillJobsGauge.Reset()
	for rows.Next() {
		var tier, status string
		var count float64
		if err := rows.Scan(&tier, &status, &count); err != nil {
			return
		}
		BackfillJobsGauge.WithLabelValues(prometheusLabel(tier), prometheusLabel(status)).Set(count)
	}
}

func prometheusLabel(v string) string {
	v = trimPrometheusLabel(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func trimPrometheusLabel(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 64 {
		return v[:64]
	}
	return v
}
