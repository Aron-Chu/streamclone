package archive

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultIncrementalExportBatch = 25

// IncrementalExportWorker scans streams whose rollups lack a confirmed manifest row
// (or were updated after the last export) and uploads them via SyncExporter.
type IncrementalExportWorker struct {
	db       *pgxpool.Pool
	exporter *SyncExporter
	limit    int
}

func NewIncrementalExportWorker(db *pgxpool.Pool, exporter *SyncExporter, limit int) *IncrementalExportWorker {
	if limit <= 0 {
		limit = defaultIncrementalExportBatch
	}
	return &IncrementalExportWorker{db: db, exporter: exporter, limit: limit}
}

type pendingStreamExport struct {
	StreamID string
	Login    string
}

func rollupsNaturalKey(streamID string) string {
	return fmt.Sprintf("rollups:%s", streamID)
}

func (w *IncrementalExportWorker) listPending(ctx context.Context) ([]pendingStreamExport, error) {
	if w == nil || w.db == nil {
		return nil, nil
	}
	limit := w.limit
	if limit <= 0 {
		limit = defaultIncrementalExportBatch
	}
	rows, err := w.db.Query(ctx, `
		SELECT s.stream_id, s.login
		FROM analytics_streams s
		INNER JOIN (
			SELECT stream_id, MAX(updated_at) AS rollup_updated
			FROM analytics_minute_rollups
			GROUP BY stream_id
		) r ON r.stream_id = s.stream_id
		LEFT JOIN archive_exports ae ON ae.artifact_type = $1
			AND ae.natural_key = 'rollups:' || s.stream_id
			AND ae.export_status = 'confirmed'
		WHERE ae.exported_at IS NULL OR r.rollup_updated > ae.exported_at
		ORDER BY COALESCE(ae.exported_at, TIMESTAMPTZ 'epoch') ASC, s.stream_id ASC
		LIMIT $2`, ArtifactAnalyticsRollups, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingStreamExport
	for rows.Next() {
		var item pendingStreamExport
		if err := rows.Scan(&item.StreamID, &item.Login); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (w *IncrementalExportWorker) exportStreams(ctx context.Context, pending []pendingStreamExport) (exported, failed int) {
	if w == nil || w.exporter == nil {
		return 0, 0
	}
	for _, item := range pending {
		if err := w.exporter.ExportSync(ctx, item.StreamID, item.Login, "incremental export"); err != nil {
			failed++
			continue
		}
		exported++
	}
	return exported, failed
}

// RunOnce exports up to limit pending streams. Returns counts of successes and failures.
func (w *IncrementalExportWorker) RunOnce(ctx context.Context) (exported, failed int, err error) {
	if w == nil || w.exporter == nil || w.db == nil {
		return 0, 0, nil
	}
	pending, err := w.listPending(ctx)
	if err != nil {
		return 0, 0, err
	}
	exported, failed = w.exportStreams(ctx, pending)
	return exported, failed, nil
}

func StartIncrementalExportWorker(ctx context.Context, worker *IncrementalExportWorker, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if worker == nil || worker.exporter == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func(label string) {
			exported, failed, err := worker.RunOnce(ctx)
			if err != nil && log != nil {
				log.Warn("archive incremental export tick failed", "trigger", label, "err", err)
				return
			}
			if (exported > 0 || failed > 0) && log != nil {
				log.Info("archive incremental export tick", "trigger", label, "exported", exported, "failed", failed)
			}
		}
		run("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
			}
		}
	}()
}
