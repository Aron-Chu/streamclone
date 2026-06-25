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
// LOAD-003c: each tick evaluates candidates in dry-run when write is disabled.
func StartTop500SilverGate(ctx context.Context, gate *Top500SilverGate) {
	if gate == nil || !gate.cfg.Enabled {
		return
	}
	interval := gate.cfg.Interval
	if interval <= 0 {
		interval = DefaultTop500SilverGateInterval
	}
	go func() {
		runSilverGateTick(ctx, gate)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runSilverGateTick(ctx, gate)
			}
		}
	}()
}

func runSilverGateTick(ctx context.Context, gate *Top500SilverGate) {
	metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "tick").Inc()
	if _, err := gate.RunTick(ctx); err != nil && gate.log != nil {
		gate.log.Warn("top500 silver gate tick failed", "err", err)
	}
}

// RunTop500SilverGateOnce evaluates one gate window synchronously (local dry-run harness).
func RunTop500SilverGateOnce(ctx context.Context, gate *Top500SilverGate) (SilverGateTickSummary, error) {
	if gate == nil {
		return SilverGateTickSummary{Decisions: map[SilverGateDecisionReason]int{}}, nil
	}
	metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "tick").Inc()
	return gate.RunTick(ctx)
}

// LogSilverGateStartup logs the expected startup line for local dry-run.
func LogSilverGateStartup(log *slog.Logger, cfg SilverGateConfig, fixtureCandidates, realCounterReader bool) {
	if log == nil {
		return
	}
	log.Info("top500 silver gate started",
		"dry_run", cfg.DryRun,
		"write_enabled", cfg.WriteEnabled,
		"max_candidates", cfg.MaxCandidates,
		"max_enqueue_per_run", cfg.MaxEnqueuePerRun,
		"interval", cfg.Interval.String(),
		"fixture_candidates", fixtureCandidates,
		"real_counter_reader", realCounterReader,
	)
}
