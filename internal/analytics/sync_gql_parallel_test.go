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

func TestBuildGQLMomentWindowsFromViewerPoints(t *testing.T) {
	points := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 1000},
		{OffsetSeconds: 60, Viewers: 1100},
		{OffsetSeconds: 120, Viewers: 1050},
		{OffsetSeconds: 180, Viewers: 5000},
		{OffsetSeconds: 240, Viewers: 1020},
		{OffsetSeconds: 300, Viewers: 4800},
	}
	windows := buildGQLMomentWindowsFromViewerPoints(points)
	if len(windows) == 0 {
		t.Fatal("expected moment windows around viewer spikes")
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 120, EndSec: 240}, gqlFetchScheduleHints{
		MomentWindows: windows,
	}); got != gqlSegPriorityMoment {
		t.Fatalf("expected moment priority inside spike window, got %d", got)
	}
}

func TestSegmentSchedulePriority(t *testing.T) {
	hints := gqlFetchScheduleHints{
		VodDurationSec:  3600,
		EdgePrioritySec: 600,
		MomentWindows:   []gqlTimeRange{{StartSec: 1000, EndSec: 1100}},
		GameRanges:      []gqlTimeRange{{StartSec: 2000, EndSec: 2600}},
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 1000, EndSec: 1200}, hints); got != gqlSegPriorityMoment {
		t.Fatalf("expected moment priority, got %d", got)
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 2100, EndSec: 2200}, hints); got != gqlSegPriorityGame {
		t.Fatalf("expected game priority, got %d", got)
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 0, EndSec: 600}, hints); got != gqlSegPriorityEdge {
		t.Fatalf("expected edge priority for stream start, got %d", got)
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 3100, EndSec: 3600}, hints); got != gqlSegPriorityEdge {
		t.Fatalf("expected edge priority for stream end, got %d", got)
	}
	if got := segmentSchedulePriority(gqlSegmentProgress{StartSec: 1300, EndSec: 1900}, hints); got != gqlSegPriorityBackground {
		t.Fatalf("expected background priority, got %d", got)
	}
}

func TestShouldSplitHotSegmentAdaptiveTriggers(t *testing.T) {
	recent := []gqlPageSample{
		{offsetAdvance: 5, commentCount: 20},
		{offsetAdvance: 4, commentCount: 18},
		{offsetAdvance: 3, commentCount: 22},
		{offsetAdvance: 2, commentCount: 19},
		{offsetAdvance: 1, commentCount: 21},
	}
	if !shouldSplitHotSegment(10, 10, recent, 30, 5, 80) {
		t.Fatal("expected page threshold split")
	}
	if !shouldSplitHotSegment(3, 10, recent, 30, 5, 80) {
		t.Fatal("expected slow advance split")
	}
	dense := []gqlPageSample{
		{offsetAdvance: 120, commentCount: 90},
		{offsetAdvance: 110, commentCount: 95},
		{offsetAdvance: 100, commentCount: 85},
		{offsetAdvance: 90, commentCount: 88},
		{offsetAdvance: 80, commentCount: 92},
	}
	if !shouldSplitHotSegment(3, 10, dense, 30, 5, 80) {
		t.Fatal("expected comments/page split")
	}
	if shouldSplitHotSegment(3, 10, dense, 30, 5, 200) {
		t.Fatal("did not expect split when density threshold disabled")
	}
}

func TestGQLSegmentWorkQueuePriorityOrder(t *testing.T) {
	q := newGQLSegmentWorkQueue()
	q.push(3, gqlSegPriorityBackground)
	q.push(1, gqlSegPriorityEdge)
	q.push(2, gqlSegPriorityGame)
	q.push(0, gqlSegPriorityMoment)

	ctx := context.Background()
	for _, want := range []int{0, 2, 1, 3} {
		idx, ok := q.acquire(ctx)
		if !ok {
			t.Fatalf("expected work item for idx %d", want)
		}
		if idx != want {
			t.Fatalf("expected idx %d, got %d", want, idx)
		}
		q.release()
	}
	if _, ok := q.acquire(ctx); ok {
		t.Fatal("expected empty queue")
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
