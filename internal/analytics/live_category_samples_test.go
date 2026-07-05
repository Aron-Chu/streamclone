package analytics

import (
	"testing"
	"time"
)

func TestGameCategoryAtOffsetMultiSegment(t *testing.T) {
	segments := []GameSegment{
		{GameName: "Just Chatting", OffsetSeconds: 0, DurationSeconds: 3600},
		{GameName: "VALORANT", OffsetSeconds: 3600, DurationSeconds: 7200},
	}
	if got := gameCategoryAtOffset(segments, 1800); got != "Just Chatting" {
		t.Fatalf("offset 1800 = %q, want Just Chatting", got)
	}
	if got := gameCategoryAtOffset(segments, 4000); got != "VALORANT" {
		t.Fatalf("offset 4000 = %q, want VALORANT", got)
	}
}

func TestDominantCategoryFromSegments(t *testing.T) {
	segments := []GameSegment{
		{GameName: "Just Chatting", DurationSeconds: 600},
		{GameName: "Minecraft", DurationSeconds: 5400},
	}
	if got := dominantCategoryFromSegments(segments, "Live"); got != "Minecraft" {
		t.Fatalf("dominant = %q, want Minecraft", got)
	}
}

func TestGamesSummaryFromSegments(t *testing.T) {
	segments := []GameSegment{
		{GameName: "Just Chatting", DurationSeconds: 600},
		{GameName: "VALORANT", DurationSeconds: 1200},
		{GameName: "VALORANT", DurationSeconds: 900},
	}
	got := gamesSummaryFromSegments(segments, "")
	if got != "Just Chatting · VALORANT" {
		t.Fatalf("summary = %q, want Just Chatting · VALORANT", got)
	}
}

func TestBuildGameSegmentsFromIRCSnapshotSamples(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 18, 0, 0, 0, time.UTC)
	samples := []categoryTimelineSample{
		{At: startedAt.Add(5 * time.Minute), Category: "Just Chatting"},
		{At: startedAt.Add(90 * time.Minute), Category: "Holdfast: Nations At War"},
	}
	got := buildGameSegmentsFromCategoryTimeline("stream-z", startedAt, startedAt.Add(4*time.Hour), samples)
	if len(got) != 2 {
		t.Fatalf("segments = %#v, want 2", got)
	}
	if got[1].GameName != "Holdfast: Nations At War" {
		t.Fatalf("second segment = %+v", got[1])
	}
}
