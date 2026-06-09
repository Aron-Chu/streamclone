package batch

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestFirstMessageFlushesImmediately(t *testing.T) {
	var mu sync.Mutex
	var got []Frame

	b := New(200, func(_ string, data []byte) {
		var f Frame
		if err := json.Unmarshal(data, &f); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		got = append(got, f)
		mu.Unlock()
	})

	b.Add("streamer", BatchMessage{ID: "1", User: "alice", Fragments: []Fragment{{T: "text", C: "hi"}}})

	time.Sleep(25 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(got) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(got))
	}
	if got[0].Type != "batch" {
		t.Errorf("type: got %q", got[0].Type)
	}
	if got[0].Channel != "streamer" {
		t.Errorf("channel: got %q", got[0].Channel)
	}
	if got[0].ServerSentTS == 0 {
		t.Errorf("expected server_sent_ts")
	}
	if len(got[0].Messages) != 1 {
		t.Errorf("messages count: got %d", len(got[0].Messages))
	}
	if got[0].Messages[0].ServerReceivedTS == 0 {
		t.Errorf("expected server_received_ts")
	}
}

func TestBurstMessagesFlushAfterWindow(t *testing.T) {
	var mu sync.Mutex
	var got []Frame

	b := New(75, func(_ string, data []byte) {
		var f Frame
		if err := json.Unmarshal(data, &f); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		got = append(got, f)
		mu.Unlock()
	})

	b.Add("streamer", BatchMessage{ID: "1", User: "alice", Fragments: []Fragment{{T: "text", C: "hi"}}})
	b.Add("streamer", BatchMessage{ID: "2", User: "bob", Fragments: []Fragment{{T: "text", C: "hey"}}})
	b.Add("streamer", BatchMessage{ID: "3", User: "chris", Fragments: []Fragment{{T: "text", C: "yo"}}})

	time.Sleep(25 * time.Millisecond)

	mu.Lock()
	count := len(got)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected only immediate flush before window, got %d", count)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(got) != 2 {
		t.Fatalf("expected immediate and burst flush, got %d", len(got))
	}
	if len(got[0].Messages) != 1 || got[0].Messages[0].ID != "1" {
		t.Fatalf("unexpected immediate frame: %+v", got[0])
	}
	if len(got[1].Messages) != 2 {
		t.Fatalf("expected 2 burst messages, got %d", len(got[1].Messages))
	}
}

func TestZeroWindowFlushesEveryMessage(t *testing.T) {
	var mu sync.Mutex
	flushCount := 0

	b := New(0, func(_ string, _ []byte) {
		mu.Lock()
		flushCount++
		mu.Unlock()
	})

	b.Add("streamer", BatchMessage{ID: "1"})
	time.Sleep(10 * time.Millisecond)
	b.Add("streamer", BatchMessage{ID: "2"})
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	count := flushCount
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 flushes, got %d", count)
	}
}
