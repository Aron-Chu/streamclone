package analytics

import (
	"context"
	"sync"
	"testing"
	"time"

	"streamclone/internal/chat/batch"
)

func TestOpenMinuteFlushIntervalDefaultAndCustom(t *testing.T) {
	store := &capturingRollupStore{}
	c := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, 30*24*time.Hour, 200)
	if c.openMinuteFlushIntervalOrDefault() != defaultOpenMinuteFlushMinInterval {
		t.Fatalf("default interval = %v, want %v", c.openMinuteFlushIntervalOrDefault(), defaultOpenMinuteFlushMinInterval)
	}
	c.WithOpenMinuteFlushInterval(250 * time.Millisecond)
	if c.openMinuteFlushIntervalOrDefault() != 250*time.Millisecond {
		t.Fatalf("custom interval = %v", c.openMinuteFlushIntervalOrDefault())
	}

	now := time.Now().UTC()
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: "stream-open-interval"}
	c.addViewerSample("stream-open-interval", now, 100)
	c.flushOpenMinuteToStore(context.Background(), "stream-open-interval")
	if len(store.flushedRollups()) != 1 {
		t.Fatalf("expected first flush, got %d", len(store.flushedRollups()))
	}
	c.addViewerSample("stream-open-interval", now, 200)
	c.flushOpenMinuteToStore(context.Background(), "stream-open-interval")
	if len(store.flushedRollups()) != 1 {
		t.Fatal("expected rate limit before custom interval elapsed")
	}
	time.Sleep(260 * time.Millisecond)
	c.flushOpenMinuteToStore(context.Background(), "stream-open-interval")
	if len(store.flushedRollups()) != 2 {
		t.Fatalf("expected second flush after interval, got %d", len(store.flushedRollups()))
	}
}

func TestOpenMinuteFlushUsesOpenWriteModeCompletedUsesCompletedMode(t *testing.T) {
	store := &capturingRollupStore{}
	c := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, 30*24*time.Hour, 200)
	now := time.Now().UTC().Truncate(time.Minute)
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: "stream-mode"}
	c.addViewerSample("stream-mode", now, 50)
	c.flushOpenMinuteToStore(context.Background(), "stream-mode")
	if store.lastWriteMode != LiveRollupWriteOpenMinute {
		t.Fatalf("open flush mode = %q, want %q", store.lastWriteMode, LiveRollupWriteOpenMinute)
	}

	priorMinute := now.Add(-time.Minute)
	shard := c.bucketShard("stream-mode")
	shard.mu.Lock()
	key := "stream-mode|" + priorMinute.Format(time.RFC3339)
	shard.buckets[key] = &minuteAccumulator{
		streamID:  "stream-mode",
		minute:    priorMinute,
		emotes:    map[string]int{},
		chatCount: 3,
	}
	shard.mu.Unlock()
	c.flushCompleted(context.Background(), now)
	if store.lastWriteMode != LiveRollupWriteCompletedMinute {
		t.Fatalf("completed flush mode = %q, want %q", store.lastWriteMode, LiveRollupWriteCompletedMinute)
	}
}

func TestConcurrentChatIngestionPreservesCounts(t *testing.T) {
	store := &capturingRollupStore{}
	c := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, 30*24*time.Hour, 200)
	streamID := "stream-concurrent"
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: streamID}
	at := time.Now().UTC().Truncate(time.Minute)
	const workers = 8
	const perWorker = 250
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				c.addChat(streamID, at.Add(time.Duration(j)*time.Millisecond).UnixMilli(), []batch.Fragment{
					{T: "text"},
					{T: "emote", Provider: "seventv", ID: "1", C: "KEKW"},
				})
			}
		}(i)
	}
	wg.Wait()
	shard := c.bucketShard(streamID)
	shard.mu.Lock()
	key := streamID + "|" + at.Format(time.RFC3339)
	acc := shard.buckets[key]
	shard.mu.Unlock()
	if acc == nil {
		t.Fatal("expected minute accumulator")
	}
	wantChat := workers * perWorker
	if acc.chatCount != wantChat {
		t.Fatalf("chatCount = %d, want %d", acc.chatCount, wantChat)
	}
	if acc.totalEmoteCount != wantChat {
		t.Fatalf("totalEmoteCount = %d, want %d", acc.totalEmoteCount, wantChat)
	}
	if acc.sevenTVEmoteCount != wantChat {
		t.Fatalf("sevenTVEmoteCount = %d, want %d", acc.sevenTVEmoteCount, wantChat)
	}
}

func TestOpenMinuteFlushConcurrentWithChatIngestion(t *testing.T) {
	store := &capturingRollupStore{}
	c := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, 30*24*time.Hour, 200).
		WithOpenMinuteFlushInterval(time.Millisecond)
	streamID := "stream-race"
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: streamID}
	at := time.Now().UTC().Truncate(time.Minute)
	const workers = 6
	var chatWG sync.WaitGroup
	var flushWG sync.WaitGroup
	flushWG.Add(1)
	go func() {
		defer flushWG.Done()
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			c.flushOpenMinuteToStore(context.Background(), streamID)
		}
	}()
	chatWG.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer chatWG.Done()
			for j := 0; j < 500; j++ {
				c.addChat(streamID, at.Add(time.Duration(j)*time.Millisecond).UnixMilli(), []batch.Fragment{
					{T: "text"},
					{T: "emote", Provider: "seventv", ID: "1", C: "KEKW"},
				})
			}
		}()
	}
	chatWG.Wait()
	flushWG.Wait()
}
