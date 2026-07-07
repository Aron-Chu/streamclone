package render

import (
	"strings"

	appconfig "streamclone/internal/config"
)

// Config controls demand-driven emote rendering behavior.
type Config struct {
	TwitchEager                 bool
	ThirdpartyEager             bool
	OnChatObserved              bool
	OnUIRequest                 bool
	BackfillEnabled             bool
	DefaultScales               []string
	AllowedScales               []string
	QueueMaxDepth               int
	ChatObservedRateLimitPerMin int
	UIRequestRateLimitPerMin    int
}

func ConfigFromApp(cfg appconfig.Config) Config {
	return Config{
		TwitchEager:                 cfg.EmoteRenderTwitchEager,
		ThirdpartyEager:             cfg.EmoteRenderThirdpartyEager,
		OnChatObserved:              cfg.EmoteRenderOnChatObserved,
		OnUIRequest:                 cfg.EmoteRenderOnUIRequest,
		BackfillEnabled:             cfg.EmoteRenderBackfillEnabled,
		DefaultScales:               parseScaleList(cfg.EmoteRenderDefaultScales, []string{"1x"}),
		AllowedScales:               parseScaleList(cfg.EmoteRenderAllowedScales, []string{"1x", "2x", "3x", "4x"}),
		QueueMaxDepth:               maxInt(cfg.EmoteRenderQueueMaxDepth, 1),
		ChatObservedRateLimitPerMin: maxInt(cfg.EmoteRenderChatObservedRateLimitPerMin, 0),
		UIRequestRateLimitPerMin:    maxInt(cfg.EmoteRenderUIRequestRateLimitPerMin, 0),
	}
}

func parseScaleList(raw string, fallback []string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		scale := strings.TrimSpace(part)
		if scale == "" {
			continue
		}
		if _, ok := seen[scale]; ok {
			continue
		}
		seen[scale] = struct{}{}
		out = append(out, scale)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func maxInt(a, b int) int {
	if a < b {
		return b
	}
	return a
}
