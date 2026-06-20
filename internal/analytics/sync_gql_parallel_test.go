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
	if got := effectiveGQLSegmentSeconds(600, 120, 900, 3600, 0); got != 600 {
		t.Fatalf("expected 600 for 1h VOD, got %d", got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, 900, vodGQLLargeVODDurationSec, 0); got != 900 {
		t.Fatalf("expected 900 for quiet long VOD, got %d", got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, 900, 7200, 60_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for 60k comments, got %d", vodGQLSegmentDenseVOD, got)
	}
	if got := effectiveGQLSegmentSeconds(600, 120, 900, 7200, 300_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for 300k comments, got %d", vodGQLSegmentDenseVOD, got)
	}
}

func TestEffectiveGQLSegmentSecondsCommentDensityPerHour(t *testing.T) {
	// 2h VOD with 40k comments => 20k/hour => 2-minute segments
	if got := effectiveGQLSegmentSeconds(600, 120, 900, 7200, 40_000); got != vodGQLSegmentDenseVOD {
		t.Fatalf("expected %d for high per-hour density, got %d", vodGQLSegmentDenseVOD, got)
	}
	// Quiet 8h VOD with low comment volume => 15-minute segments
	if got := effectiveGQLSegmentSeconds(600, 120, 900, 8*3600, 500); got != 900 {
		t.Fatalf("expected 900 for quiet long VOD, got %d", got)
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
	coord.Throttle(429, 0, 0)
	coord.mu.Lock()
	until := coord.pauseUntil
	coord.mu.Unlock()
	if until.IsZero() {
		t.Fatal("expected pause window after throttle")
	}
}

func TestGQLRateCoordinatorWaitRespectsPause(t *testing.T) {
	coord := &gqlRateCoordinator{}
	coord.Throttle(429, 50*time.Millisecond, 0)
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
	edge := GQLCommentEdge{}
	edge.Node.ID = "abc"
	edge.Node.ContentOffsetSeconds = 90
	edge.Node.Message.Body = "hello"
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
	edge := GQLCommentEdge{}
	edge.Node.ID = "abc"
	edge.Node.ContentOffsetSeconds = 0
	edge.Node.Message.Body = "hello"
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
		edge := GQLCommentEdge{}
		edge.Node.ID = id
		edge.Node.ContentOffsetSeconds = offset
		edge.Node.Message.Body = body
		return edge
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
		edge := GQLCommentEdge{}
		edge.Node.ID = id
		edge.Node.ContentOffsetSeconds = offset
		edge.Node.Message.Body = "ok"
		return edge
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

func TestHotSegmentSplitReasonAdaptiveTriggers(t *testing.T) {
	recent := []gqlPageSample{
		{offsetAdvance: 5, commentCount: 20},
		{offsetAdvance: 4, commentCount: 18},
		{offsetAdvance: 3, commentCount: 22},
		{offsetAdvance: 2, commentCount: 19},
		{offsetAdvance: 1, commentCount: 21},
	}
	if got := hotSegmentSplitReason(10, 10, recent, 30, 5, 80); got != "page_threshold" {
		t.Fatalf("expected page_threshold split, got %q", got)
	}
	if got := hotSegmentSplitReason(3, 10, recent, 30, 5, 80); got != "slow_advance" {
		t.Fatalf("expected slow_advance split, got %q", got)
	}
	dense := []gqlPageSample{
		{offsetAdvance: 120, commentCount: 90},
		{offsetAdvance: 110, commentCount: 95},
		{offsetAdvance: 100, commentCount: 85},
		{offsetAdvance: 90, commentCount: 88},
		{offsetAdvance: 80, commentCount: 92},
	}
	if got := hotSegmentSplitReason(3, 10, dense, 30, 5, 80); got != "comments_per_page" {
		t.Fatalf("expected comments_per_page split, got %q", got)
	}
	if got := hotSegmentSplitReason(3, 10, dense, 30, 5, 200); got != "" {
		t.Fatalf("did not expect split when density threshold disabled, got %q", got)
	}

	state := &vodCommentsFetchState{}
	state.recordHotSplit("page_threshold")
	state.recordHotSplit("slow_advance")
	if got := state.latestHotSplitReason(); got != "slow_advance" {
		t.Fatalf("expected latest split reason slow_advance, got %q", got)
	}
}

func TestImpliedAvgSegmentSec(t *testing.T) {
	if got := impliedAvgSegmentSecFromSegments(3600, 300, 12); got != 300 {
		t.Fatalf("expected vod/segments=300, got %v", got)
	}
	if got := impliedAvgSegmentSecFromSegments(0, 300, 12); got != 300 {
		t.Fatalf("expected effectiveSegmentSec fallback, got %v", got)
	}
	if got := impliedAvgSegmentSecFromSegments(3600, 300, 0); got != 300 {
		t.Fatalf("expected effectiveSegmentSec when no segments, got %v", got)
	}
}

func TestTryAutoCloseSegment(t *testing.T) {
	state := &vodCommentsFetchState{vodDurationSec: 3600}
	seg := &gqlSegmentProgress{StartSec: 600, EndSec: 720, OffsetSec: 720}
	if !tryAutoCloseSegment(state, seg) {
		t.Fatal("expected non-final segment at end boundary to auto-close")
	}
	if !seg.Done {
		t.Fatal("expected segment to be marked done")
	}
	if got := seg.OffsetSec; got != 720 {
		t.Fatalf("expected offset capped at segment end 720, got %d", got)
	}
	if got := state.autoClosedTotal.Load(); got != 1 {
		t.Fatalf("expected auto-closed counter 1, got %d", got)
	}

	finalState := &vodCommentsFetchState{vodDurationSec: 3600}
	finalSeg := &gqlSegmentProgress{StartSec: 3300, EndSec: 3600, OffsetSec: 3600}
	if tryAutoCloseSegment(finalState, finalSeg) {
		t.Fatal("did not expect final segment at vod boundary to auto-close")
	}

	tailState := &vodCommentsFetchState{vodDurationSec: 3600}
	tailSeg := &gqlSegmentProgress{StartSec: 3300, EndSec: 3600, OffsetSec: 3601}
	if !tryAutoCloseSegment(tailState, tailSeg) {
		t.Fatal("expected final segment past vod boundary to auto-close")
	}
	if got := tailSeg.OffsetSec; got != 3600 {
		t.Fatalf("expected final segment offset capped at vod end 3600, got %d", got)
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

func TestGQLSegmentPointerSnapshotsSurviveAppend(t *testing.T) {
	segments := gqlSegmentPointers(buildGQLSegments(1200, 600))
	state := &vodCommentsFetchState{segments: &segments}
	first, ok := state.segmentAt(0)
	if !ok {
		t.Fatal("expected first segment")
	}

	for i := 0; i < 32; i++ {
		state.appendSegment(gqlSegmentProgress{
			StartSec:  1200 + i,
			EndSec:    1201 + i,
			OffsetSec: 1200 + i,
		})
	}
	first.Done = true
	first.OffsetSec = 599

	snapshot := state.snapshotSegments()
	if len(snapshot) != 34 {
		t.Fatalf("expected 34 segments after appends, got %d", len(snapshot))
	}
	if !snapshot[0].Done || snapshot[0].OffsetSec != 599 {
		t.Fatalf("expected first segment mutation to survive append, got %+v", snapshot[0])
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
