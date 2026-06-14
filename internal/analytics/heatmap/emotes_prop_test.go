package heatmap

import (
	"regexp"
	"testing"

	"pgregory.net/rapid"
)

// Feature: moment-timeline, Property 13: Top Emotes Ordering and Format
//
// topEmotes must return 1–3 entries ordered by count descending, with each
// ImageURL matching the pattern /emotes/{uuid}/1x.webp. An empty input map
// returns nil (no emotes attached). The limit parameter is clamped to
// maxTopEmotes (3) so callers cannot request more than 3.
//
// rapid runs at least 100 iterations by default.
//
// **Validates: Requirements 10.3**

// emoteURLPattern matches the expected emote image URL format produced by
// emoteImageURL: /emotes/{non-empty-id}/1x.webp where the id is the local
// emote-service UUID (alphanumeric + dashes).
var emoteURLPattern = regexp.MustCompile(`^/emotes/[a-zA-Z0-9_-]+/1x\.webp$`)

// drawEmoteMap generates an arbitrary non-empty emote map with 1–10 entries.
// Keys use the "provider:id:name" format with non-empty id components so the
// resulting ImageURL is always well-formed.
func drawEmoteMap(t *rapid.T) map[string]int {
	n := rapid.IntRange(1, 10).Draw(t, "numEmotes")
	m := make(map[string]int, n)
	for i := 0; i < n; i++ {
		provider := rapid.StringMatching(`[a-z]{2,6}`).Draw(t, "provider")
		id := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).Draw(t, "id")
		name := rapid.StringMatching(`[A-Za-z][A-Za-z0-9]{1,15}`).Draw(t, "name")
		key := provider + ":" + id + ":" + name
		count := rapid.IntRange(1, 10000).Draw(t, "count")
		m[key] = count
	}
	return m
}

// TestPropTopEmotesCountBound checks that topEmotes always returns between 1
// and 3 entries when the input map is non-empty, and never exceeds the limit or
// maxTopEmotes.
func TestPropTopEmotesCountBound(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		emotes := drawEmoteMap(t)
		limit := rapid.IntRange(1, 5).Draw(t, "limit")

		result := topEmotes(emotes, limit)

		if result == nil {
			t.Fatal("topEmotes returned nil for non-empty input")
		}

		expectedMax := limit
		if expectedMax > maxTopEmotes {
			expectedMax = maxTopEmotes
		}
		if len(emotes) < expectedMax {
			expectedMax = len(emotes)
		}

		if len(result) < 1 || len(result) > 3 {
			t.Fatalf("expected 1–3 entries, got %d", len(result))
		}
		if len(result) > expectedMax {
			t.Fatalf("expected at most %d entries, got %d", expectedMax, len(result))
		}
	})
}

// TestPropTopEmotesDescendingOrder checks that entries are ordered by count
// descending (Requirement 10.3: count descending).
func TestPropTopEmotesDescendingOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		emotes := drawEmoteMap(t)
		limit := rapid.IntRange(1, 3).Draw(t, "limit")

		result := topEmotes(emotes, limit)
		if result == nil {
			t.Fatal("topEmotes returned nil for non-empty input")
		}

		for i := 1; i < len(result); i++ {
			if result[i].Count > result[i-1].Count {
				t.Fatalf("entries not in count-descending order: index %d count %d > index %d count %d",
					i, result[i].Count, i-1, result[i-1].Count)
			}
		}
	})
}

// TestPropTopEmotesImageURLFormat checks that each emote's ImageURL matches the
// expected /emotes/{id}/1x.webp pattern.
func TestPropTopEmotesImageURLFormat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		emotes := drawEmoteMap(t)
		limit := rapid.IntRange(1, 3).Draw(t, "limit")

		result := topEmotes(emotes, limit)
		if result == nil {
			t.Fatal("topEmotes returned nil for non-empty input")
		}

		for i, e := range result {
			if !emoteURLPattern.MatchString(e.ImageURL) {
				t.Fatalf("entry %d ImageURL %q does not match pattern /emotes/{id}/1x.webp", i, e.ImageURL)
			}
		}
	})
}

// TestPropTopEmotesNilOnEmpty checks that an empty or nil emote map returns nil.
func TestPropTopEmotesNilOnEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.IntRange(1, 5).Draw(t, "limit")

		if got := topEmotes(nil, limit); got != nil {
			t.Fatalf("expected nil for nil map, got %v", got)
		}
		if got := topEmotes(map[string]int{}, limit); got != nil {
			t.Fatalf("expected nil for empty map, got %v", got)
		}
	})
}
