package render

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEnqueueAsyncCoalescesDuplicateKeys(t *testing.T) {
	q := &Queue{
		asyncSem:      make(chan struct{}, maxAsyncEnqueue),
		asyncInFlight: make(map[string]struct{}),
	}
	key := "e1|1x"
	q.mu.Lock()
	q.asyncInFlight[key] = struct{}{}
	q.mu.Unlock()

	q.EnqueueAsync(context.Background(), Request{EmoteID: "e1", Scale: "1x", Reason: ReasonUIRequest})
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.asyncInFlight) != 1 {
		t.Fatalf("expected coalesce keep one in-flight, got %d", len(q.asyncInFlight))
	}
}

func TestEnqueueAsyncDropsWhenSemaphoreFull(t *testing.T) {
	q := &Queue{
		asyncSem:      make(chan struct{}, 1),
		asyncInFlight: make(map[string]struct{}),
		cfg:           Config{OnUIRequest: true},
	}
	q.asyncSem <- struct{}{}

	before := q.asyncDropped
	q.EnqueueAsync(context.Background(), Request{EmoteID: "a", Scale: "1x", Reason: ReasonUIRequest})
	if q.asyncDropped != before+1 {
		t.Fatalf("asyncDropped = %d, want %d", q.asyncDropped, before+1)
	}
}

func TestEnqueueAsyncReleasesSemaphore(t *testing.T) {
	q := NewQueue(nil, nil, Config{}, nil)
	var wg sync.WaitGroup
	for i := 0; i < maxAsyncEnqueue+4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.EnqueueAsync(context.Background(), Request{
				EmoteID: "id",
				Scale:   "1x",
				Reason:  ReasonUIRequest,
			})
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EnqueueAsync callers blocked")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		inflight := len(q.asyncInFlight)
		q.mu.Unlock()
		if inflight == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	q.mu.Lock()
	inflight := len(q.asyncInFlight)
	q.mu.Unlock()
	t.Fatalf("in-flight leaked: %d", inflight)
}
