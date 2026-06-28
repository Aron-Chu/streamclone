package analytics

import (
	"context"
	"sync"
	"testing"
	"time"
)

type cacheInvalidateRecorder struct {
	mu       sync.Mutex
	bff      []string
	heatmap  []string
	fullBoth []string
}

func (r *cacheInvalidateRecorder) hook(includeHeatmap bool) func(ctx context.Context, login, streamID string, heatmap bool) {
	return func(_ context.Context, login, streamID string, hm bool) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if hm {
			r.fullBoth = append(r.fullBoth, login+":"+streamID)
			r.heatmap = append(r.heatmap, streamID)
		}
		r.bff = append(r.bff, login)
	}
}

func TestVodTerminalUnlinkedInvalidatesBFFOnly(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", Login: "chan1", BroadcasterID: "bc-1"}}
	rec := &cacheInvalidateRecorder{}
	c := NewCollector(store, fakeProvider{vodID: ""}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0}
	c.WithPulseCacheInvalidator(rec.hook(false))

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bff) == 0 {
		t.Fatal("expected BFF invalidation on terminal unlinked")
	}
	if len(rec.heatmap) != 0 {
		t.Fatalf("terminal unlinked must not invalidate heatmap, got %v", rec.heatmap)
	}
}

func TestVodLinkInvalidatesBFFOnly(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", Login: "chan1", BroadcasterID: "bc-1"}}
	rec := &cacheInvalidateRecorder{}
	c := NewCollector(store, fakeProvider{vodID: "vod-9"}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0}
	c.WithPulseCacheInvalidator(func(ctx context.Context, login, streamID string, includeHeatmap bool) {
		rec.hook(includeHeatmap)(ctx, login, streamID, includeHeatmap)
	})

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bff) == 0 {
		t.Fatal("expected BFF invalidation on vod link")
	}
	if len(rec.heatmap) != 0 {
		t.Fatalf("vod link must not invalidate heatmap, got %v", rec.heatmap)
	}
}

func TestVodResolveFakeClockTerminal60m(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", Login: "chan1", BroadcasterID: "bc-1"}}
	rec := &cacheInvalidateRecorder{}
	closedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Fake clock past the final 60m retry — all offsets elapse without real sleeps.
	now := closedAt.Add(61 * time.Minute)
	c := NewCollector(store, fakeProvider{vodID: ""}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.WithNowClock(func() time.Time { return now })
	c.WithPulseCacheInvalidator(func(ctx context.Context, login, streamID string, includeHeatmap bool) {
		rec.hook(includeHeatmap)(ctx, login, streamID, includeHeatmap)
	})

	c.resolveVodIDWithRetry("stream-1", closedAt)

	if len(store.unlinked) != 1 {
		t.Fatalf("expected terminal unlinked after full offset window, got %v", store.unlinked)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bff) == 0 {
		t.Fatal("expected BFF invalidation on terminal unlinked")
	}
	if len(rec.heatmap) != 0 {
		t.Fatalf("60m terminal path must not invalidate heatmap, got %v", rec.heatmap)
	}
}

func TestInvalidatePulseCachesNilRedisDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	InvalidatePulseBFFCache(ctx, nil, "chan", nil)
	InvalidatePulseHeatmapCache(ctx, nil, "stream-1", nil)
	InvalidatePulseCaches(ctx, nil, nil, "chan", "stream-1", nil)
}
