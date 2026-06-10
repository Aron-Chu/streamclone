package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestHasGoodChatCoverageFromRollups(t *testing.T) {
	start := time.Date(2026, 6, 7, 16, 0, 0, 0, time.UTC)
	rollups := make([]MinuteRollup, 120)
	for i := range rollups {
		rollups[i] = MinuteRollup{
			MinuteTS:  start.Add(time.Duration(i) * time.Minute),
			ChatCount: 10,
		}
	}
	stream := &StreamRecord{ChatMessages: 1200}
	if !hasGoodChatCoverageFromRollups(rollups, stream) {
		t.Fatal("expected good coverage for full-span chat rollups")
	}

	compressed := make([]MinuteRollup, 120)
	for i := range compressed {
		chat := 0
		if i >= 90 {
			chat = 20
		}
		compressed[i] = MinuteRollup{MinuteTS: start.Add(time.Duration(i) * time.Minute), ChatCount: chat}
	}
	if hasGoodChatCoverageFromRollups(compressed, &StreamRecord{ChatMessages: 600}) {
		t.Fatal("expected false for chat compressed in tail only")
	}
}

func TestChatCoverageSummaryPartialWhenVODShort(t *testing.T) {
	start := time.Date(2026, 6, 10, 19, 10, 0, 0, time.UTC)
	end := start.Add(29*time.Hour + 30*time.Minute)
	rollups := make([]MinuteRollup, 1770)
	for i := range rollups {
		chat := 0
		if i < 52 {
			chat = 100
		}
		rollups[i] = MinuteRollup{
			MinuteTS:  start.Add(time.Duration(i) * time.Minute),
			ChatCount: chat,
		}
	}
	stream := &StreamRecord{
		StartedAt: start,
		EndedAt:   &end,
	}
	summary := chatCoverageSummary(rollups, stream, 51*60+31)
	if !summary.Partial {
		t.Fatalf("expected partial coverage, got %+v", summary)
	}
	if summary.ChatSpanMinutes != 52 {
		t.Fatalf("chat span minutes = %d, want 52", summary.ChatSpanMinutes)
	}
	if summary.CoveragePct >= 35 {
		t.Fatalf("coverage pct = %f, want < 35", summary.CoveragePct)
	}
}

func TestFormatPartialChatCoverageMessage(t *testing.T) {
	msg := formatPartialChatCoverageMessage("2792534709", ChatCoverageSummary{
		ChatSpanMinutes:   52,
		StreamSpanMinutes: 1770,
		Partial:           true,
	})
	if !strings.Contains(msg, "2792534709") {
		t.Fatalf("expected vod id in message: %q", msg)
	}
	if !strings.Contains(msg, "re-sync later") {
		t.Fatalf("expected resync hint in message: %q", msg)
	}
}
