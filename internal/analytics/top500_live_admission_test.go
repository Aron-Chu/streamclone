package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"streamclone/internal/config"
)

type fakeLiveAdmissionSource struct {
	live []Top500Current
}

func (f *fakeLiveAdmissionSource) ListLiveCandidates(context.Context, int) ([]Top500Current, error) {
	return f.live, nil
}

func TestHelixTopLiveAdmissionSourceRankByViewer(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"data": []map[string]any{
			{"id": "1", "user_id": "u1", "user_login": "alpha", "user_name": "Alpha", "viewer_count": 9000, "started_at": "2026-07-04T12:00:00Z"},
			{"id": "2", "user_id": "u2", "user_login": "beta", "user_name": "Beta", "viewer_count": 100, "started_at": "2026-07-04T12:00:00Z"},
		},
		"pagination": map[string]any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/streams":
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	helix := NewHelixClient(srv.URL, srv.URL+"/oauth2/token", "cid", "secret", "test")
	source := &HelixTopLiveAdmissionSource{helix: helix}
	live, err := source.ListLiveCandidates(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListLiveCandidates() err = %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("len(live) = %d", len(live))
	}
	if live[0].Login != "alpha" || live[0].Rank != 1 {
		t.Fatalf("first = %+v", live[0])
	}
	if live[1].Login != "beta" || live[1].Rank != 2 {
		t.Fatalf("second = %+v", live[1])
	}
	if live[0].CoverageSource != Top500CoverageSourceHelix {
		t.Fatalf("coverage source = %q", live[0].CoverageSource)
	}
}

func TestNewReadinessLiveAdmissionSourceUsesRoster(t *testing.T) {
	store := &fakeTop500PriorityStore{}
	source := NewReadinessLiveAdmissionSource(store)
	if source == nil {
		t.Fatal("expected readiness source")
	}
	if _, ok := source.(*RosterTopLiveAdmissionSource); !ok {
		t.Fatalf("source = %T, want roster DB path", source)
	}
}

func TestNewLiveAdmissionSourceSelectsRoster(t *testing.T) {
	store := &fakeTop500PriorityStore{}
	source := NewLiveAdmissionSource(config.Config{PulseTop500AdmissionSource: "roster"}, store, nil, nil)
	if source == nil {
		t.Fatal("expected roster source")
	}
	if _, ok := source.(*RosterTopLiveAdmissionSource); !ok {
		t.Fatalf("source = %T, want *RosterTopLiveAdmissionSource", source)
	}
}

func TestNewLiveAdmissionSourceFallsBackToRosterWithoutHelix(t *testing.T) {
	store := &fakeTop500PriorityStore{}
	source := NewLiveAdmissionSource(config.Config{PulseTop500AdmissionSource: "helix_top_live"}, store, NewHelixClient("", "", "", "", ""), nil)
	if _, ok := source.(*RosterTopLiveAdmissionSource); !ok {
		t.Fatalf("source = %T, want roster fallback", source)
	}
}

func TestTop500PriorityWatchAdmitsNonRosterTopLive(t *testing.T) {
	streamID := "999"
	source := &fakeLiveAdmissionSource{live: []Top500Current{
		{Login: "viral", Rank: 1, IsLive: true, StreamID: &streamID, ViewerCount: intPtr(80000), SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	p := NewTop500PriorityWatchPoller(source, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
		PulseTop500AdmissionSource:  PulseTop500AdmissionSourceHelixTopLive,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 1 || joiner.joined[0] != "viral" {
		t.Fatalf("joined = %#v", joiner.joined)
	}
}

func TestTop500PriorityWatchSkipsDuplicateStream(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamID := "dup-stream"
	source := &fakeLiveAdmissionSource{live: []Top500Current{
		{Login: "live", Rank: 1, IsLive: true, StreamID: &streamID, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	collector.mu.Lock()
	collector.tracked["live"] = &trackedChannel{
		login:           "live",
		currentStreamID: streamID,
		addedAt:         time.Now().UTC(),
		lastViewedAt:    time.Now().UTC(),
		refCounts:       map[string]int{},
		watchPriority:   TrackPriorityTopRoster,
	}
	collector.mu.Unlock()

	p := NewTop500PriorityWatchPoller(source, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 0 {
		t.Fatalf("expected no new joins, joined=%#v", joiner.joined)
	}
	attempt, ok := getTopRosterAdmissionAttempt("live")
	if !ok {
		t.Fatal("expected admission attempt")
	}
	if attempt.Outcome != TopRosterAdmissionDuplicateStream {
		t.Fatalf("outcome = %q, want %q", attempt.Outcome, TopRosterAdmissionDuplicateStream)
	}
}

func TestTop500PriorityWatchDuplicateStreamRefreshesIdleClock(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamID := "dup-stream"
	source := &fakeLiveAdmissionSource{live: []Top500Current{
		{Login: "live", Rank: 1, IsLive: true, StreamID: &streamID, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Second, time.Hour, 200)
	collector.WithIdleTTL(time.Millisecond)
	collector.mu.Lock()
	collector.tracked["live"] = &trackedChannel{
		login:           "live",
		currentStreamID: streamID,
		addedAt:         time.Now().UTC(),
		lastViewedAt:    time.Now().UTC().Add(-time.Hour),
		refCounts:       map[string]int{},
		watchPriority:   TrackPriorityTopRoster,
	}
	collector.mu.Unlock()

	p := NewTop500PriorityWatchPoller(source, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())

	collector.evictIdleChannels(time.Now().UTC())
	if !collector.IsTracking("live") {
		t.Fatal("duplicate_stream admission should refresh idle and retain tracked channel")
	}
	if len(joiner.parted) != 0 {
		t.Fatalf("expected no PART after duplicate_stream refresh, parted=%v", joiner.parted)
	}
	attempt, ok := getTopRosterAdmissionAttempt("live")
	if !ok {
		t.Fatal("expected admission attempt")
	}
	if attempt.Outcome != TopRosterAdmissionDuplicateStream {
		t.Fatalf("outcome = %q, want %q", attempt.Outcome, TopRosterAdmissionDuplicateStream)
	}
}

func TestTop500PriorityWatchProtectedPreemptsTopLive(t *testing.T) {
	streamA := "111"
	source := &fakeLiveAdmissionSource{live: []Top500Current{
		{Login: "roster", Rank: 1, IsLive: true, StreamID: &streamA, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 1, time.Hour, time.Hour, 200)
	p := NewTop500PriorityWatchPoller(source, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 1 || joiner.joined[0] != "roster" {
		t.Fatalf("expected roster admission first, joined=%#v", joiner.joined)
	}

	resp := collector.WatchWithPriority(context.Background(), "protected", "", TrackPriorityGlobalProtected)
	if !resp.Tracking {
		t.Fatalf("expected protected channel to preempt top-roster: %+v", resp)
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "roster" {
		t.Fatalf("expected roster preempted, parted=%#v", joiner.parted)
	}
}
