package analytics

import (
	"context"
	"testing"
	"time"
)

type stubHelixGameName struct {
	game string
	err  error
}

func (s stubHelixGameName) VideoGameName(context.Context, string) (string, error) {
	return s.game, s.err
}

func TestResolveSyncGameSegmentsPrefersTT(t *testing.T) {
	svc := &SyncService{}
	got := svc.resolveSyncGameSegments(context.Background(), "s1", "vod1", "Live", 3600, []scrapedGame{{Title: "Minecraft"}})
	if len(got) != 1 || got[0].Title != "Minecraft" {
		t.Fatalf("segments = %#v, want TT game", got)
	}
}

func TestResolveSyncGameSegmentsUsesCategoryFallback(t *testing.T) {
	svc := &SyncService{}
	got := svc.resolveSyncGameSegments(context.Background(), "s1", "", "Just Chatting", 7200, nil)
	if len(got) != 1 || got[0].Title != "Just Chatting" {
		t.Fatalf("segments = %#v, want category fallback", got)
	}
}

func TestResolveSyncGameSegmentsFallbackBuildsFullDurationSegment(t *testing.T) {
	svc := &SyncService{}
	games := svc.resolveSyncGameSegments(context.Background(), "s1", "", "Just Chatting", 7200, nil)
	segments := buildGameSegments(games, 7200)
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one full-stream segment", segments)
	}
	if segments[0].GameName != "Just Chatting" || segments[0].OffsetSeconds != 0 || segments[0].DurationSeconds != 7200 {
		t.Fatalf("segment = %+v, want Just Chatting from 0 to 7200", segments[0])
	}
}

func TestFallbackGameSegmentsForEndedStreamCategory(t *testing.T) {
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(90 * time.Minute)
	segments := fallbackGameSegmentsForStream(&StreamRecord{
		StreamID:  "s1",
		Category:  "Just Chatting",
		StartedAt: startedAt,
		EndedAt:   &endedAt,
	})
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one category segment", segments)
	}
	if segments[0].GameName != "Just Chatting" || segments[0].DurationSeconds != 90*60 {
		t.Fatalf("segment = %+v, want category for stream duration", segments[0])
	}
}

func TestFallbackGameSegmentsForEndedStreamSkipsUnknownCategory(t *testing.T) {
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(90 * time.Minute)
	if got := fallbackGameSegmentsForStream(&StreamRecord{StreamID: "s1", Category: "Live", StartedAt: startedAt, EndedAt: &endedAt}); len(got) != 0 {
		t.Fatalf("segments = %#v, want empty for placeholder category", got)
	}
	if got := fallbackGameSegmentsForStream(&StreamRecord{StreamID: "s1", Category: "Just Chatting", StartedAt: startedAt}); len(got) != 0 {
		t.Fatalf("segments = %#v, want empty for live stream", got)
	}
}

func TestResolveGameCategoryFallbackUsesHelix(t *testing.T) {
	got := resolveGameCategoryFallback(context.Background(), "Live", "", "12345", stubHelixGameName{game: "VALORANT"})
	if got != "VALORANT" {
		t.Fatalf("category = %q, want VALORANT", got)
	}
}

func TestResolveSyncGameSegmentsSkipsWhenNoSignal(t *testing.T) {
	svc := &SyncService{}
	got := svc.resolveSyncGameSegments(context.Background(), "s1", "", "Live", 1800, nil)
	if len(got) != 0 {
		t.Fatalf("segments = %#v, want empty", got)
	}
}
