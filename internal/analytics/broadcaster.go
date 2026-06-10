package analytics

import (
	"context"
	"strings"
)

// NormalizeBroadcasterID treats placeholder values as missing.
func NormalizeBroadcasterID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.EqualFold(id, "pending") {
		return ""
	}
	return id
}

func (c *HelixClient) ResolveBroadcasterID(ctx context.Context, login, existing string) string {
	if id := NormalizeBroadcasterID(existing); id != "" {
		return id
	}
	login = normalizeLogin(login)
	if login == "" || c == nil || !c.Enabled() {
		return ""
	}
	profiles, err := c.UsersByLogin(ctx, []string{login})
	if err != nil {
		return ""
	}
	if profile, ok := profiles[login]; ok {
		return strings.TrimSpace(profile.ID)
	}
	return ""
}
