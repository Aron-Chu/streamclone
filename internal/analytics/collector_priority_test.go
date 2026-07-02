package analytics

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestProtectedChannelPreemptsTopRosterChannel(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, 30*24*time.Hour, 200)
	now := time.Now().UTC()
	c.tracked["roster"] = &trackedChannel{
		login:         "roster",
		addedAt:       now.Add(-time.Minute),
		lastViewedAt:  now.Add(-time.Minute),
		refCounts:     map[string]int{},
		watchPriority: TrackPriorityTopRoster,
	}
	c.tracked["other"] = &trackedChannel{
		login:         "other",
		addedAt:       now,
		lastViewedAt:  now,
		refCounts:     map[string]int{},
		watchPriority: TrackPriorityTopRoster,
	}

	resp := c.WatchWithPriority(context.Background(), "protected", "", TrackPriorityGlobalProtected)
	if !resp.Tracking {
		t.Fatalf("expected protected channel to join via preemption: %+v", resp)
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "roster" {
		t.Fatalf("expected top-roster channel preempted, parted=%v", joiner.parted)
	}
}

func TestTopRosterDoesNotPreemptProtectedChannel(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 1, time.Hour, 30*24*time.Hour, 200)
	c.alwaysTracked["protected"] = true
	c.tracked["protected"] = &trackedChannel{
		login:        "protected",
		addedAt:      time.Now().UTC(),
		lastViewedAt: time.Now().UTC(),
		refCounts:    map[string]int{},
	}

	resp := c.WatchWithPriority(context.Background(), "roster", "", TrackPriorityTopRoster)
	if resp.Tracking {
		t.Fatalf("expected top-roster watch to be rejected at capacity: %+v", resp)
	}
	if len(joiner.parted) != 0 {
		t.Fatalf("expected no preemption, parted=%v", joiner.parted)
	}
}

func TestManualWatchPreemptsTopRosterOnly(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 1, time.Hour, 30*24*time.Hour, 200)
	c.tracked["roster"] = &trackedChannel{
		login:         "roster",
		addedAt:       time.Now().UTC(),
		lastViewedAt:  time.Now().UTC(),
		refCounts:     map[string]int{},
		watchPriority: TrackPriorityTopRoster,
	}

	ok := c.WatchWithPriority(context.Background(), "viewer", "principal-a", TrackPriorityManualWatch)
	if !ok.Tracking {
		t.Fatalf("expected manual watch to preempt top roster: %+v", ok)
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "roster" {
		t.Fatalf("expected roster preempted, parted=%v", joiner.parted)
	}

	c2 := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 1, time.Hour, 30*24*time.Hour, 200)
	c2.tracked["protected"] = &trackedChannel{
		login:         "protected",
		addedAt:       time.Now().UTC(),
		lastViewedAt:  time.Now().UTC(),
		refCounts:     map[string]int{"p": 1},
		watchPriority: TrackPriorityManualWatch,
	}
	blocked := c2.WatchWithPriority(context.Background(), "late", "principal-b", TrackPriorityManualWatch)
	if blocked.Tracking {
		t.Fatalf("expected manual watch blocked by equal/higher manual watch: %+v", blocked)
	}
}

func TestCollectorInvalidatesPulseCacheOnVodResolved(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", Login: "chan", BroadcasterID: "bc-1"}}
	var mu sync.Mutex
	var calls [][2]string
	c := NewCollector(store, fakeProvider{vodID: "vod-9"}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.WithPulseCacheInvalidator(func(_ context.Context, login, streamID string, includeHeatmap bool) {
		mu.Lock()
		calls = append(calls, [2]string{login, streamID})
		mu.Unlock()
	})
	c.vodResolveOffsets = []time.Duration{0}

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0][0] != "chan" {
		t.Fatalf("expected BFF cache invalidation on vod resolve, got %v", calls)
	}
	if calls[0][1] != "" {
		t.Fatalf("vod link must not pass streamID for heatmap invalidation, got %q", calls[0][1])
	}
}

func TestCollectorInvalidatesPulseCacheOnVodUnavailable(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", Login: "chan", BroadcasterID: "bc-1"}}
	var mu sync.Mutex
	var calls [][2]string
	c := NewCollector(store, fakeProvider{vodID: ""}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.WithPulseCacheInvalidator(func(_ context.Context, login, streamID string, includeHeatmap bool) {
		mu.Lock()
		calls = append(calls, [2]string{login, streamID})
		mu.Unlock()
	})
	c.vodResolveOffsets = []time.Duration{0}

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0][0] != "chan" {
		t.Fatalf("expected BFF cache invalidation on vod unavailable, got %v", calls)
	}
	if calls[0][1] != "" {
		t.Fatalf("terminal unlinked must not pass streamID for heatmap invalidation, got %q", calls[0][1])
	}
}
