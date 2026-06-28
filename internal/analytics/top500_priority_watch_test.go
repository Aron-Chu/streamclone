package analytics

import (
	"context"
	"testing"
	"time"

	"streamclone/internal/config"
)

type fakeTop500PriorityStore struct {
	live []Top500Current
}

func (f *fakeTop500PriorityStore) ListTop500LiveForPriorityWatch(context.Context, int, int) ([]Top500Current, error) {
	return f.live, nil
}

func TestTop500PriorityWatchPollerDisabled(t *testing.T) {
	p := NewTop500PriorityWatchPoller(nil, nil, config.Config{}, nil)
	if p.Enabled() {
		t.Fatal("expected disabled by default")
	}
}

func TestTop500PriorityWatchPollerAdmitsByViewerOrder(t *testing.T) {
	streamA := "111"
	streamB := "222"
	store := &fakeTop500PriorityStore{live: []Top500Current{
		{Login: "big", IsLive: true, StreamID: &streamA, ViewerCount: intPtr(50000)},
		{Login: "small", IsLive: true, StreamID: &streamB, ViewerCount: intPtr(1000)},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	p := NewTop500PriorityWatchPoller(store, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 2 || joiner.joined[0] != "big" || joiner.joined[1] != "small" {
		t.Fatalf("joined = %#v", joiner.joined)
	}
}

func TestTop500PriorityWatchPollerStopsAtCapacity(t *testing.T) {
	streamA := "111"
	streamB := "222"
	streamC := "333"
	store := &fakeTop500PriorityStore{live: []Top500Current{
		{Login: "a", IsLive: true, StreamID: &streamA},
		{Login: "b", IsLive: true, StreamID: &streamB},
		{Login: "c", IsLive: true, StreamID: &streamC},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 1, time.Hour, time.Hour, 200)
	p := NewTop500PriorityWatchPoller(store, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 1 {
		t.Fatalf("expected one admission at cap=1, joined=%#v", joiner.joined)
	}
}
