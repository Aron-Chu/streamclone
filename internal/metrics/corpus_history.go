package metrics

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CorpusMinuteRollupsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_minute_rollups_total",
		Help: "Historical analytics minute rollup rows retained in Postgres.",
	})
	CorpusMinuteRollupStreamsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_minute_rollup_streams_total",
		Help: "Distinct streams with historical analytics minute rollups retained in Postgres.",
	})
	CorpusMinuteRollupOldestTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_minute_rollup_oldest_timestamp",
		Help: "Unix timestamp of the oldest reasonable analytics minute rollup retained in Postgres.",
	})
	CorpusMinuteRollupNewestTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_minute_rollup_newest_timestamp",
		Help: "Unix timestamp of the newest analytics minute rollup retained in Postgres.",
	})
	CorpusVODChatMessagesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_vod_chat_messages_total",
		Help: "Historical VOD chat messages retained in hot Postgres.",
	})
	CorpusVODChatStreamsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_vod_chat_streams_total",
		Help: "Distinct streams with VOD chat messages retained in hot Postgres.",
	})
	CorpusVODChatOldestTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_vod_chat_oldest_timestamp",
		Help: "Unix timestamp of the oldest VOD chat message minute retained in hot Postgres.",
	})
	CorpusVODChatNewestTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_corpus_vod_chat_newest_timestamp",
		Help: "Unix timestamp of the newest VOD chat message minute retained in hot Postgres.",
	})
	CorpusArchiveExportsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_corpus_archive_exports_total",
		Help: "Historical archive export records by tier, artifact type, and export status.",
	}, []string{"tier", "artifact_type", "export_status"})
	CorpusArchiveExportRowsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_corpus_archive_export_rows_total",
		Help: "Historical archived row counts by tier, artifact type, and export status.",
	}, []string{"tier", "artifact_type", "export_status"})
)

// RefreshCorpusHistoryGauges exports aggregate historical corpus coverage.
// It avoids per-login, per-stream, and per-VOD labels so Prometheus stays small.
func RefreshCorpusHistoryGauges(ctx context.Context, db *pgxpool.Pool) {
	if db == nil {
		return
	}

	var rollups, rollupStreams, oldestRollup, newestRollup float64
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)::float8,
		       COUNT(DISTINCT stream_id)::float8,
		       COALESCE(EXTRACT(EPOCH FROM MIN(minute_ts) FILTER (WHERE minute_ts > '2020-01-01'::timestamptz)), 0)::float8,
		       COALESCE(EXTRACT(EPOCH FROM MAX(minute_ts)), 0)::float8
		FROM analytics_minute_rollups`).Scan(&rollups, &rollupStreams, &oldestRollup, &newestRollup); err == nil {
		CorpusMinuteRollupsTotal.Set(rollups)
		CorpusMinuteRollupStreamsTotal.Set(rollupStreams)
		CorpusMinuteRollupOldestTimestamp.Set(oldestRollup)
		CorpusMinuteRollupNewestTimestamp.Set(newestRollup)
	}

	var chatMessages, chatStreams, oldestChat, newestChat float64
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)::float8,
		       COUNT(DISTINCT stream_id)::float8,
		       COALESCE(EXTRACT(EPOCH FROM MIN(minute_ts)), 0)::float8,
		       COALESCE(EXTRACT(EPOCH FROM MAX(minute_ts)), 0)::float8
		FROM analytics_vod_chat_messages`).Scan(&chatMessages, &chatStreams, &oldestChat, &newestChat); err == nil {
		CorpusVODChatMessagesTotal.Set(chatMessages)
		CorpusVODChatStreamsTotal.Set(chatStreams)
		CorpusVODChatOldestTimestamp.Set(oldestChat)
		CorpusVODChatNewestTimestamp.Set(newestChat)
	}

	rows, err := db.Query(ctx, `
		SELECT COALESCE(NULLIF(tier,''),'legacy'),
		       COALESCE(NULLIF(artifact_type,''),'unknown'),
		       COALESCE(NULLIF(export_status,''),'unknown'),
		       COUNT(*)::float8,
		       COALESCE(SUM(row_count), 0)::float8
		FROM archive_exports
		GROUP BY tier, artifact_type, export_status`)
	if err != nil {
		return
	}
	defer rows.Close()
	CorpusArchiveExportsTotal.Reset()
	CorpusArchiveExportRowsTotal.Reset()
	for rows.Next() {
		var tier, artifactType, exportStatus string
		var exports, rowCount float64
		if err := rows.Scan(&tier, &artifactType, &exportStatus, &exports, &rowCount); err != nil {
			return
		}
		tier = prometheusLabel(tier)
		artifactType = prometheusLabel(artifactType)
		exportStatus = prometheusLabel(exportStatus)
		CorpusArchiveExportsTotal.WithLabelValues(tier, artifactType, exportStatus).Set(exports)
		CorpusArchiveExportRowsTotal.WithLabelValues(tier, artifactType, exportStatus).Set(rowCount)
	}
}
