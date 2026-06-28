package analytics

import (
	"sync/atomic"
	"testing"
)

func TestGQLCommentDeduperSameIDSameMinute(t *testing.T) {
	var d gqlCommentDeduper
	if d.markSeen("msg-1") {
		t.Fatal("first mark should not be already seen")
	}
	if !d.markSeen("msg-1") {
		t.Fatal("second mark should be already seen")
	}
}

func TestGQLCommentDeduperAcrossPages(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	edge := GQLCommentEdge{}
	edge.Node.ID = "page-dup"
	edge.Node.ContentOffsetSeconds = 30
	edge.Node.Message.Body = "hello"
	state.mergeEdge(edge, 0, 600)
	// Simulate same ID on next page fetch.
	state.mergeEdge(edge, 0, 600)
	if count.Load() != 1 {
		t.Fatalf("cross-page duplicate ID count = %d, want 1", count.Load())
	}
}

func TestGQLCommentDeduperAdjacentSegmentBoundary(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	edge := GQLCommentEdge{}
	edge.Node.ID = "boundary-1"
	edge.Node.ContentOffsetSeconds = 599
	edge.Node.Message.Body = "a"
	state.mergeEdge(edge, 0, 600)
	state.mergeEdge(edge, 600, 1200)
	if count.Load() != 1 {
		t.Fatalf("segment boundary duplicate count = %d, want 1", count.Load())
	}
}

func TestGQLCommentDeduperIdenticalContentDifferentID(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	for i, id := range []string{"id-a", "id-b"} {
		edge := GQLCommentEdge{}
		edge.Node.ID = id
		edge.Node.ContentOffsetSeconds = 10 + i
		edge.Node.Message.Body = "same text"
		state.mergeEdge(edge, 0, 600)
	}
	if count.Load() != 2 {
		t.Fatalf("different IDs same text count = %d, want 2", count.Load())
	}
}

func TestGQLCommentDeduperEmptyIDStillCountsOncePerRow(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	edge := GQLCommentEdge{}
	edge.Node.ContentOffsetSeconds = 15
	edge.Node.Message.Body = "no id"
	state.mergeEdge(edge, 0, 600)
	state.mergeEdge(edge, 0, 600)
	if count.Load() != 2 {
		t.Fatalf("empty ID rows count = %d, want 2 (no ID dedupe)", count.Load())
	}
}

func TestGQLCommentDeduperRollupMinuteCount(t *testing.T) {
	var count atomic.Int64
	state := &vodCommentsFetchState{
		commentsMap:   make(map[int][]string),
		commentsCount: &count,
	}
	edge := GQLCommentEdge{}
	edge.Node.ID = "rollup-dup"
	edge.Node.ContentOffsetSeconds = 90
	edge.Node.Message.Body = "chat"
	state.mergeEdge(edge, 0, 600)
	state.mergeEdge(edge, 0, 600)
	state.shardedComments.mergeInto(state.commentsMap)
	if len(state.commentsMap[1]) != 1 {
		t.Fatalf("minute bucket len = %d, want 1 unique chat/min entry", len(state.commentsMap[1]))
	}
}
