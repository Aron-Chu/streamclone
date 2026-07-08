package ingestcore

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLoginHashPrefix(t *testing.T) {
	if got := normalizeLogin("#XQC"); got != "xqc" {
		t.Fatalf("normalizeLogin(#XQC) = %q, want xqc", got)
	}
	if got := normalizeLogin("  Ludwig  "); got != "ludwig" {
		t.Fatalf("normalizeLogin(Ludwig) = %q, want ludwig", got)
	}
}

func TestCompareKeyNormalization(t *testing.T) {
	minute := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	c := NewShadowComparer(Config{ShadowTolerancePct: 2})
	snap := RollupSnapshot{
		StreamID:  "123",
		Minute:    minute,
		ChatCount: 10,
		Closed:    true,
	}
	keyHash := c.key("#xqc", snap)
	keyPlain := c.key("xqc", snap)
	if keyHash != keyPlain {
		t.Fatalf("key mismatch: %q vs %q", keyHash, keyPlain)
	}
}

func TestWithinTolerance_closedMatch(t *testing.T) {
	rec := ShadowRecord{
		LegacyChat:    100,
		ShadowChat:    101,
		LegacyViewers: 5,
		ShadowViewers: 5,
	}
	ok, reason := withinTolerance(rec, 2)
	if !ok {
		t.Fatalf("expected match within 2%%, got reason=%q", reason)
	}
}

func TestWithinTolerance_legacyZeroShadowNonzero(t *testing.T) {
	rec := ShadowRecord{
		LegacyChat: 0,
		ShadowChat: 50,
	}
	ok, reason := withinTolerance(rec, 2)
	if ok || reason != "legacy_zero_shadow_nonzero" {
		t.Fatalf("expected legacy_zero_shadow_nonzero, ok=%v reason=%q", ok, reason)
	}
}

func TestWithinTolerance_openMinuteSkew(t *testing.T) {
	rec := ShadowRecord{
		LegacyChat: 50,
		ShadowChat: 0,
	}
	ok, reason := withinTolerance(rec, 2)
	if ok || reason != "chat_diff_pct=100.00" {
		t.Fatalf("expected chat_diff_pct=100.00 for legacy>0 shadow=0, ok=%v reason=%q", ok, reason)
	}
}

func TestShadowComparer_closedOnlyPairing(t *testing.T) {
	cfg := Config{ShadowTolerancePct: 2, ShadowArtifactDir: t.TempDir()}
	c := NewShadowComparer(cfg)
	minute := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	openSnap := RollupSnapshot{StreamID: "1", Minute: minute, ChatCount: 10, Closed: false}
	closedSnap := RollupSnapshot{StreamID: "1", Minute: minute, ChatCount: 10, Closed: true}
	c.RecordLegacy("xqc", openSnap)
	c.RecordShadow("xqc", closedSnap)
	if len(c.legacy) != 1 || len(c.shadow) != 1 {
		t.Fatalf("expected one legacy and one shadow entry, legacy=%d shadow=%d", len(c.legacy), len(c.shadow))
	}
	for k := range c.legacy {
		if _, ok := c.shadow[k]; ok {
			t.Fatal("open and closed snapshots must not share compare key")
		}
	}
}

func TestShadowComparer_closedMinuteMatch(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{ShadowTolerancePct: 2, ShadowArtifactDir: dir}
	c := NewShadowComparer(cfg)
	minute := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	snap := RollupSnapshot{
		StreamID:        "123",
		Minute:          minute,
		ChatCount:       23,
		TotalEmoteCount: 40,
		Closed:          true,
	}
	c.RecordLegacy("xqc", snap)
	c.RecordShadow("xqc", snap)
	data, err := os.ReadFile(dir + "/latest.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"match":true`) {
		t.Fatalf("expected closed aligned match=true, got %s", string(data))
	}
}

func TestShadowComparer_openMinuteReasonPrefix(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{ShadowTolerancePct: 2, ShadowArtifactDir: dir}
	c := NewShadowComparer(cfg)
	minute := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	leg := RollupSnapshot{StreamID: "1", Minute: minute, ChatCount: 50, Closed: false}
	sh := RollupSnapshot{StreamID: "1", Minute: minute, ChatCount: 0, Closed: false}
	c.RecordLegacy("xqc", leg)
	c.RecordShadow("xqc", sh)
	data, err := os.ReadFile(dir + "/latest.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "open_minute_excluded:") {
		t.Fatalf("expected open_minute_excluded reason prefix, got %s", string(data))
	}
}
