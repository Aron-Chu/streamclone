package analytics

import (
	"testing"
	"time"
)

func TestConsolidateRollupsByMinutePrefersNonZeroViewers(t *testing.T) {
	start := time.Date(2026, 6, 7, 13, 25, 47, 0, time.UTC)
	in := []MinuteRollup{
		{MinuteTS: start.Add(time.Minute), ViewerAvg: 3887, ViewerMax: 3887, ViewerLatest: 3887, ViewerSamples: 1},
		{MinuteTS: start.Add(time.Minute), ViewerAvg: 0, ViewerMax: 0, ViewerLatest: 0, ViewerSamples: 1},
	}
	out := consolidateRollupsByMinute(in)
	key := minuteBucketKey(start.Add(time.Minute).Truncate(time.Minute))
	got, ok := out[key]
	if !ok {
		t.Fatalf("expected consolidated rollup for %s", key)
	}
	if got.ViewerAvg != 3887 {
		t.Fatalf("expected viewer avg 3887, got %d", got.ViewerAvg)
	}
}

func TestConsolidateRollupsByMinuteMergesChatFromOffsetRows(t *testing.T) {
	start := time.Date(2026, 6, 7, 16, 44, 0, 0, time.UTC)
	in := []MinuteRollup{
		{
			MinuteTS:          start,
			ViewerAvg:         33778,
			ViewerMax:         33778,
			ViewerLatest:      33778,
			ViewerSamples:     1,
			ChatCount:         425,
			SevenTVEmoteCount: 209,
			Emotes:            map[string]int{"seventv:1:KEKW": 209},
		},
		{
			MinuteTS:      start.Add(47 * time.Second),
			ViewerAvg:     36351,
			ViewerMax:     36351,
			ViewerLatest:  36351,
			ViewerSamples: 1,
			Emotes:        map[string]int{},
		},
	}
	out := consolidateRollupsByMinute(in)
	key := minuteBucketKey(start)
	got, ok := out[key]
	if !ok {
		t.Fatalf("expected consolidated rollup for %s", key)
	}
	if got.ViewerAvg != 36351 {
		t.Fatalf("expected viewer avg 36351, got %d", got.ViewerAvg)
	}
	if got.ChatCount != 425 {
		t.Fatalf("expected chat count 425, got %d", got.ChatCount)
	}
	if got.SevenTVEmoteCount != 209 {
		t.Fatalf("expected 7tv count 209, got %d", got.SevenTVEmoteCount)
	}
}

func TestFillMissingRollupsInvertedEndDoesNotPanic(t *testing.T) {
	startedAt := time.Date(2026, 5, 26, 0, 9, 56, 0, time.UTC)
	endedAt := time.Date(2026, 5, 26, 0, 7, 8, 0, time.UTC)
	in := []MinuteRollup{
		{MinuteTS: time.Date(2026, 5, 26, 0, 9, 0, 0, time.UTC), Emotes: map[string]int{}},
		{MinuteTS: time.Date(2026, 5, 26, 0, 7, 0, 0, time.UTC), Emotes: map[string]int{}},
	}
	out := fillMissingRollups(in, startedAt, &endedAt)
	if len(out) == 0 {
		t.Fatalf("expected rollups after inverted end clamp, got 0")
	}
}

func TestFillMissingRollupsUsesConsolidatedViewers(t *testing.T) {
	startedAt := time.Date(2026, 6, 7, 13, 25, 47, 0, time.UTC)
	endedAt := startedAt.Add(2 * time.Minute)
	in := []MinuteRollup{
		{
			MinuteTS: startedAt.Add(time.Minute),
			ViewerAvg: 3887,
			ViewerMax: 3887,
			ViewerLatest: 3887,
			ViewerSamples: 1,
			Emotes: map[string]int{},
		},
	}
	out := fillMissingRollups(in, startedAt, &endedAt)
	if len(out) != 3 {
		t.Fatalf("expected 3 rollups, got %d", len(out))
	}
	if out[1].ViewerAvg != 3887 {
		t.Fatalf("expected minute 1 viewer avg 3887, got %d", out[1].ViewerAvg)
	}
}

func TestSlimRollupsForChartOmitsEmotes(t *testing.T) {
	start := time.Date(2026, 6, 7, 13, 25, 0, 0, time.UTC)
	in := []MinuteRollup{{
		MinuteTS:          start,
		ViewerAvg:         100,
		ChatCount:         5,
		Emotes:            map[string]int{"seventv:1:KEKW": 2},
	}}
	out := slimRollupsForChart(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 rollup, got %d", len(out))
	}
	if out[0].Emotes != nil {
		t.Fatalf("expected emotes omitted from sparse response")
	}
	if out[0].ChatCount != 5 {
		t.Fatalf("expected chat count preserved, got %d", out[0].ChatCount)
	}
}
