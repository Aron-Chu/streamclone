package analytics

import (
	"log/slog"
	"strings"
)

// LogGoldIVREffectiveConfig prints the effective IVR accelerator flags at startup (no secrets).
func LogGoldIVREffectiveConfig(log *slog.Logger, cfg GoldIVRConfig, rawAllowlist string) {
	if log == nil {
		return
	}
	allowlist := allowlistSummary(cfg.Allowlist, rawAllowlist)
	log.Info("gold_ivr effective config",
		"enabled", cfg.Enabled,
		"shadow", cfg.ShadowMode,
		"lite", cfg.LiteEnabled,
		"peaks_only", cfg.PeaksOnlyEnabled,
		"canonical_replace", cfg.CanonicalReplace,
		"allowlist", allowlist,
		"shadow_artifact_dir", resolveGoldIVRShadowArtifactDir(cfg),
		"shadow_retention_days", cfg.ShadowArtifactRetentionDays,
		"shadow_max_files", cfg.ShadowArtifactMaxFiles,
		"base_url", strings.TrimSpace(cfg.BaseURL),
	)
}

func allowlistSummary(parsed GoldIVRAllowlist, raw string) string {
	if len(parsed.Logins) == 0 && len(parsed.ChannelIDs) == 0 {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return "[]"
		}
		return "[" + raw + "]"
	}
	parts := make([]string, 0, len(parsed.Logins)+len(parsed.ChannelIDs))
	for login := range parsed.Logins {
		parts = append(parts, login)
	}
	for id := range parsed.ChannelIDs {
		parts = append(parts, id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
