package analytics

import (
	"context"
	"log/slog"
	"time"

	"streamclone/internal/config"
)

// Top500SamplerConfigFromApp maps application config into sampler runtime settings.
func Top500SamplerConfigFromApp(cfg config.Config) Top500SamplerConfig {
	return normalizeTop500SamplerConfig(Top500SamplerConfig{
		Enabled:         cfg.Top500MetadataEnabled,
		DryRun:          cfg.Top500MetadataDryRun,
		WriteEnabled:    cfg.Top500MetadataWriteEnabled,
		TopN:            cfg.Top500MetadataTopN,
		BatchSize:       cfg.Top500MetadataBatchSize,
		LiveInterval:    cfg.Top500MetadataLiveInterval,
		OfflineInterval: cfg.Top500MetadataOfflineInterval,
	})
}

// ShouldStartTop500MetadataSampler reports whether analytics should launch the sampler goroutine.
func ShouldStartTop500MetadataSampler(cfg config.Config) bool {
	return cfg.Top500MetadataEnabled
}

// InitTop500MetadataSamplerMetrics publishes configured sampler flag state to Prometheus.
func InitTop500MetadataSamplerMetrics(cfg Top500SamplerConfig) {
	recordTop500SamplerConfigMetrics(normalizeTop500SamplerConfig(cfg))
}

// NewTop500MetadataProvider selects the runtime metadata provider. Fixture mode is
// local-only and must remain disabled outside approved dry-run rehearsals.
func NewTop500MetadataProvider(fixtureEnabled bool, helix *HelixClient) Top500MetadataProvider {
	if fixtureEnabled {
		return NewFixtureTop500MetadataProvider()
	}
	return NewHelixTop500MetadataProvider(helix)
}

// StartTop500MetadataSampler launches a non-blocking ticker loop for sampler ticks.
// When the sampler is nil or disabled, this is a no-op and does not start a goroutine.
func StartTop500MetadataSampler(ctx context.Context, sampler *Top500MetadataSampler, tickInterval time.Duration, log *slog.Logger) {
	if sampler == nil || !sampler.enabled() {
		return
	}
	if tickInterval <= 0 {
		tickInterval = DefaultTop500MetadataLiveInterval
	}
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := sampler.RunTick(ctx, time.Now().UTC()); err != nil && log != nil {
					log.Warn("top500 metadata sampler tick failed", "err", err)
				}
			}
		}
	}()
}

func (s *Top500MetadataSampler) enabled() bool {
	return s != nil && normalizeTop500SamplerConfig(s.cfg).Enabled
}
