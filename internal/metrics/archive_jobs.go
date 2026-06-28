package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ArchiveJobsRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_archive_jobs_running",
		Help: "Archive jobs currently running by tier and job type.",
	}, []string{"tier", "job_type"})
	ArchiveJobsQueued = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_archive_jobs_queued",
		Help: "Archive jobs queued by tier and job type.",
	}, []string{"tier", "job_type"})
	ArchiveJobsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_archive_jobs_completed_total",
		Help: "Archive jobs completed by tier and job type.",
	}, []string{"tier", "job_type"})
	ArchiveJobsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_archive_jobs_failed_total",
		Help: "Archive jobs failed by tier and job type.",
	}, []string{"tier", "job_type"})
	ArchiveItemsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_archive_items_completed_total",
		Help: "Archive job items completed by tier and artifact type.",
	}, []string{"tier", "artifact_type"})
	ArchiveItemsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_archive_items_failed_total",
		Help: "Archive job items failed by tier and artifact type.",
	}, []string{"tier", "artifact_type"})
	ArchiveCoverageRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamclone_archive_coverage_ratio",
		Help: "Latest archive coverage ratio by tier.",
	}, []string{"tier"})
	ArchiveUploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_archive_uploads_total",
		Help: "Confirmed archive uploads by provider, tier, and artifact type.",
	}, []string{"provider", "tier", "artifact_type"})
)

// RefreshArchiveJobGauges loads archive_jobs counters from Postgres (TASK-026).
func RefreshArchiveJobGauges(ctx context.Context, db *pgxpool.Pool) {
	if db == nil {
		return
	}
	ArchiveJobsRunning.Reset()
	ArchiveJobsQueued.Reset()
	rows, err := db.Query(ctx, `
		SELECT COALESCE(tier,'unknown'), COALESCE(job_type,'unknown'), status, COUNT(*)::float8
		FROM archive_jobs
		GROUP BY tier, job_type, status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tier, jobType, status string
		var count float64
		if err := rows.Scan(&tier, &jobType, &status, &count); err != nil {
			return
		}
		tier = prometheusLabel(tier)
		jobType = prometheusLabel(jobType)
		switch strings.ToLower(status) {
		case "running":
			ArchiveJobsRunning.WithLabelValues(tier, jobType).Add(count)
		case "queued", "pending":
			ArchiveJobsQueued.WithLabelValues(tier, jobType).Add(count)
		case "done", "completed":
			ArchiveJobsCompletedTotal.WithLabelValues(tier, jobType).Add(count)
		case "failed", "cancelled":
			ArchiveJobsFailedTotal.WithLabelValues(tier, jobType).Add(count)
		}
	}
	itemRows, err := db.Query(ctx, `
		SELECT COALESCE(j.tier,'unknown'), COALESCE(i.artifact_type,'unknown'), i.status, COUNT(*)::float8
		FROM archive_job_items i
		JOIN archive_jobs j ON j.id = i.job_id
		GROUP BY j.tier, i.artifact_type, i.status`)
	if err != nil {
		return
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var tier, artifactType, status string
		var count float64
		if err := itemRows.Scan(&tier, &artifactType, &status, &count); err != nil {
			return
		}
		tier = prometheusLabel(tier)
		artifactType = prometheusLabel(artifactType)
		switch strings.ToLower(status) {
		case "done", "confirmed", "complete":
			ArchiveItemsCompletedTotal.WithLabelValues(tier, artifactType).Add(count)
		case "failed", "error":
			ArchiveItemsFailedTotal.WithLabelValues(tier, artifactType).Add(count)
		}
	}
	covRows, err := db.Query(ctx, `
		SELECT COALESCE(tier,'unknown'),
			CASE WHEN COUNT(*) = 0 THEN 0
				ELSE COUNT(*) FILTER (WHERE export_status = 'confirmed')::float8 / COUNT(*)::float8
			END AS ratio
		FROM archive_exports
		WHERE tier IS NOT NULL AND tier <> ''
		GROUP BY tier`)
	if err != nil {
		return
	}
	defer covRows.Close()
	ArchiveCoverageRatio.Reset()
	for covRows.Next() {
		var tier string
		var ratio float64
		if err := covRows.Scan(&tier, &ratio); err != nil {
			return
		}
		ArchiveCoverageRatio.WithLabelValues(prometheusLabel(tier)).Set(ratio)
	}
}

func StartArchiveMetricsRefresh(ctx context.Context, db *pgxpool.Pool, interval time.Duration) {
	if db == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		refresh := func() {
			RefreshBackfillJobGauges(ctx, db)
			RefreshArchiveJobGauges(ctx, db)
			RefreshTop500VODInventoryGauges(ctx, db)
			RefreshCorpusHistoryGauges(ctx, db)
		}
		refresh()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

func RecordArchiveUpload(provider, tier, artifactType string) {
	ArchiveUploadsTotal.WithLabelValues(
		prometheusLabel(provider),
		prometheusLabel(tier),
		prometheusLabel(artifactType),
	).Inc()
}
