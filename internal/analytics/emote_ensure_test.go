package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type slowEmoteHTTPServer struct {
	calls atomic.Int32
}

func (s *slowEmoteHTTPServer) handler(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	time.Sleep(200 * time.Millisecond)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"state":   "processing",
		"count":   0,
		"pending": 1,
	})
}

func TestWatchReturnsBeforeEmoteEnsureCompletes(t *testing.T) {
	slow := &slowEmoteHTTPServer{}
	srv := httptest.NewServer(http.HandlerFunc(slow.handler))
	defer srv.Close()

	ensurer := NewLiveEmoteEnsurer(LiveEmoteEnsurerConfig{
		EmoteURL:      srv.URL,
		Resolver:      fakeProvider{users: map[string]UserProfile{"xqc": {ID: "71092938"}}},
		Cooldown:      time.Minute,
		MaxConcurrent: 2,
	})
	c := NewCollector(nil, nil, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, time.Hour, 10).
		WithLiveEmoteEnsurer(ensurer)

	start := time.Now()
	resp := c.Watch(context.Background(), "xqc")
	elapsed := time.Since(start)

	if !resp.Tracking {
		t.Fatalf("tracking = false, want true")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("watch blocked for %s, want immediate IRC join", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if slow.calls.Load() > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("expected async emote ensure to start after watch")
}

func TestLiveEmoteEnsurerDedupesKickoff(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"state": "ready", "count": 10, "pending": 0})
	}))
	defer srv.Close()

	ensurer := NewLiveEmoteEnsurer(LiveEmoteEnsurerConfig{
		EmoteURL:      srv.URL,
		Resolver:      fakeProvider{users: map[string]UserProfile{"xqc": {ID: "71092938"}}},
		Cooldown:      time.Minute,
		MaxConcurrent: 1,
	})

	ensurer.Kickoff(context.Background(), "xqc")
	ensurer.Kickoff(context.Background(), "xqc")
	ensurer.Kickoff(context.Background(), "xqc")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("ensure calls = %d, want 1 deduped in-flight kickoff", got)
	}
}

func TestRequireReadyForGoldFailsWhenNotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"state":   "processing",
			"count":   0,
			"pending": 1,
		})
	}))
	defer srv.Close()

	client := NewEmoteEnsureClient(srv.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := client.RequireReadyForGold(ctx, "xqc", "71092938", nil)
	if err == nil {
		t.Fatal("expected gold ensure to fail when dictionary never becomes ready")
	}
}

func TestRequireReadyForGoldSkipsWhenEmoteURLUnset(t *testing.T) {
	client := NewEmoteEnsureClient("", nil)
	if err := client.RequireReadyForGold(context.Background(), "xqc", "", nil); err != nil {
		t.Fatalf("unset emote URL should skip gold ensure, got %v", err)
	}
}

func TestCollectorStartKicksEmoteEnsureForAlwaysTracked(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"state": "ready", "count": 10, "pending": 0})
	}))
	defer srv.Close()

	resolver := fakeProvider{users: map[string]UserProfile{"xqc": {ID: "71092938"}}}
	ensurer := NewLiveEmoteEnsurer(LiveEmoteEnsurerConfig{
		EmoteURL:      srv.URL,
		Resolver:      resolver,
		Cooldown:      time.Minute,
		MaxConcurrent: 2,
	})
	c := NewCollector(nil, resolver, &fakeJoiner{}, nil, nilLogger(), 5, time.Hour, time.Hour, 10).
		WithAlwaysTracked([]string{"xqc"}).
		WithLiveEmoteEnsurer(ensurer)

	c.Start(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("expected async emote ensure to start for always-tracked channels at startup")
}

func TestEmoteEnsureDictionaryUsable(t *testing.T) {
	if !emoteEnsureDictionaryUsable(emoteEnsureResponse{State: "processing", Count: 405, Pending: 1, Providers: []struct {
		Provider string `json:"provider"`
		State    string `json:"state"`
		Count    int    `json:"count"`
		Error    string `json:"error"`
	}{{Provider: "twitch", Count: 405}}}) {
		t.Fatal("expected channel with twitch emotes ready to be usable")
	}
	if emoteEnsureDictionaryUsable(emoteEnsureResponse{State: "processing", Count: 0, Pending: 1}) {
		t.Fatal("expected empty dictionary to be unusable")
	}
}

func TestEmoteSyncSnapshotMessages(t *testing.T) {
	ready := emoteSyncSnapshotForState(EmoteSyncReady, true, "fresh", nil)
	if ready.Message != "7TV synced" {
		t.Fatalf("ready message = %q", ready.Message)
	}
	stale := emoteSyncSnapshotForState(EmoteSyncStale, false, "cache", nil)
	if stale.Message != "7TV stale — using cached set" {
		t.Fatalf("stale message = %q", stale.Message)
	}
	unavailable := emoteSyncSnapshotForState(EmoteSyncUnavailable, false, "", nil)
	if unavailable.Message == "" {
		t.Fatal("expected unavailable message")
	}
}
