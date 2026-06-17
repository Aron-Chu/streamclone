package chatreplay

import (
	"context"
	"log/slog"
	"time"
)

const defaultLiveRetentionDays = 14

// LiveRetentionWorker purges live chat messages and mod events on a fixed interval.
type LiveRetentionWorker struct {
	store     *Store
	retention time.Duration
	interval  time.Duration
	log       *slog.Logger
}

func NewLiveRetentionWorker(store *Store, retentionDays int, logger *slog.Logger) *LiveRetentionWorker {
	if retentionDays <= 0 {
		retentionDays = defaultLiveRetentionDays
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LiveRetentionWorker{
		store:     store,
		retention: time.Duration(retentionDays) * 24 * time.Hour,
		interval:  retentionInterval,
		log:       logger,
	}
}

func (w *LiveRetentionWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.store.db == nil {
		return
	}
	go w.loop(ctx)
}

func (w *LiveRetentionWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.purge(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.purge(ctx)
		}
	}
}

func (w *LiveRetentionWorker) purge(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-w.retention)
	purged, err := w.store.PurgeLiveRetention(ctx, cutoff)
	if err != nil {
		w.log.Warn("live chat retention purge failed", "err", err, "cutoff", cutoff)
		return
	}
	if purged > 0 {
		w.log.Info("live chat retention purge", "cutoff", cutoff, "purged", purged)
	}
}
