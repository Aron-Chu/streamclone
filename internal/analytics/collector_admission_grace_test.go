package analytics

import (
	"context"
	"testing"
	"time"

	"streamclone/internal/config"
)

func TestReconcileTopRosterAdmissionMissesEvictsAfterGrace(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	c.tracked["roster"] = &trackedChannel{
		login:           "roster",
		currentStreamID: "stream-roster-evict",
		watchPriority:   TrackPriorityTopRoster,
		addedAt:         time.Now().UTC(),
		lastViewedAt:    time.Now().UTC().Add(-time.Hour),
		refCounts:       map[string]int{},
	}
	desired := map[string]struct{}{"other": {}}
	if n := c.ReconcileTopRosterAdmissionMisses(desired, 3); n != 0 {
		t.Fatalf("first cycle evicted = %d, want 0", n)
	}
	if n := c.ReconcileTopRosterAdmissionMisses(desired, 3); n != 0 {
		t.Fatalf("second cycle evicted = %d, want 0", n)
	}
	if n := c.ReconcileTopRosterAdmissionMisses(desired, 3); n != 1 {
		t.Fatalf("third cycle evicted = %d, want 1", n)
	}
	if c.IsTracking("roster") {
		t.Fatal("expected roster channel evicted after grace")
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "roster" {
		t.Fatalf("parted = %#v, want [roster]", joiner.parted)
	}
	if len(store.closed) != 1 || store.closed[0] != "stream-roster-evict" {
		t.Fatalf("closed streams = %#v, want [stream-roster-evict]", store.closed)
	}
}

func TestReconcileTopRosterAdmissionMissesRetainsProtectedAndManual(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	c.alwaysTracked["protected"] = true
	c.tracked["protected"] = &trackedChannel{
		login:         "protected",
		watchPriority: TrackPriorityTopRoster,
		refCounts:     map[string]int{},
	}
	c.tracked["manual"] = &trackedChannel{
		login:         "manual",
		watchPriority: TrackPriorityTopRoster,
		refCounts:     map[string]int{"principal-a": 1},
	}
	desired := map[string]struct{}{}
	for i := 0; i < 5; i++ {
		c.ReconcileTopRosterAdmissionMisses(desired, 2)
	}
	if !c.IsTracking("protected") || !c.IsTracking("manual") {
		t.Fatalf("protected/manual retained: protected=%v manual=%v", c.IsTracking("protected"), c.IsTracking("manual"))
	}
	if len(joiner.parted) != 0 {
		t.Fatalf("unexpected IRC part: %#v", joiner.parted)
	}
}

func TestTop500PriorityWatchPollerEvictsStaleRosterAfterGrace(t *testing.T) {
	source := &fakeTop500PriorityStore{live: []Top500Current{
		{Login: "live1", IsLive: true, StreamID: strPtr("s-live1"), Rank: 1},
	}}
	store := &fakeStore{}
	collector := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	collector.tracked["stale"] = &trackedChannel{
		login:               "stale",
		currentStreamID:     "stream-stale",
		watchPriority:       TrackPriorityTopRoster,
		admissionMissCycles: 2,
		refCounts:           map[string]int{},
	}
	p := NewTop500PriorityWatchPoller(source, collector, config.Config{
		PulseTop500AdmissionEnabled:         true,
		PulseTop500AdmissionTopN:            10,
		PulseTop500AdmissionMissGraceCycles: 3,
	}, nil)
	p.runOnce(context.Background())
	if collector.IsTracking("stale") {
		t.Fatal("expected stale roster channel evicted on admission cycle")
	}
	if len(store.closed) != 1 || store.closed[0] != "stream-stale" {
		t.Fatalf("closed streams = %#v, want [stream-stale]", store.closed)
	}
}

func strPtr(v string) *string { return &v }
