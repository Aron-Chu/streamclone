package analytics

import (
	"context"
	"testing"
	"time"

	"streamclone/internal/config"
)

func TestTopRosterAdmissionObservationRefreshesIdleClock(t *testing.T) {
	joiner := &fakeJoiner{}
	c := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Second, time.Hour, 200)
	c.WithIdleTTL(time.Millisecond)

	resp := c.WatchWithPriority(context.Background(), "roster", "", TrackPriorityTopRoster)
	if !resp.Tracking {
		t.Fatalf("expected roster channel to track: %+v", resp)
	}

	c.mu.Lock()
	c.tracked["roster"].lastViewedAt = time.Now().UTC().Add(-time.Hour)
	c.mu.Unlock()

	if !c.TouchAdmissionObservation("roster") {
		t.Fatal("expected touch on tracked roster channel")
	}

	c.evictIdleChannels(time.Now().UTC())
	if !c.IsTracking("roster") {
		t.Fatal("expected roster channel retained after admission observation touch")
	}
	if len(joiner.parted) != 0 {
		t.Fatalf("expected no PART after touch, parted=%v", joiner.parted)
	}
}

func TestTopRosterIdleEvictsWithoutAdmissionTouch(t *testing.T) {
	joiner := &fakeJoiner{}
	c := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Second, time.Hour, 200)
	c.WithIdleTTL(time.Millisecond)

	resp := c.WatchWithPriority(context.Background(), "roster", "", TrackPriorityTopRoster)
	if !resp.Tracking {
		t.Fatalf("expected roster channel to track: %+v", resp)
	}

	c.mu.Lock()
	c.tracked["roster"].lastViewedAt = time.Now().UTC().Add(-time.Hour)
	c.mu.Unlock()

	c.evictIdleChannels(time.Now().UTC())
	if c.IsTracking("roster") {
		t.Fatal("expected idle eviction without admission touch")
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "roster" {
		t.Fatalf("expected roster PART after idle TTL, parted=%v", joiner.parted)
	}
}

func TestTouchAdmissionObservationDoesNotGrantProtectedRetention(t *testing.T) {
	joiner := &fakeJoiner{}
	c := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Second, time.Hour, 200)
	c.WithIdleTTL(time.Millisecond)

	resp := c.WatchWithPriority(context.Background(), "roster", "", TrackPriorityTopRoster)
	if !resp.Tracking {
		t.Fatalf("expected roster channel to track: %+v", resp)
	}

	c.mu.Lock()
	tc := c.tracked["roster"]
	tc.lastViewedAt = time.Now().UTC().Add(-time.Hour)
	tc.poolAlwaysTrack = false
	c.mu.Unlock()

	if !c.TouchAdmissionObservation("roster") {
		t.Fatal("expected touch on tracked roster channel")
	}

	c.mu.Lock()
	tc = c.tracked["roster"]
	if tc.poolAlwaysTrack {
		t.Fatal("touch must not set poolAlwaysTrack")
	}
	if c.channelRefCount(tc) != 0 {
		t.Fatal("touch must not add principal refs")
	}
	c.mu.Unlock()

	c.mu.Lock()
	c.tracked["roster"].lastViewedAt = time.Now().UTC().Add(-time.Hour)
	c.mu.Unlock()
	c.evictIdleChannels(time.Now().UTC())
	if c.IsTracking("roster") {
		t.Fatal("expected eventual idle eviction when admission observation stops")
	}
}

func TestTop500PriorityWatchPollerRefreshesIdleForAlreadyTracking(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamA := "111"
	source := &fakeLiveAdmissionSource{live: []Top500Current{
		{Login: "live", Rank: 4, IsLive: true, StreamID: &streamA, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Second, time.Hour, 200)
	collector.WithIdleTTL(time.Millisecond)
	collector.WatchWithPriority(context.Background(), "live", "", TrackPriorityTopRoster)
	collector.mu.Lock()
	collector.tracked["live"].lastViewedAt = time.Now().UTC().Add(-time.Hour)
	collector.mu.Unlock()

	p := NewLiveAdmissionPoller(source, collector, config.Config{
		PulseLiveAdmissionEnabled: true,
		PulseLiveAdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())

	collector.evictIdleChannels(time.Now().UTC())
	if !collector.IsTracking("live") {
		t.Fatal("poller should refresh idle for already-tracking top-roster channel")
	}
	if len(joiner.parted) != 0 {
		t.Fatalf("expected no PART after poller refresh, parted=%v", joiner.parted)
	}
}
