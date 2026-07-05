package render

import (
	"fmt"
	"strings"
)

const jobKeySep = "@"

// JobSourceKey encodes source hash and requested render scales for idempotent job rows.
func JobSourceKey(sourceHash string, scales []string) string {
	sourceHash = strings.TrimSpace(sourceHash)
	if len(scales) == 0 {
		return sourceHash
	}
	return sourceHash + jobKeySep + strings.Join(scales, ",")
}

// ParseJobSourceKey splits a processing_jobs.source_key into hash and scale list.
func ParseJobSourceKey(sourceKey string) (sourceHash string, scales []string) {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		return "", nil
	}
	parts := strings.SplitN(sourceKey, jobKeySep, 2)
	sourceHash = parts[0]
	if len(parts) == 1 || strings.TrimSpace(parts[1]) == "" {
		return sourceHash, nil
	}
	for _, scale := range strings.Split(parts[1], ",") {
		scale = strings.TrimSpace(scale)
		if scale != "" {
			scales = append(scales, scale)
		}
	}
	return sourceHash, scales
}

// ResolveScales picks worker output scales using job key, defaults, and allowlist.
func ResolveScales(jobScales, defaults, allowed []string) []string {
	requested := jobScales
	if len(requested) == 0 {
		requested = defaults
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, scale := range allowed {
		allowedSet[scale] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, scale := range requested {
		if _, ok := allowedSet[scale]; !ok {
			continue
		}
		if _, ok := seen[scale]; ok {
			continue
		}
		seen[scale] = struct{}{}
		out = append(out, scale)
	}
	if len(out) == 0 && len(defaults) > 0 {
		if _, ok := allowedSet[defaults[0]]; ok {
			return []string{defaults[0]}
		}
	}
	return out
}

// ObserveRedisKey is the Redis list key for cross-service chat-observed render hints.
const ObserveRedisKey = "emote:render:observe"

// ObservePayload is JSON pushed by chat tokenization for async render enqueue.
type ObservePayload struct {
	EmoteID         string `json:"emote_id"`
	Provider        string `json:"provider"`
	ProviderEmoteID string `json:"provider_emote_id,omitempty"`
	ChannelLogin    string `json:"channel_login,omitempty"`
	Scale           string `json:"scale,omitempty"`
}

func (p ObservePayload) validate() error {
	if strings.TrimSpace(p.EmoteID) == "" {
		return fmt.Errorf("missing emote_id")
	}
	if strings.TrimSpace(p.Provider) == "" {
		return fmt.Errorf("missing provider")
	}
	return nil
}
