package analytics

import (
	"os"
	"strings"
	"time"
)

type PulseRuntimeConfig struct {
	Configured              bool
	HelixLiveEnabled        bool
	HelixVodEnabled         bool
	HelixMetadataEnabled    bool
	HelixGoLiveEnabled      bool
	GQLCommentsEnabled      bool
	BackfillEnabled         bool
	ProtectedGoLiveEnabled  bool
	TopRosterPollEnabled    bool
	ContextEnrichment       bool
	EmoteEnsureBlocking     bool
	BFFCacheEnabled         bool
	ReadOnlyMode            bool
	RosterSize              int
	ProtectedGlobalLimit    int
	ProtectedGoLiveInterval time.Duration
	TopRosterPollInterval   time.Duration
	GoLiveBatchSize         int
}

func DefaultPulseRuntimeConfig() PulseRuntimeConfig {
	return PulseRuntimeConfig{
		Configured:           true,
		HelixLiveEnabled:     true,
		HelixVodEnabled:      true,
		HelixMetadataEnabled: true,
		HelixGoLiveEnabled:   true,
		GQLCommentsEnabled:   true,
		BackfillEnabled:      true,
		BFFCacheEnabled:      true,
		RosterSize:           500,
		ProtectedGlobalLimit: 500,
	}
}

func PulseRuntimeConfigFromEnv() PulseRuntimeConfig {
	cfg := DefaultPulseRuntimeConfig()
	if raw, ok := os.LookupEnv("PULSE_HELIX_ENABLED"); ok && !envBool(raw, true) {
		cfg.HelixLiveEnabled = false
		cfg.HelixVodEnabled = false
		cfg.HelixMetadataEnabled = false
		cfg.HelixGoLiveEnabled = false
	} else {
		cfg.HelixLiveEnabled = envBoolDefault("PULSE_HELIX_LIVE_ENABLED", cfg.HelixLiveEnabled)
		cfg.HelixVodEnabled = envBoolDefault("PULSE_HELIX_VOD_ENABLED", cfg.HelixVodEnabled)
		cfg.HelixMetadataEnabled = envBoolDefault("PULSE_HELIX_METADATA_ENABLED", cfg.HelixMetadataEnabled)
		cfg.HelixGoLiveEnabled = envBoolDefault("PULSE_HELIX_GOLIVE_ENABLED", cfg.HelixGoLiveEnabled)
	}
	cfg.GQLCommentsEnabled = envBoolDefault("PULSE_GQL_COMMENTS_ENABLED", cfg.GQLCommentsEnabled)
	cfg.BackfillEnabled = envBoolDefault("PULSE_BACKFILL_ENABLED", cfg.BackfillEnabled)
	cfg.ProtectedGoLiveEnabled = envBoolDefault("PULSE_PROTECTED_GOLIVE_ENABLED", cfg.ProtectedGoLiveEnabled)
	cfg.TopRosterPollEnabled = envBoolDefault("PULSE_TOP_ROSTER_POLL_ENABLED", cfg.TopRosterPollEnabled)
	cfg.ContextEnrichment = envBoolDefault("PULSE_CONTEXT_ENRICHMENT_ENABLED", cfg.ContextEnrichment)
	cfg.EmoteEnsureBlocking = envBoolDefault("PULSE_EMOTE_ENSURE_BLOCKING", cfg.EmoteEnsureBlocking)
	cfg.BFFCacheEnabled = envBoolDefault("PULSE_BFF_CACHE_ENABLED", cfg.BFFCacheEnabled)
	cfg.ReadOnlyMode = envBoolDefault("PULSE_READ_ONLY_MODE", cfg.ReadOnlyMode)
	cfg.RosterSize = envIntDefault("PULSE_ROSTER_SIZE", cfg.RosterSize)
	cfg.ProtectedGlobalLimit = envIntDefault("PULSE_PROTECTED_CHANNEL_LIMIT_GLOBAL", cfg.ProtectedGlobalLimit)
	cfg.ProtectedGoLiveInterval = envDurationDefault("PULSE_PROTECTED_GOLIVE_INTERVAL", 60*time.Second)
	cfg.TopRosterPollInterval = envDurationDefault("PULSE_TOP_ROSTER_INTERVAL", 180*time.Second)
	cfg.GoLiveBatchSize = envIntDefault("PULSE_GOLIVE_BATCH_SIZE", 100)
	return cfg
}

func (cfg PulseRuntimeConfig) withDefaults() PulseRuntimeConfig {
	if !cfg.Configured {
		return DefaultPulseRuntimeConfig()
	}
	if cfg.RosterSize <= 0 {
		cfg.RosterSize = 500
	}
	if cfg.ProtectedGlobalLimit <= 0 {
		cfg.ProtectedGlobalLimit = 500
	}
	if cfg.GoLiveBatchSize <= 0 {
		cfg.GoLiveBatchSize = 100
	}
	return cfg
}

func envDurationDefault(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func envBoolDefault(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return envBool(raw, fallback)
}

func envBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
