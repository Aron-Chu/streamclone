package analytics

import (
	"context"
	"log/slog"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

// SilverGateConfigFromApp maps application config into gate runtime settings.
func SilverGateConfigFromApp(cfg config.Config) SilverGateConfig {
	return normalizeSilverGateConfig(SilverGateConfig{
		Enabled:          cfg.Top500SilverGateEnabled,
		DryRun:           cfg.Top500SilverGateDryRun,
		WriteEnabled:     cfg.Top500SilverGateWriteEnabled,
		MaxCandidates:    cfg.Top500SilverGateMaxCandidates,
		MaxEnqueuePerRun: cfg.Top500SilverGateMaxEnqueuePerRun,
		Interval:         cfg.Top500SilverGateInterval,
	})
}

// ShouldStartTop500SilverGate reports whether analytics should launch the gate goroutine.
func ShouldStartTop500SilverGate(cfg config.Config) bool {
	return cfg.Top500SilverGateEnabled
}

// InitTop500SilverGateMetrics publishes configured gate flag state to Prometheus.
func InitTop500SilverGateMetrics(cfg SilverGateConfig) {
	cfg = normalizeSilverGateConfig(cfg)
	if cfg.Enabled {
		metrics.Top500SilverGateEnabled.Set(1)
	} else {
		metrics.Top500SilverGateEnabled.Set(0)
	}
	if cfg.DryRun {
		metrics.Top500SilverGateDryRun.Set(1)
	} else {
		metrics.Top500SilverGateDryRun.Set(0)
	}
	if cfg.WriteEnabled {
		metrics.Top500SilverGateWriteEnabled.Set(1)
	} else {
		metrics.Top500SilverGateWriteEnabled.Set(0)
	}
}

// StartTop500SilverGate launches a non-blocking ticker loop when enabled.
// LOAD-003a: tick body is intentionally empty — no candidate reads or enqueue.
func StartTop500SilverGate(ctx context.Context, cfg SilverGateConfig, log *slog.Logger) {
	cfg = normalizeSilverGateConfig(cfg)
	if !cfg.Enabled {
		return
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultTop500SilverGateInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "tick").Inc()
				if log != nil {
					log.Debug("top500 silver gate tick (scaffold; no enqueue)",
						"dry_run", cfg.DryRun,
						"write_enabled", cfg.WriteEnabled,
					)
				}
			}
		}
	}()
}
