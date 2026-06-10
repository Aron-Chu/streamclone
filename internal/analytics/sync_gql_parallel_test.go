package analytics

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestVodChatAlignSeconds(t *testing.T) {
	streamStart := time.Date(2026, 6, 7, 16, 57, 0, 0, time.UTC)
	vodCreated := time.Date(2026, 6, 7, 21, 36, 0, 0, time.UTC)
	if got := vodChatAlignSeconds(streamStart, vodCreated); got != 16740 {
		t.Fatalf("expected align 16740, got %d", got)
	}
	if got := vodChatAlignSeconds(streamStart, streamStart); got != 0 {
		t.Fatalf("expected zero align for matching starts, got %d", got)
	}
}

func TestBuildGQLSegments(t *testing.T) {
	segs := buildGQLSegments(1800, 600)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	if segs[0].StartSec != 0 || segs[0].EndSec != 600 {
		t.Fatalf("unexpected first segment: %+v", segs[0])
	}
	if segs[2].StartSec != 1200 || segs[2].EndSec != 1800 {
		t.Fatalf("unexpected last segment: %+v", segs[2])
	}
}

func TestBuildGQLSegmentsDefaults(t *testing.T) {
	segs := buildGQLSegments(0, 0)
	if len(segs) != 1 {
		t.Fatalf("expected 1 default segment, got %d", len(segs))
	}
}

func TestEffectiveGQLSegmentSeconds(t *testing.T) {
	if got := effectiveGQLSegmentSeconds(600, 120, 3600, 0); got != 600 {
		t.Fatalf("expected 600 for 1h VOD, got %d", got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, vodGQLLargeVODDurationSec, 0); got != vodGQLSegmentLargeVOD {
		t.Fatalf("expected %d for long VOD, got %d", vodGQLSegmentLargeVOD, got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, 7200, 60_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for 60k comments, got %d", vodGQLSegmentDenseVOD, got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, 7200, 300_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for 300k comments, got %d", vodGQLSegmentDenseVOD, got)
	}
}

func TestEffectiveGQLSegmentSecondsCommentDensityPerHour(t *testing.T) {
	// 2h VOD with 40k comments => 20k/hour => 5-minute segments
	if got := effectiveGQLSegmentSeconds(600, 120, 7200, 40_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for high per-hour density, got %d", vodGQLSegmentDenseVOD, got)
	}
}

func TestBuildGQLSegmentsLongVODUsesFiveMinuteChunks(t *testing.T) {
	segs := buildGQLSegments(5*3600, vodGQLSegmentLargeVOD)
	if len(segs) != 60 {
		t.Fatalf("expected 60 five-minute segments for 5h VOD, got %d", len(segs))
	}
}

func TestGQLRateCoordinatorThrottle(t *testing.T) {
	coord := &gqlRateCoordinator{}
	coord.Throttle(0, 0)
	coord.mu.Lock()
	until := coord.pauseUntil
	coord.mu.Unlock()
	if until.IsZero() {
		t.Fatal("expected pause window after throttle")
	}
}

func TestGQLRateCoordinatorWaitRespectsPause(t *testing.T) {
	coord := &gqlRateCoordinator{}
	coord.Throttle(50*time.Millisecond, 0)
	start := time.Now()
	if err := coord.Wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("expected wait through pause, got %v", elapsed)
	}
}

func TestVODCommentsFetchStateMergeDedupe(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	edge := GQLCommentEdge{
		Node: struct {
			ID                   string `json:"id"`
			ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
			Message              struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			} `json:"message"`
		}{
			ID:                   "abc",
			ContentOffsetSeconds: 90,
			Message: struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			}{Body: "hello"},
		},
	}
	state.mergeEdge(edge, 0, 600)
	state.mergeEdge(edge, 0, 600)
	if count.Load() != 1 {
		t.Fatalf("expected deduped count 1, got %d", count.Load())
	}
	state.shardedComments.mergeInto(state.commentsMap)
	if len(state.commentsMap[1]) != 1 {
		t.Fatalf("expected one comment in minute bucket, got %+v", state.commentsMap[1])
	}
}

func TestVODCommentsFetchStateMergeAppliesChatAlign(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
		chatAlignSec:  16740,
	}
	edge := GQLCommentEdge{
		Node: struct {
			ID                   string `json:"id"`
			ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
			Message              struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			} `json:"message"`
		}{
			ID:                   "abc",
			ContentOffsetSeconds: 0,
			Message: struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			}{Body: "hello"},
		},
	}
	state.mergeEdge(edge, 0, 600)
	state.shardedComments.mergeInto(state.commentsMap)
	if len(state.commentsMap[279]) != 1 {
		t.Fatalf("expected aligned minute bucket 279, got %+v", state.commentsMap)
	}
}

func TestVODCommentsFetchStateMergeSkipsOutOfRange(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:    make(map[int][]string),
		commentsCount:  &count,
		vodDurationSec: 3600,
	}
	makeEdge := func(id string, offset int, body string) GQLCommentEdge {
		return GQLCommentEdge{
			Node: struct {
				ID                   string `json:"id"`
				ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
				Message              struct {
					Body      string `json:"body"`
					Fragments []struct {
						Text string `json:"text"`
					} `json:"fragments"`
				} `json:"message"`
			}{
				ID:                   id,
				ContentOffsetSeconds: offset,
				Message: struct {
					Body      string `json:"body"`
					Fragments []struct {
						Text string `json:"text"`
					} `json:"fragments"`
				}{Body: body},
			},
		}
	}
	before := makeEdge("before", 500, "early")
	inRange := makeEdge("in", 650, "ok")
	after := makeEdge("after", 750, "late")

	if state.mergeEdge(before, 600, 720) {
		t.Fatal("expected before-segment comment not to mark past segment")
	}
	if count.Load() != 0 {
		t.Fatalf("expected out-of-range comment skipped, count=%d", count.Load())
	}
	if state.mergeEdge(inRange, 600, 720) {
		t.Fatal("expected in-range comment accepted")
	}
	if count.Load() != 1 {
		t.Fatalf("expected count 1, got %d", count.Load())
	}
	if !state.mergeEdge(after, 600, 720) {
		t.Fatal("expected past-segment comment to mark segment complete")
	}
	if count.Load() != 1 {
		t.Fatalf("expected past-segment comment not stored, count=%d", count.Load())
	}
}

func TestVODCommentsFetchStateMergeExcludesSegmentEndBoundary(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:    make(map[int][]string),
		commentsCount:  &count,
		vodDurationSec: 3600,
	}
	makeEdge := func(id string, offset int) GQLCommentEdge {
		return GQLCommentEdge{
			Node: struct {
				ID                   string `json:"id"`
				ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
				Message              struct {
					Body      string `json:"body"`
					Fragments []struct {
						Text string `json:"text"`
					} `json:"fragments"`
				} `json:"message"`
			}{
				ID:                   id,
				ContentOffsetSeconds: offset,
				Message: struct {
					Body      string `json:"body"`
					Fragments []struct {
						Text string `json:"text"`
					} `json:"fragments"`
				}{Body: "ok"},
			},
		}
	}
	// Non-final segment [600,720): offset 720 belongs to next segment.
	if !state.mergeEdge(makeEdge("boundary", 720), 600, 720) {
		t.Fatal("expected offset == segmentEnd on non-final segment to mark past segment")
	}
	if count.Load() != 0 {
		t.Fatalf("expected boundary comment excluded, count=%d", count.Load())
	}
	// Final segment includes offset == vod duration.
	finalState := &vodCommentsFetchState{
		commentsMap:    make(map[int][]string),
		commentsCount:  &count,
		vodDurationSec: 3600,
	}
	if finalState.mergeEdge(makeEdge("final", 3600), 3300, 3600) {
		t.Fatal("expected final-segment boundary comment to be accepted")
	}
	if count.Load() != 1 {
		t.Fatalf("expected final boundary comment stored, count=%d", count.Load())
	}
}

func TestGQLRateCoordinatorAdaptiveConcurrency(t *testing.T) {
	coord := newGQLRateCoordinator(2, 6, 4)
	if got := coord.ActiveConcurrency(); got != 4 {
		t.Fatalf("expected initial concurrency 4, got %d", got)
	}
	coord.RecordRateLimit()
	if got := coord.ActiveConcurrency(); got != 3 {
		t.Fatalf("expected concurrency 3 after rate limit, got %d", got)
	}
	for i := 0; i < 50; i++ {
		coord.RecordSuccess()
	}
	if got := coord.ActiveConcurrency(); got != 4 {
		t.Fatalf("expected concurrency 4 after success streak, got %d", got)
	}
}

func TestTopNMapLimitsSyncFlushEmotes(t *testing.T) {
	emotes := make(map[string]int, 60)
	for i := 0; i < 60; i++ {
		emotes[fmt.Sprintf("e%d", i)] = i
	}
	trimmed := topNMap(emotes, syncChatRollupTopEmotes)
	if len(trimmed) != syncChatRollupTopEmotes {
		t.Fatalf("expected %d emotes, got %d", syncChatRollupTopEmotes, len(trimmed))
	}
}

func TestSegmentAlignedMinuteBounds(t *testing.T) {
	seg := gqlSegmentProgress{StartSec: 0, EndSec: 600}
	start, end := segmentAlignedMinuteBounds(seg, 16740)
	if start != 279 || end != 288 {
		t.Fatalf("expected aligned bounds 279-288, got %d-%d", start, end)
	}
	start, end = segmentAlignedMinuteBounds(seg, 0)
	if start != 0 || end != 9 {
		t.Fatalf("expected zero-align bounds 0-9, got %d-%d", start, end)
	}
}

func TestFinishSegmentExtractsAlignedMinutesForIncrementalPatch(t *testing.T) {
	var count atomic.Int64
	commentsMap := make(map[int][]string)
	state := &vodCommentsFetchState{
		commentsMap:   commentsMap,
		commentsCount: &count,
		chatAlignSec:  16740,
	}
	edge := GQLCommentEdge{
		Node: struct {
			ID                   string `json:"id"`
			ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
			Message              struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			} `json:"message"`
		}{
			ID:                   "abc",
			ContentOffsetSeconds: 90,
			Message: struct {
				Body      string `json:"body"`
				Fragments []struct {
					Text string `json:"text"`
				} `json:"fragments"`
			}{Body: "hello"},
		},
	}
	state.mergeEdge(edge, 0, 600)
	seg := gqlSegmentProgress{StartSec: 0, EndSec: 600, OffsetSec: 90}
	state.finishSegment(&seg, 90)
	if len(commentsMap[280]) != 1 {
		t.Fatalf("expected aligned minute 280 in commentsMap, got %+v", commentsMap)
	}
	remaining := make(map[int][]string)
	state.shardedComments.mergeInto(remaining)
	if len(remaining) != 0 {
		t.Fatalf("expected sharded comments drained for segment, got %+v", remaining)
	}
}
