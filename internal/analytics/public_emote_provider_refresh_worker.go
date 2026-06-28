package analytics

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
)

type PublicEmoteProviderRefreshConfig struct {
	Enabled  bool
	Interval time.Duration
	Range    string
}

type publicEmoteProviderRefreshStore interface {
	RefreshPublicEmoteProviderMaterialization(context.Context, string, time.Time) (PublicEmoteProviderMaterializationStats, error)
}

type PublicEmoteProviderRefreshWorker struct {
	store   publicEmoteProviderRefreshStore
	cfg     PublicEmoteProviderRefreshConfig
	log     *slog.Logger
	running atomic.Bool
}

func NewPublicEmoteProviderRefreshWorker(store publicEmoteProviderRefreshStore, cfg PublicEmoteProviderRefreshConfig, log *slog.Logger) *PublicEmoteProviderRefreshWorker {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	if cfg.Range == "" {
		cfg.Range = "24h"
	}
	if log == nil {
		log = slog.Default()
	}
	return &PublicEmoteProviderRefreshWorker{store: store, cfg: cfg, log: log}
}

func (w *PublicEmoteProviderRefreshWorker) Enabled() bool {
	return w != nil && w.cfg.Enabled
}

func (w *PublicEmoteProviderRefreshWorker) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.store == nil {
		return
	}
	go func() {
		w.RunOnce(ctx, "startup")
		ticker := time.NewTicker(w.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx, "interval")
			}
		}
	}()
}

func (w *PublicEmoteProviderRefreshWorker) RunOnce(ctx context.Context, trigger string) (PublicEmoteProviderMaterializationStats, error) {
	var stats PublicEmoteProviderMaterializationStats
	if w == nil || w.store == nil {
		return stats, errPublicEmoteProviderStoreUnavailable
	}
	if !w.running.CompareAndSwap(false, true) {
		stats.Status = "skipped"
		stats.ErrorCode = "refresh_already_running"
		if w.log != nil {
			w.log.Warn("public emote provider refresh skipped", "trigger", trigger, "reason", stats.ErrorCode)
		}
		return stats, errPublicEmoteProviderRefreshRunning
	}
	defer w.running.Store(false)

	started := time.Now().UTC()
	if w.log != nil {
		w.log.Info("public emote provider refresh started", "trigger", trigger, "range", parsePublicEmotesRange(w.cfg.Range), "started_at", started)
	}
	stats, err := w.store.RefreshPublicEmoteProviderMaterialization(ctx, w.cfg.Range, started)
	if err != nil {
		if w.log != nil {
			w.log.Warn("public emote provider refresh failed",
				"trigger", trigger,
				"range", parsePublicEmotesRange(w.cfg.Range),
				"range_start", stats.RangeStart,
				"range_end", stats.RangeEnd,
				"duration", time.Since(started).String(),
				"rows_upserted", stats.RowsUpserted,
				"error_code", publicEmoteProviderErrorCode(err),
				"err", err,
			)
		}
		return stats, err
	}
	if w.log != nil {
		w.log.Info("public emote provider refresh completed",
			"trigger", trigger,
			"range", parsePublicEmotesRange(w.cfg.Range),
			"range_start", stats.RangeStart,
			"range_end", stats.RangeEnd,
			"duration", stats.Duration.String(),
			"rows_upserted", stats.RowsUpserted,
			"status", stats.Status,
		)
	}
	return stats, nil
}

func IsPublicEmoteProviderRefreshAlreadyRunning(err error) bool {
	return errors.Is(err, errPublicEmoteProviderRefreshRunning)
}
