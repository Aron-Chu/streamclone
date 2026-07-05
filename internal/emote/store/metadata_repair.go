package store

import "strings"

func IsProviderMetadataReady(e *Emote) bool {
	if e == nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(e.Provider))
	if provider == "" || provider == "custom" {
		return false
	}
	return strings.TrimSpace(e.ProviderEmoteID) != ""
}
