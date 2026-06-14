package heatmap

import (
	"sort"
	"strings"
)

// maxTopEmotes is the upper bound on the number of emotes attached to a scored
// window (Requirement 10.3: 1–3 entries). ComputeHeatmap passes 3.
const maxTopEmotes = 3

// splitEmoteKey decomposes a rollup emote key of the form "provider:id:name"
// into its parts. It mirrors analytics.splitEmoteKey so the heatmap package can
// stay pure (no analytics import) while producing identical identity fields:
//   - 3 parts ("provider:id:name") -> provider, id, name
//   - 2 parts ("provider:id")      -> provider, id, name=id
//   - otherwise                    -> name=key, id="", provider=""
//
// id is the local emote-service id (the MinIO object path component), not the
// 7TV provider id, so it is the correct value for the /emotes/{id}/1x.webp URL.
func splitEmoteKey(key string) (provider, id, name string) {
	parts := strings.SplitN(key, ":", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], parts[1]
	default:
		return "", "", key
	}
}

// emoteImageURL builds the local emote-service image URL for a window emote.
// The id is the local emote id stored in the rollup key; an empty id yields an
// empty URL rather than a malformed "/emotes//1x.webp" path.
func emoteImageURL(id string) string {
	if id == "" {
		return ""
	}
	return "/emotes/" + id + "/1x.webp"
}

// topEmotes returns the top emotes for a scoring window, ordered by per-window
// use count descending (Requirement 10.3). At most limit entries are returned
// (clamped to maxTopEmotes); an empty or nil map returns nil so windows with no
// emote data omit the field entirely.
//
// Ordering is fully deterministic (Requirement 9.6): emotes are sorted by count
// descending, and ties are broken by the raw rollup key ascending. Each entry's
// "provider:id:name" key is parsed into ID, Name, and Provider, and ImageURL is
// set to /emotes/{id}/1x.webp using the local emote id.
func topEmotes(emotes map[string]int, limit int) []HeatmapEmote {
	if len(emotes) == 0 || limit <= 0 {
		return nil
	}
	if limit > maxTopEmotes {
		limit = maxTopEmotes
	}

	keys := make([]string, 0, len(emotes))
	for key := range emotes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if emotes[keys[i]] != emotes[keys[j]] {
			return emotes[keys[i]] > emotes[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if len(keys) > limit {
		keys = keys[:limit]
	}

	out := make([]HeatmapEmote, 0, len(keys))
	for _, key := range keys {
		provider, id, name := splitEmoteKey(key)
		out = append(out, HeatmapEmote{
			ID:       id,
			Name:     name,
			ImageURL: emoteImageURL(id),
			Count:    emotes[key],
			Provider: provider,
		})
	}
	return out
}
