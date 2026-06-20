package store

import (
	"encoding/json"
	"strings"
)

var thumbnailMetricKeys = []string{"thumbnail_url", "thumbnail_source", "thumbnail_status"}

// MergeSocialMetrics overlays incoming onto existing and preserves thumbnail
// fields when the incoming poll omits them or sends empty strings.
func MergeSocialMetrics(existing, incoming json.RawMessage) json.RawMessage {
	base := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &base)
	}
	saved := map[string]string{}
	for _, key := range thumbnailMetricKeys {
		if v, ok := base[key].(string); ok && strings.TrimSpace(v) != "" {
			saved[key] = v
		}
	}
	overlay := map[string]any{}
	if len(incoming) > 0 {
		_ = json.Unmarshal(incoming, &overlay)
	}
	for key, value := range overlay {
		base[key] = value
	}
	for _, key := range thumbnailMetricKeys {
		if v, ok := overlay[key].(string); ok && strings.TrimSpace(v) != "" {
			continue
		}
		if saved[key] != "" {
			base[key] = saved[key]
		}
	}
	if len(base) == 0 {
		return json.RawMessage("{}")
	}
	out, err := json.Marshal(base)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

// displayThumbnailURL returns the same-origin proxy path (or direct HTTPS URL)
// clients should load for a thumbnail. Metrics Helix URLs win over stale evidence.
func displayThumbnailURL(storedThumb, metricsThumb string) string {
	raw := preferServingThumb(metricsThumb, storedThumb)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if proxied := proxiedThumbPath(raw); proxied != "" {
		return proxied
	}
	if strings.HasPrefix(strings.ToLower(raw), "https://") {
		return raw
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
