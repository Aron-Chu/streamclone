package render

import (
	"testing"
	"time"
)

func TestObservePublisherLocalDedupe(t *testing.T) {
	pub := NewObservePublisher(nil, nil)
	payload := ObservePayload{EmoteID: "75f49395-d5fc-41da-998c-880c6d8fddcb", Provider: "seventv", Scale: "1x"}

	pub.TryPublish(payload)
	pub.TryPublish(payload)

	pub.mu.Lock()
	count := len(pub.localSeen)
	pub.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected one local dedupe entry, got %d", count)
	}
}

func TestObservePublisherAllowsAfterWindow(t *testing.T) {
	pub := NewObservePublisher(nil, nil)
	pub.dedupeWindow = time.Millisecond
	payload := ObservePayload{EmoteID: "abc", Provider: "bttv", Scale: "1x"}
	pub.TryPublish(payload)
	time.Sleep(2 * time.Millisecond)
	pub.TryPublish(payload)
	pub.mu.Lock()
	count := len(pub.localSeen)
	pub.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected single key refreshed, got %d entries", count)
	}
}

func TestResolveScalesRequestedScaleOnly(t *testing.T) {
	got := ResolveScales([]string{"2x"}, []string{"1x"}, []string{"1x", "2x", "3x"})
	if len(got) != 1 || got[0] != "2x" {
		t.Fatalf("got %#v", got)
	}
}
