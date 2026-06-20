package analytics

import (
	"testing"
	"time"
)

func TestGoldRulesAlwaysTracked(t *testing.T) {
	engine := NewGoldRulesEngine([]string{"ohnepixel"}, 0, 0)
	if !engine.Match("OhnePixel", 0, 0) {
		t.Fatal("always-tracked login should match")
	}
	if engine.Match("other", 0, 0) {
		t.Fatal("unlisted login should not match when thresholds disabled")
	}
}

func TestGoldRulesPeakViewers(t *testing.T) {
	engine := NewGoldRulesEngine(nil, 1000, 0)
	if !engine.Match("x", 1000, 0) {
		t.Fatal("peak at threshold should match")
	}
	if !engine.Match("x", 5000, 0) {
		t.Fatal("peak above threshold should match")
	}
	if engine.Match("x", 999, 0) {
		t.Fatal("peak below threshold should not match")
	}
}

func TestGoldRulesDuration(t *testing.T) {
	engine := NewGoldRulesEngine(nil, 0, 120)
	if !engine.Match("x", 0, 120) {
		t.Fatal("duration at threshold should match")
	}
	if engine.Match("x", 0, 119) {
		t.Fatal("duration below threshold should not match")
	}
}

func TestGoldRulesEvalReasons(t *testing.T) {
	engine := NewGoldRulesEngine([]string{"a"}, 500, 60)
	got := engine.Eval("1", "a", 100, 10)
	if !got.Matched || len(got.Reasons) != 1 || got.Reasons[0] != "always_tracked" {
		t.Fatalf("eval = %+v", got)
	}
	got = engine.Eval("2", "b", 600, 90)
	if !got.Matched || len(got.Reasons) != 2 {
		t.Fatalf("eval = %+v", got)
	}
	got = engine.Eval("3", "c", 1, 1)
	if got.Matched {
		t.Fatalf("eval should not match: %+v", got)
	}
}

func TestStreamDurationMinutes(t *testing.T) {
	start := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Minute)
	if got := streamDurationMinutes(start, &end); got != 90 {
		t.Fatalf("duration = %d, want 90", got)
	}
	if got := streamDurationMinutes(start, nil); got != 0 {
		t.Fatalf("nil ended_at = %d, want 0", got)
	}
}
