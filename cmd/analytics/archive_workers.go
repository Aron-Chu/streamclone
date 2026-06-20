package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/analytics"
	"streamclone/internal/archive"
	"streamclone/internal/config"
)

func startArchiveWorkers(
	ctx context.Context,
	cfg config.Config,
	pool *pgxpool.Pool,
	syncService *analytics.SyncService,
	syncExporter *archive.SyncExporter,
	archiveWriter *archive.Writer,
	logger *slog.Logger,
) {
	if syncService != nil && archiveWriter != nil {
		syncService.WithArchiveTTDetailExporter(archiveWriter)
	}
	if !cfg.ArchiveEnabled || syncExporter == nil || pool == nil {
		return
	}
	interval := cfg.ArchiveExportInterval
	if interval <= 0 {
		interval = time.Hour
	}
	worker := archive.NewIncrementalExportWorker(pool, syncExporter, 25)
	archive.StartIncrementalExportWorker(ctx, worker, interval, logger)
	logger.Info("archive incremental export worker started", "interval", interval.String())
}
