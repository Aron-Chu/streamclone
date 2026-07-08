package ingestcore

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

// BatchWriter persists rollup snapshots (implemented by analytics store adapter).
type BatchWriter interface {
	WriteRollupBatch(ctx context.Context, open, closed []RollupSnapshot) error
}

type pendingRollup struct {
	snap     RollupSnapshot
	enqueued time.Time
}

// BatchFlusher batches rollup snapshots to Postgres on interval and max batch size.
type BatchFlusher struct {
	cfg       Config
	writer    BatchWriter
	log       *slog.Logger
	queue     chan pendingRollup
	lastOpen  map[string]time.Time
	mu        sync.Mutex
	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// NewBatchFlusher creates a flusher with bounded queue.
func NewBatchFlusher(cfg Config, writer BatchWriter, log *slog.Logger) *BatchFlusher {
	if log == nil {
		log = slog.Default()
	}
	return &BatchFlusher{
		cfg:       cfg,
		writer:    writer,
		log:       log,
		queue:     make(chan pendingRollup, cfg.FlushQueueSize),
		lastOpen:  map[string]time.Time{},
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start runs flush loop.
func (f *BatchFlusher) Start(ctx context.Context) {
	go f.loop(ctx)
}

// Stop drains and exits.
func (f *BatchFlusher) Stop(ctx context.Context) {
	close(f.stopCh)
	select {
	case <-f.stoppedCh:
	case <-ctx.Done():
	}
}

func (f *BatchFlusher) loop(ctx context.Context) {
	defer close(f.stoppedCh)
	ticker := time.NewTicker(f.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			f.drainQueue(ctx)
			return
		case <-f.stopCh:
			f.drainQueue(ctx)
			return
		case item := <-f.queue:
			f.flushOne(ctx, item)
			f.drainAvailable(ctx)
		case <-ticker.C:
			f.drainAvailable(ctx)
		}
	}
}

func (f *BatchFlusher) drainAvailable(ctx context.Context) {
	for {
		select {
		case item := <-f.queue:
			f.flushOne(ctx, item)
		default:
			return
		}
	}
}

func (f *BatchFlusher) drainQueue(ctx context.Context) {
	for {
		select {
		case item := <-f.queue:
			f.flushOne(ctx, item)
		default:
			return
		}
	}
}

// Enqueue adds rollups to the flush queue, coalescing by stream+minute when full.
func (f *BatchFlusher) Enqueue(snaps []RollupSnapshot) {
	for _, snap := range snaps {
		item := pendingRollup{snap: snap, enqueued: time.Now().UTC()}
		select {
		case f.queue <- item:
			metrics.IngestFlushQueueDepth.Set(float64(len(f.queue)))
			metrics.IngestFlushQueueAgeSeconds.Observe(time.Since(item.enqueued).Seconds())
		default:
			// Queue full: coalesce drop — Phase E may defer; Phase D treats as failure metric.
			metrics.IngestMessagesDroppedTotal.WithLabelValues("flush").Inc()
		}
	}
}

func (f *BatchFlusher) flushOne(ctx context.Context, item pendingRollup) {
	batch := []pendingRollup{item}
	for len(batch) < f.cfg.FlushMaxBatch {
		select {
		case next := <-f.queue:
			batch = append(batch, next)
		default:
			goto flush
		}
	}
flush:
	f.flushBatch(ctx, batch)
}

func (f *BatchFlusher) flushBatch(ctx context.Context, batch []pendingRollup) {
	if f.writer == nil || len(batch) == 0 {
		return
	}
	var openSnaps, closedSnaps []RollupSnapshot
	for _, item := range batch {
		if item.snap.Closed {
			closedSnaps = append(closedSnaps, item.snap)
		} else {
			openSnaps = append(openSnaps, item.snap)
		}
	}
	if err := f.writer.WriteRollupBatch(ctx, openSnaps, closedSnaps); err != nil {
		metrics.IngestPostgresWriteErrorsTotal.Inc()
		if f.log != nil {
			f.log.Warn("ingest flush batch failed", "err", err, "count", len(batch))
		}
		return
	}
	metrics.AnalyticsRollupWriteBatchSize.WithLabelValues("ingest_batch").Observe(float64(len(batch)))
	metrics.IngestFlushQueueDepth.Set(float64(len(f.queue)))
}

// LastOpenFlushMap returns the open-minute flush throttle map (owned by flusher).
func (f *BatchFlusher) LastOpenFlushMap() map[string]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastOpen == nil {
		f.lastOpen = map[string]time.Time{}
	}
	return f.lastOpen
}
