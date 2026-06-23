package metrics



import (

	"context"

	"strings"

	"time"



	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/client_golang/prometheus/promauto"

)



const defaultBackfillStaleRunningAfter = 2 * time.Hour



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

	BackfillOldestQueuedAgeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{

		Name: "backfill_oldest_queued_age_seconds",

		Help: "Age in seconds of the oldest queued backfill job by tier.",

	}, []string{"tier"})

	BackfillStaleRunningGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{

		Name: "backfill_stale_running_gauge",

		Help: "Running backfill jobs older than the stale lease by tier.",

	}, []string{"tier"})

	BackfillJobsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{

		Name: "backfill_jobs_completed_total",

		Help: "Backfill jobs finished by tier and terminal status.",

	}, []string{"tier", "status"})

	BackfillWorkerLastTickTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{

		Name: "backfill_worker_last_tick_timestamp",

		Help: "Unix timestamp of the last backfill worker tick by worker name.",

	}, []string{"worker"})

	BackfillWorkerPanicsTotal = promauto.NewCounterVec(prometheus.CounterOpts{

		Name: "backfill_worker_panics_total",

		Help: "Recovered panics in backfill worker loops by worker name.",

	}, []string{"worker"})

	BronzeChannelsIndexedTotal = promauto.NewCounter(prometheus.CounterOpts{

		Name: "bronze_channels_indexed_total",

		Help: "Bronze channels with a successful Helix VOD index export.",

	})

	BronzeChannelsTarget = promauto.NewGauge(prometheus.GaugeOpts{

		Name: "bronze_channels_target",

		Help: "Target bronze channel count for the current roster pass.",

	})

	BronzeChannelsRosterGauge = promauto.NewGauge(prometheus.GaugeOpts{

		Name: "bronze_channels_roster_gauge",

		Help: "Distinct channels in bronze_index_state (roster progress).",

	})

	CorpusTierCompletionRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{

		Name: "corpus_tier_completion_ratio",

		Help: "Corpus completion ratio 0-1 by tier and measure (roster, export_confirmed, jobs_done).",

	}, []string{"tier", "measure"})

	Tier0CoveragePctHistogram = promauto.NewHistogram(prometheus.HistogramOpts{

		Name:    "tier0_coverage_pct",

		Help:    "Tier-0 viewer minute coverage percent observed on stream export.",

		Buckets: []float64{0, 20, 40, 60, 70, 80, 85, 90, 95, 100},

	})

)



var bronzeTargetCount float64



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

	bronzeTargetCount = float64(count)

	BronzeChannelsTarget.Set(float64(count))

}



func IncBronzeChannelsIndexed() {

	BronzeChannelsIndexedTotal.Inc()

}



func RecordBackfillJobCompleted(tier, status string) {

	BackfillJobsCompletedTotal.WithLabelValues(prometheusLabel(tier), prometheusLabel(status)).Inc()

}



func RecordBackfillWorkerTick(worker string) {

	BackfillWorkerLastTickTimestamp.WithLabelValues(prometheusLabel(worker)).Set(float64(time.Now().Unix()))

}



func RecordBackfillWorkerPanic(worker string) {

	BackfillWorkerPanicsTotal.WithLabelValues(prometheusLabel(worker)).Inc()

}



func RefreshBackfillJobGauges(ctx context.Context, db *pgxpool.Pool, staleAfter ...time.Duration) {

	if db == nil {

		return

	}

	after := defaultBackfillStaleRunningAfter

	if len(staleAfter) > 0 && staleAfter[0] > 0 {

		after = staleAfter[0]

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

	BackfillOldestQueuedAgeSeconds.Reset()

	ageRows, err := db.Query(ctx, `

		SELECT tier, COALESCE(EXTRACT(EPOCH FROM (now() - MIN(next_run_at))), 0)::float8

		FROM backfill_jobs

		WHERE status = 'queued'

		GROUP BY tier`)

	if err == nil {

		defer ageRows.Close()

		for ageRows.Next() {

			var tier string

			var age float64

			if err := ageRows.Scan(&tier, &age); err != nil {

				break

			}

			BackfillOldestQueuedAgeSeconds.WithLabelValues(prometheusLabel(tier)).Set(age)

		}

	}

	BackfillStaleRunningGauge.Reset()

	staleRows, err := db.Query(ctx, `

		SELECT tier, COUNT(*)::float8

		FROM backfill_jobs

		WHERE status = 'running' AND updated_at < now() - $1::interval

		GROUP BY tier`, after)

	if err == nil {

		defer staleRows.Close()

		for staleRows.Next() {

			var tier string

			var count float64

			if err := staleRows.Scan(&tier, &count); err != nil {

				break

			}

			BackfillStaleRunningGauge.WithLabelValues(prometheusLabel(tier)).Set(count)

		}

	}

	RefreshCorpusCompletionGauges(ctx, db)

}



func RefreshCorpusCompletionGauges(ctx context.Context, db *pgxpool.Pool) {

	if db == nil {

		return

	}

	var bronzeCount float64

	if err := db.QueryRow(ctx, `SELECT COUNT(*)::float8 FROM bronze_index_state`).Scan(&bronzeCount); err == nil {

		BronzeChannelsRosterGauge.Set(bronzeCount)

		if bronzeTargetCount > 0 {

			CorpusTierCompletionRatio.WithLabelValues("bronze", "roster").Set(bronzeCount / bronzeTargetCount)

		}

	}

	CorpusTierCompletionRatio.Reset()

	tierRows, err := db.Query(ctx, `

		SELECT tier,

			COUNT(*)::float8 AS total,

			COUNT(*) FILTER (WHERE status = 'done')::float8 AS done,

			COUNT(*) FILTER (WHERE export_status = 'confirmed')::float8 AS confirmed

		FROM backfill_jobs

		WHERE tier IN ('silver', 'gold')

		GROUP BY tier`)

	if err != nil {

		return

	}

	defer tierRows.Close()

	for tierRows.Next() {

		var tier string

		var total, done, confirmed float64

		if err := tierRows.Scan(&tier, &total, &done, &confirmed); err != nil {

			return

		}

		tier = prometheusLabel(tier)

		if total <= 0 {

			continue

		}

		CorpusTierCompletionRatio.WithLabelValues(tier, "jobs_done").Set(done / total)

		CorpusTierCompletionRatio.WithLabelValues(tier, "export_confirmed").Set(confirmed / total)

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
