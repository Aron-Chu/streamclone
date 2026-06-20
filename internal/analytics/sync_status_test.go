package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSyncChatProgressJSONRoundTrip(t *testing.T) {
	progress := SyncChatProgress{
		Active:              true,
		VodID:               "123",
		FetchMode:           "parallel",
		Concurrency:         4,
		EffectiveSegmentSec: 300,
		InitialSegments:     96,
		SegmentsTotal:       142,
		SegmentsDone:        139,
		SegmentsIncomplete:  3,
		HotSplits:           46,
		CleanupPhase:        "parallel_cleanup",
		CommentsFetched:     12000,
		CommentsSaved:       800,
		TimelineSec:         28740,
		VodDurationSec:      28800,
		GQLPages:            450,
		Throttled:           false,
		Message:             "cleanup",
	}
	raw, err := json.Marshal(progress)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SyncChatProgress
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.InitialSegments != 96 || decoded.HotSplits != 46 || decoded.SegmentsIncomplete != 3 || decoded.CleanupPhase != "parallel_cleanup" || decoded.CommentsSaved != 800 {
		t.Fatalf("unexpected decoded progress: %+v", decoded)
	}
}

func TestHasLiveCollectorViewerCoverage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Minute)
	rollups := make([]MinuteRollup, 20)
	for i := range rollups {
		val := 100 + i*10
		if i >= 15 {
			val = 250
		}
		rollups[i] = MinuteRollup{
			MinuteTS:      now.Add(time.Duration(i) * time.Minute),
			ViewerAvg:     val,
			ViewerSamples: 1,
		}
	}
	stream := &StreamRecord{
		ViewerSamples: 20,
		StartedAt:     now,
		EndedAt:       ptrTime(now.Add(20 * time.Minute)),
		LastSeenAt:    now.Add(10 * time.Minute),
	}
	if !hasLiveCollectorViewerCoverage(stream, rollups) {
		t.Fatal("expected live collector coverage")
	}

	stale := *stream
	stale.LastSeenAt = now.Add(-72 * time.Hour)
	if hasLiveCollectorViewerCoverage(&stale, rollups) {
		t.Fatal("expected stale last_seen to fail live collector coverage")
	}
}

func TestInferViewerSource(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{{
		MinuteTS:      now,
		ViewerAvg:     500,
		ViewerSamples: 1,
	}}
	stream := &StreamRecord{ViewerSamples: 5, LastSeenAt: now, StartedAt: now, EndedAt: ptrTime(now.Add(time.Hour))}
	if got := inferViewerSource(stream, rollups); got != "partial" {
		t.Fatalf("expected partial viewer source, got %q", got)
	}
}
