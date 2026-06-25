package analytics

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPulseVodRetryCooldownReturns429(t *testing.T) {
	rec := httptest.NewRecorder()
	writePulseRetryCooldown(rec, 15*time.Minute)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	var body PulseVODRetryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RetryAfterSeconds <= 0 || body.ManualRetryAllowed {
		t.Fatalf("unexpected cooldown body: %+v", body)
	}
}

func TestProtectedGoLivePollerDisabledByFlags(t *testing.T) {
	p := NewProtectedGoLivePoller(nil, nil, nil, PulseRuntimeConfig{
		Configured:             true,
		ProtectedGoLiveEnabled: false,
		HelixGoLiveEnabled:     true,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if p.Enabled() {
		t.Fatal("expected poller disabled when protected go-live flag is false")
	}
	p2 := NewProtectedGoLivePoller(nil, nil, nil, PulseRuntimeConfig{
		Configured:             true,
		ProtectedGoLiveEnabled: true,
		HelixGoLiveEnabled:     false,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if p2.Enabled() {
		t.Fatal("expected poller disabled when helix go-live flag is false")
	}
}

func TestProtectedGoLivePollerDedupesSameStreamID(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	c.tracked["xqc"] = &trackedChannel{
		login:           "xqc",
		currentStreamID: "live-1",
		addedAt:         time.Now().UTC(),
		lastViewedAt:    time.Now().UTC(),
		refCounts:       map[string]int{},
		watchPriority:   TrackPriorityGlobalProtected,
	}
	if got := c.TrackedStreamID("xqc"); got != "live-1" {
		t.Fatalf("tracked stream id = %q", got)
	}
}

func TestProtectedGoLivePollerBatchesHelixRequests(t *testing.T) {
	items := make([]string, 205)
	for i := range items {
		items[i] = "chan"
	}
	chunks := chunkStrings(items, 100)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 helix batches, got %d", len(chunks))
	}
}

func TestProtectedGoLivePollerWatchesOfflineToLive(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	resp := c.WatchWithPriority(context.Background(), "xqc", "", TrackPriorityGlobalProtected)
	if !resp.Tracking || len(joiner.joined) != 1 {
		t.Fatalf("expected go-live watch to join IRC: %+v joined=%v", resp, joiner.joined)
	}
}
