package heatmap

import (
	"encoding/json"
	"testing"
	"time"
)

// Integration-level tests for the heatmap compute + cache key flow.
// These tests validate the end-to-end behaviour of the scoring engine
// with realistic rollup data, cache key derivation, and empty-input edge cases.
//
// Requirements: 8.1, 13.2, 29.2

func TestIntegration_ComputeHeatmap_PreloadedRollups(t *testing.T) {
	base := time.Date(2026, 6, 11, 14, 0, 0, 0, time.UTC)

	rollups := []MinuteRollup{
		{MinuteTS: base, ChatCount: 120, TotalEmoteCount: 45, SevenTVEmoteCount: 30, ViewerAvg: 500, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 20, "seventv:def:LUL": 10, "twitch:ghi:PogChamp": 15}},
		{MinuteTS: base.Add(1 * time.Minute), ChatCount: 80, TotalEmoteCount: 20, SevenTVEmoteCount: 10, ViewerAvg: 520, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 10, "ffz:jkl:catJAM": 10}},
		{MinuteTS: base.Add(2 * time.Minute), ChatCount: 350, TotalEmoteCount: 200, SevenTVEmoteCount: 180, ViewerAvg: 1200, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 150, "seventv:mno:Sadge": 30, "twitch:pqr:Kappa": 20}},
		{MinuteTS: base.Add(3 * time.Minute), ChatCount: 90, TotalEmoteCount: 30, SevenTVEmoteCount: 20, ViewerAvg: 600, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 15, "seventv:def:LUL": 15}},
		{MinuteTS: base.Add(4 * time.Minute), ChatCount: 60, TotalEmoteCount: 10, SevenTVEmoteCount: 5, ViewerAvg: 480, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 5, "ffz:jkl:catJAM": 5}},
		{MinuteTS: base.Add(5 * time.Minute), ChatCount: 500, TotalEmoteCount: 300, SevenTVEmoteCount: 280, ViewerAvg: 2000, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 200, "seventv:stu:monkaS": 80, "twitch:pqr:Kappa": 20}},
		{MinuteTS: base.Add(6 * time.Minute), ChatCount: 70, TotalEmoteCount: 15, SevenTVEmoteCount: 10, ViewerAvg: 510, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 10, "seventv:def:LUL": 5}},
	}

	cfg := DefaultScoringConfig()
	resp := ComputeHeatmap(rollups, cfg)

	if resp.ScoringVersion != cfg.Version {
		t.Errorf("ScoringVersion = %q, want %q", resp.ScoringVersion, cfg.Version)
	}
	if resp.WindowSeconds != defaultWindowSeconds {
		t.Errorf("WindowSeconds = %d, want %d", resp.WindowSeconds, defaultWindowSeconds)
	}
	if resp.Confidence < 0 || resp.Confidence > 1 {
		t.Errorf("stream confidence = %f, must be in [0,1]", resp.Confidence)
	}
	if resp.Confidence == 0 {
		t.Errorf("expected non-zero stream confidence for rollups with data")
	}

	if len(resp.Points) == 0 {
		t.Fatal("expected at least one scored point from realistic rollups")
	}
	if len(resp.Points) > len(rollups) {
		t.Fatalf("point count %d exceeds rollup count %d", len(resp.Points), len(rollups))
	}

	lastOffset := -1
	for i, p := range resp.Points {
		if p.Score <= 0 || p.Score > 100 {
			t.Errorf("point[%d]: score %d must be in (0,100]; zero-score points must be omitted", i, p.Score)
		}
		if p.OffsetSeconds < 0 {
			t.Errorf("point[%d]: negative offset %d", i, p.OffsetSeconds)
		}
		if p.OffsetSeconds <= lastOffset {
			t.Errorf("point[%d]: offset %d not strictly after previous %d", i, p.OffsetSeconds, lastOffset)
		}
		lastOffset = p.OffsetSeconds
		if p.DurationSeconds != defaultWindowSeconds {
			t.Errorf("point[%d]: duration = %d, want %d", i, p.DurationSeconds, defaultWindowSeconds)
		}
		if !IsValidReason(p.Reason) {
			t.Errorf("point[%d]: invalid reason %q", i, p.Reason)
		}
		if p.Confidence < 0 || p.Confidence > 1 {
			t.Errorf("point[%d]: confidence %f out of [0,1]", i, p.Confidence)
		}
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	if len(respBytes) > 50*1024 {
		t.Errorf("response size %d exceeds 50 KB budget", len(respBytes))
	}

	var decoded HeatmapResponse
	if err := json.Unmarshal(respBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if decoded.StreamID != resp.StreamID {
		t.Errorf("decoded StreamID = %q, want %q", decoded.StreamID, resp.StreamID)
	}
	if decoded.ScoringVersion != resp.ScoringVersion {
		t.Errorf("decoded ScoringVersion = %q, want %q", decoded.ScoringVersion, resp.ScoringVersion)
	}
	if len(decoded.Points) != len(resp.Points) {
		t.Errorf("decoded points count = %d, want %d", len(decoded.Points), len(resp.Points))
	}
}

func TestIntegration_CacheKey_RevisionFlow(t *testing.T) {
	streamID := "stream-12345"
	version := "v1"
	window := 60

	updatedAt1 := int64(1718000000000) // initial timestamp
	updatedAt2 := int64(1718000060000) // after a rollup write (60s later)

	key1a := CacheKey(streamID, version, updatedAt1, window)
	key1b := CacheKey(streamID, version, updatedAt1, window)
	key2 := CacheKey(streamID, version, updatedAt2, window)

	if key1a != key1b {
		t.Fatalf("same inputs produced different keys: %q vs %q", key1a, key1b)
	}

	if key1a == key2 {
		t.Fatalf("different updatedAt should produce different keys: both = %q", key1a)
	}

	key3 := CacheKey(streamID, version, updatedAt1, 120)
	if key1a == key3 {
		t.Fatalf("different window should produce different keys: both = %q", key1a)
	}

	key4 := CacheKey("other-stream", version, updatedAt1, window)
	if key1a == key4 {
		t.Fatalf("different streamID should produce different keys: both = %q", key1a)
	}

	key5 := CacheKey(streamID, "v2", updatedAt1, window)
	if key1a == key5 {
		t.Fatalf("different version should produce different keys: both = %q", key1a)
	}
}

func TestIntegration_ComputeHeatmap_NoRollups(t *testing.T) {
	cfg := DefaultScoringConfig()
	resp := ComputeHeatmap(nil, cfg)

	if len(resp.Points) != 0 {
		t.Errorf("expected empty points for nil rollups, got %d", len(resp.Points))
	}
	if resp.WindowSeconds != defaultWindowSeconds {
		t.Errorf("WindowSeconds = %d, want %d", resp.WindowSeconds, defaultWindowSeconds)
	}
	if resp.ScoringVersion != cfg.Version {
		t.Errorf("ScoringVersion = %q, want %q", resp.ScoringVersion, cfg.Version)
	}

	resp2 := ComputeHeatmap([]MinuteRollup{}, cfg)
	if len(resp2.Points) != 0 {
		t.Errorf("expected empty points for empty rollups, got %d", len(resp2.Points))
	}
}

func TestIntegration_ComputeHeatmap_DetailResponse_PreloadedRollups(t *testing.T) {
	base := time.Date(2026, 6, 11, 14, 0, 0, 0, time.UTC)

	rollups := []MinuteRollup{
		{MinuteTS: base, ChatCount: 100, TotalEmoteCount: 40, SevenTVEmoteCount: 30, ViewerAvg: 500, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 25, "twitch:ghi:PogChamp": 15}},
		{MinuteTS: base.Add(1 * time.Minute), ChatCount: 400, TotalEmoteCount: 250, SevenTVEmoteCount: 200, ViewerAvg: 1500, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 150, "seventv:def:LUL": 50, "ffz:jkl:catJAM": 50}},
		{MinuteTS: base.Add(2 * time.Minute), ChatCount: 50, TotalEmoteCount: 10, SevenTVEmoteCount: 5, ViewerAvg: 450, ViewerSamples: 3, Emotes: map[string]int{"seventv:abc:KEKW": 5, "seventv:def:LUL": 5}},
	}

	cfg := DefaultScoringConfig()
	resp := ComputeHeatmapDetail(rollups, cfg)

	if resp.ScoringVersion != cfg.Version {
		t.Errorf("ScoringVersion = %q, want %q", resp.ScoringVersion, cfg.Version)
	}
	if len(resp.Points) == 0 {
		t.Fatal("expected at least one scored detail point")
	}

	for i, p := range resp.Points {
		if p.Score <= 0 || p.Score > 100 {
			t.Errorf("detail point[%d]: score %d must be in (0,100]", i, p.Score)
		}
		if p.Components == nil {
			t.Errorf("detail point[%d]: components must not be nil", i)
			continue
		}
		expectedSignals := []string{sigChatRate, sigEmoteRate, sigViewerMomentum, sigProviderSpike, sigTopEmoteDominance, sigNovelty}
		for _, sig := range expectedSignals {
			comp, ok := p.Components[sig]
			if !ok {
				t.Errorf("detail point[%d]: missing component %q", i, sig)
				continue
			}
			if comp.Confidence < 0 || comp.Confidence > 1 {
				t.Errorf("detail point[%d] component %q: confidence %f out of [0,1]", i, sig, comp.Confidence)
			}
		}
	}
}

func TestIntegration_CacheKey_InvalidationPattern(t *testing.T) {
	streamID := "stream-99999"
	version := "v1"
	updatedAtMs := int64(1718000000000)

	key60 := CacheKey(streamID, version, updatedAtMs, 60)
	key120 := CacheKey(streamID, version, updatedAtMs, 120)

	if key60 == key120 {
		t.Fatalf("different windows must produce different keys")
	}

	newUpdatedAtMs := int64(1718000090000)
	newKey60 := CacheKey(streamID, version, newUpdatedAtMs, 60)
	newKey120 := CacheKey(streamID, version, newUpdatedAtMs, 120)

	if key60 == newKey60 {
		t.Errorf("rollup write should invalidate cache: old key60 = new key60 = %q", key60)
	}
	if key120 == newKey120 {
		t.Errorf("rollup write should invalidate cache: old key120 = new key120 = %q", key120)
	}
}
