package chatreplay

import (
	"context"
	"log/slog"
	"time"
)

// DefaultRetentionDays is the fallback retention window applied when the
// configured ANALYTICS_VOD_CHAT_RETENTION_DAYS is non-positive (Requirement 30.2).
const DefaultRetentionDays = 90

// retentionInterval is how often the worker runs its purge. The requirement is
// "at least once per 24h"; running on a 24h ticker (plus an initial purge on
// Start) satisfies that bound.
const retentionInterval = 24 * time.Hour

// RetentionWorker periodically purges persisted VOD chat messages older than the
// configured retention window. It mirrors the analytics Collector cleanup-ticker
// scheduling style: an initial purge on Start followed by a fixed-interval
// ticker, both cancelled by the context.
//
// For privacy (Requirement 30.3) the worker logs only the stream id and purged
// row count, never message content.
type RetentionWorker struct {
	store     *Store
	retention time.Duration
	interval  time.Duration
	log       *slog.Logger
}

// NewRetentionWorker constructs a RetentionWorker. retentionDays is the
// env-driven ANALYTICS_VOD_CHAT_RETENTION_DAYS value; non-positive values fall
// back to DefaultRetentionDays (90). A nil store/logger is tolerated so the
// worker is safe to wire even when chat replay persistence is disabled.
func NewRetentionWorker(store *Store, retentionDays int, logger *slog.Logger) *RetentionWorker {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionWorker{
		store:     store,
		retention: time.Duration(retentionDays) * 24 * time.Hour,
		interval:  retentionInterval,
		log:       logger,
	}
}

// Start launches the periodic purge loop in a background goroutine. It runs one
// purge immediately, then once per interval until ctx is cancelled. Start is a
// no-op when the worker has no backing store.
func (w *RetentionWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.store.db == nil {
		return
	}
	go w.loop(ctx)
}

func (w *RetentionWorker) loop(ctx context.Context) {
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

// purge deletes messages older than the retention window and logs the outcome.
// It logs purged counts per stream-agnostic batch; the DELETE spans all streams,
// so the log records the cutoff and total purged count without any content.
func (w *RetentionWorker) purge(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-w.retention)
	purged, err := w.store.PurgeOlderThan(ctx, cutoff)
	if err != nil {
		w.log.Warn("vod chat retention purge failed", "err", err, "cutoff", cutoff)
		return
	}
	if purged > 0 {
		w.log.Info("vod chat retention purge", "cutoff", cutoff, "purged", purged)
	}
}
