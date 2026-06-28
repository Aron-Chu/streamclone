package metrics

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Top500VODInventoryTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_vod_inventory_total",
		Help: "Top 500 VOD inventory count by Gold status and availability state.",
	}, []string{"gold_status", "availability_state"})
	Top500VODInventoryRankBucketTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_vod_inventory_rank_bucket_total",
		Help: "Top 500 VOD inventory count by rank bucket, Gold status, and availability state.",
	}, []string{"rank_bucket", "gold_status", "availability_state"})
	Top500VODInventoryOldestQueuedAgeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_vod_inventory_oldest_queued_age_seconds",
		Help: "Age in seconds of the oldest queued Top 500 Gold VOD inventory item.",
	})
	Top500VODInventoryArchiveConfirmedTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top500_vod_inventory_archive_confirmed_total",
		Help: "Top 500 VOD inventory items with confirmed Gold chat archive export.",
	})
	Top500VODInventoryTerminalTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_top500_vod_inventory_terminal_total",
		Help: "Top 500 VOD inventory items in terminal availability states.",
	}, []string{"state"})
)

// RefreshTop500VODInventoryGauges exports aggregate-only inventory coverage.
// It intentionally avoids per-login, per-stream, and per-VOD labels.
func RefreshTop500VODInventoryGauges(ctx context.Context, db *pgxpool.Pool) {
	if db == nil {
		return
	}
	rows, err := db.Query(ctx, `
		SELECT COALESCE(NULLIF(gold_status,''),'unknown'),
		       COALESCE(NULLIF(availability_state,''),'unknown'),
		       COUNT(*)::float8
		FROM top500_vod_inventory
		GROUP BY gold_status, availability_state`)
	if err != nil {
		return
	}
	defer rows.Close()
	Top500VODInventoryTotal.Reset()
	for rows.Next() {
		var goldStatus, availability string
		var count float64
		if err := rows.Scan(&goldStatus, &availability, &count); err != nil {
			return
		}
		Top500VODInventoryTotal.WithLabelValues(prometheusLabel(goldStatus), prometheusLabel(availability)).Set(count)
	}

	bucketRows, err := db.Query(ctx, `
		SELECT CASE
		         WHEN top500_rank IS NULL THEN 'unranked'
		         WHEN top500_rank <= 50 THEN 'rank_001_050'
		         WHEN top500_rank <= 100 THEN 'rank_051_100'
		         WHEN top500_rank <= 200 THEN 'rank_101_200'
		         WHEN top500_rank <= 500 THEN 'rank_201_500'
		         ELSE 'outside_500'
		       END AS rank_bucket,
		       COALESCE(NULLIF(gold_status,''),'unknown'),
		       COALESCE(NULLIF(availability_state,''),'unknown'),
		       COUNT(*)::float8
		FROM top500_vod_inventory
		GROUP BY rank_bucket, gold_status, availability_state`)
	if err != nil {
		return
	}
	defer bucketRows.Close()
	Top500VODInventoryRankBucketTotal.Reset()
	for bucketRows.Next() {
		var bucket, goldStatus, availability string
		var count float64
		if err := bucketRows.Scan(&bucket, &goldStatus, &availability, &count); err != nil {
			return
		}
		Top500VODInventoryRankBucketTotal.WithLabelValues(
			prometheusLabel(bucket),
			prometheusLabel(goldStatus),
			prometheusLabel(availability),
		).Set(count)
	}

	var oldestQueuedAge float64
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MIN(gold_queued_at))), 0)::float8
		FROM top500_vod_inventory
		WHERE gold_status IN ('queued','running')`).Scan(&oldestQueuedAge); err == nil {
		Top500VODInventoryOldestQueuedAgeSeconds.Set(oldestQueuedAge)
	}

	var confirmed float64
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)::float8
		FROM top500_vod_inventory
		WHERE archive_export_status = 'confirmed'`).Scan(&confirmed); err == nil {
		Top500VODInventoryArchiveConfirmedTotal.Set(confirmed)
	}

	terminalRows, err := db.Query(ctx, `
		SELECT availability_state, COUNT(*)::float8
		FROM top500_vod_inventory
		WHERE availability_state IN (
			'expired',
			'deleted',
			'private_or_sub_only',
			'no_chat',
			'region_blocked',
			'gql_forbidden',
			'unknown_unavailable',
			'failed'
		)
		GROUP BY availability_state`)
	if err != nil {
		return
	}
	defer terminalRows.Close()
	Top500VODInventoryTerminalTotal.Reset()
	for terminalRows.Next() {
		var state string
		var count float64
		if err := terminalRows.Scan(&state, &count); err != nil {
			return
		}
		Top500VODInventoryTerminalTotal.WithLabelValues(prometheusLabel(state)).Set(count)
	}
}
