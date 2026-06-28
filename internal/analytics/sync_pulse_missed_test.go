package analytics

import (
	"testing"
	"time"
)

func TestSummarizeMissedChatFinalizeCountsCachedRollups(t *testing.T) {
	rollupStart := time.Date(2026, 6, 25, 18, 36, 0, 0, time.UTC)
	cache := newChatRollupCache()
	cache.store(rollupStart, []MinuteRollup{
		{MinuteTS: rollupStart.Add(1 * time.Minute), ChatCount: 2},
		{MinuteTS: rollupStart.Add(2 * time.Minute), ChatCount: 1},
	})
	comments := map[int][]string{
		1: {"one", "two"},
		2: {"three"},
	}

	got := summarizeMissedChatFinalize(rollupStart, comments, cache)
	if got.CommentsMatched != 3 {
		t.Fatalf("comments matched = %d, want 3", got.CommentsMatched)
	}
	if got.RollupMinutes != 2 || got.RollupsMatched != 2 || got.PendingMinutes != 0 {
		t.Fatalf("unexpected rollup summary: %+v", got)
	}
	if got.OffsetStart != 60 || got.OffsetEnd != 179 {
		t.Fatalf("unexpected offset range: %+v", got)
	}
	if got.QueryStart != rollupStart.Add(time.Minute) || got.QueryEnd != rollupStart.Add(3*time.Minute) {
		t.Fatalf("unexpected query range: %s - %s", got.QueryStart, got.QueryEnd)
	}
}

func TestSummarizeMissedChatFinalizeKeepsPendingMinutes(t *testing.T) {
	rollupStart := time.Date(2026, 6, 25, 18, 36, 0, 0, time.UTC)
	cache := newChatRollupCache()
	cache.store(rollupStart, []MinuteRollup{{MinuteTS: rollupStart, ChatCount: 1}})
	comments := map[int][]string{
		0: {"cached"},
		4: {"pending"},
	}

	got := summarizeMissedChatFinalize(rollupStart, comments, cache)
	if got.CommentsMatched != 2 || got.RollupMinutes != 2 || got.RollupsMatched != 1 || got.PendingMinutes != 1 {
		t.Fatalf("unexpected rollup summary: %+v", got)
	}
	if got.OffsetStart != 0 || got.OffsetEnd != 299 {
		t.Fatalf("unexpected offset range: %+v", got)
	}
}

func TestSummarizeMissedChatFinalizeEmptyRange(t *testing.T) {
	got := summarizeMissedChatFinalize(time.Now(), map[int][]string{3: nil}, newChatRollupCache())
	if got.CommentsMatched != 0 || got.RollupMinutes != 0 || got.PendingMinutes != 0 {
		t.Fatalf("empty comments should not produce rollup work: %+v", got)
	}
	if got.OffsetStart != -1 || got.OffsetEnd != -1 || !got.QueryStart.IsZero() || !got.QueryEnd.IsZero() {
		t.Fatalf("empty range should not produce query bounds: %+v", got)
	}
}
