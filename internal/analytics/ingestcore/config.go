package ingestcore

import (
	"os"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/config"
)

// Config holds ingest-core tuning and feature flags.
type Config struct {
	CoreEnabled        bool
	ShadowMode         bool
	DualReadMode       bool
	TieringEnabled     bool
	HubRosterLimit     int
	P1HotLimit         int
	MaxActiveIRC       int
	ShardCount         int
	P0QueueReserve     int
	IRCQueueSize       int
	ShardQueueSize     int
	FlushQueueSize     int
	FlushInterval      time.Duration
	FlushMaxBatch      int
	OpenMinuteFlush    time.Duration
	ShadowAllowlist    map[string]struct{}
	ShadowTolerancePct float64
	ShadowArtifactDir  string
	TopEmotesPerMinute int
	ShadowDebug        bool
}

const (
	defaultShardCount      = 32
	defaultIRCQueueSize    = 8192
	defaultP0QueueReserve  = 2048
	defaultShardQueueSize  = 256
	defaultFlushQueueSize  = 4096
	defaultFlushInterval   = 5 * time.Second
	defaultFlushMaxBatch   = 500
	defaultOpenMinuteFlush = 10 * time.Second
	defaultHubRosterLimit  = 250
	defaultP1HotLimit      = 50
	defaultShadowTolerance = 2.0
)

// ConfigFromApp merges ingest env vars with the analytics app config.
func ConfigFromApp(app config.Config) Config {
	maxIRC := app.MaxConcurrentTrackedChannels
	if app.PulseMaxActiveChannels > 0 {
		maxIRC = app.PulseMaxActiveChannels
	}
	if app.MaxActiveIRCChannels > 0 {
		maxIRC = app.MaxActiveIRCChannels
	}
	cfg := Config{
		CoreEnabled:        envBool("INGEST_CORE_ENABLED", false),
		ShadowMode:         envBool("INGEST_CORE_SHADOW_MODE", false),
		DualReadMode:       envBool("INGEST_CORE_DUAL_READ_MODE", false),
		TieringEnabled:     envBool("INGEST_TIERING_ENABLED", false),
		HubRosterLimit:     envInt("HUB_ROSTER_LIMIT", defaultHubRosterLimit),
		P1HotLimit:         envInt("INGEST_P1_HOT_LIMIT", defaultP1HotLimit),
		MaxActiveIRC:       maxIRC,
		ShardCount:         envInt("INGEST_SHARD_COUNT", defaultShardCount),
		P0QueueReserve:     envInt("INGEST_P0_QUEUE_RESERVE", defaultP0QueueReserve),
		IRCQueueSize:       envInt("INGEST_IRC_QUEUE_SIZE", defaultIRCQueueSize),
		ShardQueueSize:     envInt("INGEST_SHARD_QUEUE_SIZE", defaultShardQueueSize),
		FlushQueueSize:     envInt("INGEST_FLUSH_QUEUE_SIZE", defaultFlushQueueSize),
		FlushInterval:      envDuration("INGEST_FLUSH_INTERVAL", defaultFlushInterval),
		FlushMaxBatch:      envInt("INGEST_FLUSH_MAX_BATCH", defaultFlushMaxBatch),
		OpenMinuteFlush:    envDuration("INGEST_OPEN_MINUTE_FLUSH_INTERVAL", defaultOpenMinuteFlush),
		ShadowTolerancePct: envFloat("INGEST_SHADOW_TOLERANCE_CHAT_PCT", defaultShadowTolerance),
		ShadowArtifactDir:  strings.TrimSpace(os.Getenv("INGEST_SHADOW_ARTIFACT_DIR")),
		TopEmotesPerMinute: app.AnalyticsTopEmotesPerMinute,
	}
	if cfg.ShadowArtifactDir == "" {
		cfg.ShadowArtifactDir = "runtime/ingest-shadow"
	}
	if cfg.TopEmotesPerMinute <= 0 {
		cfg.TopEmotesPerMinute = 200
	}
	if cfg.ShardCount < 1 {
		cfg.ShardCount = 1
	}
	if cfg.MaxActiveIRC <= 0 {
		cfg.MaxActiveIRC = 50
	}
	cfg.ShadowAllowlist = parseAllowlist(os.Getenv("INGEST_SHADOW_CHANNEL_ALLOWLIST"))
	cfg.ShadowDebug = envBool("INGEST_SHADOW_DEBUG", false)
	setShadowDebugActive(cfg.ShadowDebug)
	return cfg
}

// Active returns true when ingest-core should run (any mode).
func (c Config) Active() bool {
	return c.CoreEnabled || c.ShadowMode || c.DualReadMode
}

// WritesProduction returns true when ingest-core may write rollups to Postgres.
func (c Config) WritesProduction() bool {
	return c.CoreEnabled && !c.ShadowMode && !c.DualReadMode
}

func parseAllowlist(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	return raw == "1" || raw == "true" || raw == "yes"
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
