package analytics

import "testing"

func TestGoldVODSegmentKeyIsDeterministic(t *testing.T) {
	got := GoldVODSegmentKey(" vod-1 ", 0, 600, "")
	want := "vod-1:0:600:gold-gql-v1"
	if got != want {
		t.Fatalf("GoldVODSegmentKey = %q, want %q", got, want)
	}
	if got == GoldVODSegmentKey("vod-1", 600, 1200, "") {
		t.Fatal("segment key did not change when offset window changed")
	}
	if got == GoldVODSegmentKey("vod-1", 0, 600, "gold-gql-v2") {
		t.Fatal("segment key did not change when strategy version changed")
	}
}

func TestPlanGoldVODSegmentsCoversTrailingWindow(t *testing.T) {
	segments := PlanGoldVODSegments("vod-1", "stream-1", "XQC", 1250, 600, "")
	if len(segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(segments))
	}
	windows := [][2]int{{0, 600}, {600, 1200}, {1200, 1250}}
	for i, window := range windows {
		segment := segments[i]
		if segment.StartOffsetSeconds != window[0] || segment.EndOffsetSeconds != window[1] {
			t.Fatalf("segment %d window = %d-%d, want %d-%d", i, segment.StartOffsetSeconds, segment.EndOffsetSeconds, window[0], window[1])
		}
		if segment.VODID != "vod-1" || segment.StreamID != "stream-1" || segment.Login != "xqc" || segment.StrategyVersion != GoldVODSegmentStrategyV1 {
			t.Fatalf("segment %d metadata = %+v", i, segment)
		}
	}
}

func TestPlanGoldVODSegmentsRejectsInvalidInput(t *testing.T) {
	if got := PlanGoldVODSegments("", "stream-1", "xqc", 1200, 600, ""); len(got) != 0 {
		t.Fatalf("empty vod produced %d segments", len(got))
	}
	if got := PlanGoldVODSegments("vod-1", "stream-1", "xqc", 0, 600, ""); len(got) != 0 {
		t.Fatalf("zero duration produced %d segments", len(got))
	}
	segments := PlanGoldVODSegments("vod-1", "stream-1", "xqc", 601, 0, "")
	if len(segments) != 2 || segments[0].EndOffsetSeconds != 600 || segments[1].EndOffsetSeconds != 601 {
		t.Fatalf("default segment plan = %+v", segments)
	}
}
