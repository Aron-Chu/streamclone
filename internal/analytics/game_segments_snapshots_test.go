package analytics

import (
	"testing"
	"time"
)

func TestBuildGameSegmentsFromCategoryTimelineMultiGame(t *testing.T) {
	startedAt := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	endAt := startedAt.Add(4 * time.Hour)
	samples := []categoryTimelineSample{
		{At: startedAt.Add(10 * time.Minute), Category: "Just Chatting"},
		{At: startedAt.Add(90 * time.Minute), Category: "VALORANT"},
		{At: startedAt.Add(150 * time.Minute), Category: "VALORANT"},
		{At: startedAt.Add(200 * time.Minute), Category: "Minecraft"},
	}

	got := buildGameSegmentsFromCategoryTimeline("stream-xqc", startedAt, endAt, samples)
	if len(got) != 3 {
		t.Fatalf("segments = %#v, want 3 games", got)
	}
	if got[0].GameName != "Just Chatting" || got[0].OffsetSeconds != 0 {
		t.Fatalf("first segment = %+v, want Just Chatting from offset 0", got[0])
	}
	if got[1].GameName != "VALORANT" {
		t.Fatalf("second segment = %+v, want VALORANT", got[1])
	}
	if got[2].GameName != "Minecraft" {
		t.Fatalf("third segment = %+v, want Minecraft", got[2])
	}
	if got[0].OffsetSeconds+got[0].DurationSeconds != got[1].OffsetSeconds {
		t.Fatalf("segments not contiguous: %+v then %+v", got[0], got[1])
	}
}

func TestBuildGameSegmentsFromCategoryTimelineSkipsPlaceholderCategories(t *testing.T) {
	startedAt := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	samples := []categoryTimelineSample{
		{At: startedAt.Add(5 * time.Minute), Category: "Live"},
		{At: startedAt.Add(10 * time.Minute), Category: "Fortnite"},
	}
	got := buildGameSegmentsFromCategoryTimeline("s1", startedAt, startedAt.Add(time.Hour), samples)
	if len(got) != 1 || got[0].GameName != "Fortnite" {
		t.Fatalf("segments = %#v, want single Fortnite segment", got)
	}
}

func TestPreferGameSegmentsUsesSnapshotWhenRicher(t *testing.T) {
	stored := []GameSegment{{GameName: "Just Chatting", OffsetSeconds: 0, DurationSeconds: 3600}}
	snapshot := []GameSegment{
		{GameName: "Just Chatting", OffsetSeconds: 0, DurationSeconds: 1800},
		{GameName: "VALORANT", OffsetSeconds: 1800, DurationSeconds: 1800},
	}
	got := preferGameSegments(stored, snapshot)
	if len(got) != 2 {
		t.Fatalf("segments = %#v, want snapshot timeline", got)
	}
}

func TestPreferGameSegmentsUsesSnapshotWhenMoreDistinctCategories(t *testing.T) {
	stored := []GameSegment{{GameName: "Holdfast: Nations At War", OffsetSeconds: 0, DurationSeconds: 3900}}
	snapshot := []GameSegment{
		{GameName: "Just Chatting", OffsetSeconds: 0, DurationSeconds: 1200},
		{GameName: "VALORANT", OffsetSeconds: 1200, DurationSeconds: 900},
		{GameName: "Holdfast: Nations At War", OffsetSeconds: 2100, DurationSeconds: 1800},
	}
	got := preferGameSegments(stored, snapshot)
	if len(got) != 3 {
		t.Fatalf("segments = %#v, want richer snapshot timeline", got)
	}
}

func TestMergeCategoryTimelineSamplesDedupesAndSorts(t *testing.T) {
	startedAt := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	primary := []categoryTimelineSample{
		{At: startedAt.Add(30 * time.Minute), Category: "Game B"},
	}
	extra := []categoryTimelineSample{
		{At: startedAt.Add(10 * time.Minute), Category: "Game A"},
		{At: startedAt.Add(30 * time.Minute), Category: "Game B"},
	}
	got := mergeCategoryTimelineSamples(primary, extra)
	if len(got) != 2 {
		t.Fatalf("samples = %#v, want 2 merged samples", got)
	}
	if got[0].Category != "Game A" || got[1].Category != "Game B" {
		t.Fatalf("samples = %#v, want sorted A then B", got)
	}
}

func TestDistinctCategoryCountIgnoresEmptyNames(t *testing.T) {
	samples := []categoryTimelineSample{
		{At: time.Now(), Category: "Fortnite"},
		{At: time.Now(), Category: "Fortnite"},
		{At: time.Now(), Category: "Live"},
	}
	if distinctCategoryCount(samples) != 1 {
		t.Fatalf("distinct = %d, want 1 normalized category", distinctCategoryCount(samples))
	}
}

func TestMeaningfulGameSegmentsAndFilterStored(t *testing.T) {
	if meaningfulGameSegments([]GameSegment{{GameName: "Live", OffsetSeconds: 0, DurationSeconds: 60}}) {
		t.Fatal("placeholder category should not count as meaningful")
	}
	got := filterMeaningfulStoredSegments([]GameSegment{
		{GameName: "Live", OffsetSeconds: 0, DurationSeconds: 600},
		{GameName: "Fortnite", OffsetSeconds: 600, DurationSeconds: 600},
	})
	if len(got) != 1 || got[0].GameName != "Fortnite" {
		t.Fatalf("segments = %#v, want only Fortnite", got)
	}
}

func TestResolveStreamGameSegmentsFinalizePath(t *testing.T) {
	startedAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "319387444960",
		Login:     "jynxzi",
		Category:  "Fortnite",
		StartedAt: startedAt,
	}
	var segments []GameSegment
	if !meaningfulGameSegments(segments) {
		segments = fallbackGameSegmentsForStream(stream)
	}
	if len(segments) != 1 || segments[0].GameName != "Fortnite" {
		t.Fatalf("segments = %#v, want Fortnite fallback", segments)
	}
}

func TestShouldTrySnapshotGameSegmentsLiveSingleStored(t *testing.T) {
	stream := &StreamRecord{StreamID: "s1", EndedAt: nil}
	if !shouldTrySnapshotGameSegments([]GameSegment{{GameName: "A"}}, stream) {
		t.Fatal("expected snapshot refresh for live stream with one stored segment")
	}
	ended := time.Now()
	stream.EndedAt = &ended
	if shouldTrySnapshotGameSegments([]GameSegment{{GameName: "A"}, {GameName: "B"}}, stream) {
		t.Fatal("did not expect snapshot refresh when TT segments exist on ended stream")
	}
}
