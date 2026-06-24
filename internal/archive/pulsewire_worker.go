package archive

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartPulseWireExportWorker exports Reddit batches before retention purge (TASK-032).
func StartPulseWireExportWorker(ctx context.Context, writer *Writer, pool *pgxpool.Pool, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if writer == nil || pool == nil {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func(label string) {
			if err := writer.ExportPulseWireSource(ctx, pool, "reddit", 500); err != nil && log != nil {
				log.Warn("pulsewire archive export failed", "trigger", label, "err", err)
				return
			}
			if log != nil {
				log.Info("pulsewire archive export tick", "trigger", label)
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
