package heatmap

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// TestPropCacheKeyDeterminism is a property-based test for CacheKey determinism.
// It validates three properties:
//
// 1. Deterministic: calling CacheKey with the same inputs always produces the same string.
// 2. Distinct: if any single input differs, the key differs (no collisions for different params).
// 3. Format: output matches heatmap:{streamID}:{version}:{updatedAtMs}:{window}.
//
// Feature: moment-timeline, Property 26: Cache Key Determinism
//
// **Validates: Requirements 29.1**
func TestPropCacheKeyDeterminism(t *testing.T) {
	// Property 1: Deterministic — same inputs produce the same key every time.
	t.Run("deterministic", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			streamID := rapid.StringMatching(`[a-zA-Z0-9_-]{1,64}`).Draw(t, "streamID")
			version := rapid.StringMatching(`v[0-9]{1,4}`).Draw(t, "version")
			updatedAtMs := rapid.Int64Range(0, 1e15).Draw(t, "updatedAtMs")
			window := rapid.IntRange(1, 3600).Draw(t, "window")

			key1 := CacheKey(streamID, version, updatedAtMs, window)
			key2 := CacheKey(streamID, version, updatedAtMs, window)

			if key1 != key2 {
				t.Fatalf("CacheKey not deterministic: %q != %q", key1, key2)
			}
		})
	})

	// Property 2: Distinct — if any input differs, the key differs.
	t.Run("distinct", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			streamID := rapid.StringMatching(`[a-zA-Z0-9_-]{1,64}`).Draw(t, "streamID")
			version := rapid.StringMatching(`v[0-9]{1,4}`).Draw(t, "version")
			updatedAtMs := rapid.Int64Range(0, 1e15).Draw(t, "updatedAtMs")
			window := rapid.IntRange(1, 3600).Draw(t, "window")

			base := CacheKey(streamID, version, updatedAtMs, window)

			// Vary streamID
			otherStreamID := rapid.StringMatching(`[a-zA-Z0-9_-]{1,64}`).
				Filter(func(s string) bool { return s != streamID }).
				Draw(t, "otherStreamID")
			if k := CacheKey(otherStreamID, version, updatedAtMs, window); k == base {
				t.Fatalf("different streamID produced same key: streamID=%q otherStreamID=%q key=%q", streamID, otherStreamID, k)
			}

			// Vary version
			otherVersion := rapid.StringMatching(`v[0-9]{1,4}`).
				Filter(func(s string) bool { return s != version }).
				Draw(t, "otherVersion")
			if k := CacheKey(streamID, otherVersion, updatedAtMs, window); k == base {
				t.Fatalf("different version produced same key: version=%q otherVersion=%q key=%q", version, otherVersion, k)
			}

			// Vary updatedAtMs
			otherUpdatedAtMs := rapid.Int64Range(0, 1e15).
				Filter(func(v int64) bool { return v != updatedAtMs }).
				Draw(t, "otherUpdatedAtMs")
			if k := CacheKey(streamID, version, otherUpdatedAtMs, window); k == base {
				t.Fatalf("different updatedAtMs produced same key: updatedAtMs=%d other=%d key=%q", updatedAtMs, otherUpdatedAtMs, k)
			}

			// Vary window
			otherWindow := rapid.IntRange(1, 3600).
				Filter(func(v int) bool { return v != window }).
				Draw(t, "otherWindow")
			if k := CacheKey(streamID, version, updatedAtMs, otherWindow); k == base {
				t.Fatalf("different window produced same key: window=%d other=%d key=%q", window, otherWindow, k)
			}
		})
	})

	// Property 3: Format — output matches heatmap:{streamID}:{version}:{updatedAtMs}:{window}.
	t.Run("format", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			streamID := rapid.StringMatching(`[a-zA-Z0-9_-]{1,64}`).Draw(t, "streamID")
			version := rapid.StringMatching(`v[0-9]{1,4}`).Draw(t, "version")
			updatedAtMs := rapid.Int64Range(0, 1e15).Draw(t, "updatedAtMs")
			window := rapid.IntRange(1, 3600).Draw(t, "window")

			key := CacheKey(streamID, version, updatedAtMs, window)
			expected := fmt.Sprintf("heatmap:%s:%s:%d:%d", streamID, version, updatedAtMs, window)

			if key != expected {
				t.Fatalf("CacheKey format mismatch:\n  got:  %q\n  want: %q", key, expected)
			}
		})
	})
}
