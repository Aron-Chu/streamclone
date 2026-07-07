package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type reconcilePulseStore struct {
	mu       sync.Mutex
	byLogin  map[string]*StreamRecord
	closed   []string
	upserted []LiveStream
}

func newReconcilePulseStore(initial map[string]*StreamRecord) *reconcilePulseStore {
	byLogin := make(map[string]*StreamRecord, len(initial))
	for login, rec := range initial {
		cp := *rec
		byLogin[normalizeLogin(login)] = &cp
	}
	return &reconcilePulseStore{byLogin: byLogin}
}

func (s *reconcilePulseStore) LatestStreamByLogin(_ context.Context, login string) (*StreamRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.byLogin[normalizeLogin(login)]; ok {
		cp := *rec
		return &cp, nil
	}
	return nil, nil
}

func (s *reconcilePulseStore) CloseStream(_ context.Context, streamID string, endedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = append(s.closed, streamID)
	for login, rec := range s.byLogin {
		if rec.StreamID == streamID {
			rec.EndedAt = &endedAt
			s.byLogin[login] = rec
		}
	}
	return nil
}

func (s *reconcilePulseStore) UpsertLiveStream(_ context.Context, live LiveStream, profile UserProfile, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, live)
	rec := &StreamRecord{
		StreamID:      live.ID,
		Login:         normalizeLogin(live.Login),
		BroadcasterID: live.BroadcasterID,
		DisplayName:   profile.DisplayName,
		Title:         live.Title,
		Category:      live.GameName,
		StartedAt:     live.StartedAt,
		LastSeenAt:    now,
		PeakViewers:   live.ViewerCount,
	}
	s.byLogin[normalizeLogin(live.Login)] = rec
	return nil
}

func testHelixServer(t *testing.T, streams map[string]LiveStream) *HelixClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/streams":
			logins := r.URL.Query()["user_login"]
			data := make([]map[string]any, 0, len(logins))
			for _, login := range logins {
				s, ok := streams[normalizeLogin(login)]
				if !ok {
					continue
				}
				data = append(data, map[string]any{
					"id":           s.ID,
					"user_id":      s.BroadcasterID,
					"user_login":   s.Login,
					"user_name":    s.Login,
					"game_name":    s.GameName,
					"title":        s.Title,
					"viewer_count": s.ViewerCount,
					"started_at":   s.StartedAt.UTC().Format(time.RFC3339),
					"language":     "en",
					"tags":         []string{},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "pagination": map[string]any{}})
		case "/users":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}, "pagination": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewHelixClient(srv.URL, srv.URL+"/oauth2/token", "cid", "secret", "test")
}

func TestReconcileExtensionLiveStreamUntrackedClosesStaleOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	started := time.Date(2026, 6, 30, 16, 47, 54, 0, time.UTC)
	store := newReconcilePulseStore(map[string]*StreamRecord{
		"shroud": {
			StreamID:   "320060756185",
			Login:      "shroud",
			StartedAt:  started,
			LastSeenAt: started,
		},
	})
	handler := &Handler{
		helix:        testHelixServer(t, map[string]LiveStream{}),
		pulseRuntime: PulseRuntimeConfig{Configured: true, HelixLiveEnabled: true},
	}
	stream := &StreamRecord{
		StreamID:  "320060756185",
		Login:     "shroud",
		StartedAt: started,
	}
	gotStream, isLive, err := handler.reconcileExtensionLiveStreamWithStore(ctx, "shroud", stream, store)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if isLive {
		t.Fatal("expected Helix offline to mark stream not live")
	}
	if gotStream == nil || gotStream.EndedAt == nil {
		t.Fatalf("expected closed stream, got %+v", gotStream)
	}
	if len(store.closed) != 1 || store.closed[0] != "320060756185" {
		t.Fatalf("closed = %#v", store.closed)
	}
}

func TestReconcileExtensionLiveStreamUntrackedNewStreamID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	oldStart := time.Date(2026, 6, 30, 16, 47, 54, 0, time.UTC)
	newStart := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	store := newReconcilePulseStore(map[string]*StreamRecord{
		"shroud": {
			StreamID:  "320060756185",
			Login:     "shroud",
			StartedAt: oldStart,
		},
	})
	handler := &Handler{
		helix: testHelixServer(t, map[string]LiveStream{
			"shroud": {
				ID:            "999999999999",
				Login:         "shroud",
				BroadcasterID: "37402112",
				Title:         "live now",
				GameName:      "Game",
				StartedAt:     newStart,
				ViewerCount:   12000,
			},
		}),
		pulseRuntime: PulseRuntimeConfig{Configured: true, HelixLiveEnabled: true},
	}
	stream := &StreamRecord{StreamID: "320060756185", Login: "shroud", StartedAt: oldStart}
	gotStream, isLive, err := handler.reconcileExtensionLiveStreamWithStore(ctx, "shroud", stream, store)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !isLive {
		t.Fatal("expected live after Helix reports new broadcast")
	}
	if gotStream == nil || gotStream.StreamID != "999999999999" {
		t.Fatalf("stream = %+v", gotStream)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("upserted = %#v", store.upserted)
	}
}

func TestReconcileExtensionLiveStreamTrackedMatchingStream(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	started := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	store := newReconcilePulseStore(map[string]*StreamRecord{
		"xqc": {StreamID: "111", Login: "xqc", StartedAt: started},
	})
	handler := &Handler{
		helix: testHelixServer(t, map[string]LiveStream{
			"xqc": {ID: "111", Login: "xqc", BroadcasterID: "1", StartedAt: started, ViewerCount: 50000},
		}),
		pulseRuntime: PulseRuntimeConfig{Configured: true, HelixLiveEnabled: true},
	}
	stream := &StreamRecord{StreamID: "111", Login: "xqc", StartedAt: started}
	gotStream, isLive, err := handler.reconcileExtensionLiveStreamWithStore(ctx, "xqc", stream, store)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !isLive || gotStream.StreamID != "111" {
		t.Fatalf("got stream=%+v isLive=%v", gotStream, isLive)
	}
	if len(store.closed) != 0 || len(store.upserted) != 0 {
		t.Fatalf("expected no store mutations, closed=%v upserted=%v", store.closed, store.upserted)
	}
}

func TestSanitizeExtensionPulseForCollectorTruth(t *testing.T) {
	t.Parallel()
	payload := ExtensionPulseResponse{
		Tracking: false,
		IsLive:   true,
		Rollups:  []ExtensionRollup{{OffsetSeconds: 60, ChatCount: 10}},
		Peaks:    []ExtensionPeak{{OffsetSeconds: 60, Score: 90}},
		Lanes:    ExtensionLanes{Composite: []int{1, 2}},
		Coverage: ExtensionCoverage{State: CoverageStateMissingRanges, CanBackfill: true},
	}
	sanitizeExtensionPulseForCollectorTruth(&payload)
	if len(payload.Rollups) != 0 || len(payload.Peaks) != 0 || len(payload.Lanes.Composite) != 0 {
		t.Fatalf("expected empty live artifacts, got rollups=%d peaks=%d lanes=%d", len(payload.Rollups), len(payload.Peaks), len(payload.Lanes.Composite))
	}
	if payload.Coverage.CanBackfill {
		t.Fatal("expected canBackfill=false for not tracking")
	}
	if payload.Coverage.CopyKey != "not_tracking" {
		t.Fatalf("copyKey = %q", payload.Coverage.CopyKey)
	}
}

func TestSanitizeExtensionPulseForNonTop500(t *testing.T) {
	t.Parallel()
	payload := ExtensionPulseResponse{
		RosterEligible: false,
		Top500Eligible: false,
		Tracking:       true,
		IsLive:         true,
		Rollups:        []ExtensionRollup{{OffsetSeconds: 60, ChatCount: 10}},
		Peaks:          []ExtensionPeak{{OffsetSeconds: 60, Score: 90}},
		Recap:          map[string]any{"title": "old"},
	}
	sanitizeExtensionPulseForNonTop500(&payload)
	if payload.Tracking {
		t.Fatal("expected tracking=false outside top-500")
	}
	if len(payload.Rollups) != 0 || payload.Recap != nil {
		t.Fatalf("expected stripped artifacts, rollups=%d recap=%v", len(payload.Rollups), payload.Recap)
	}
	if payload.Coverage.CopyKey != "top500_required" {
		t.Fatalf("copyKey = %q", payload.Coverage.CopyKey)
	}
}

func TestSanitizeExtensionPulseSkipsWhenTracking(t *testing.T) {
	t.Parallel()
	payload := ExtensionPulseResponse{
		Tracking: true,
		IsLive:   true,
		Rollups:  []ExtensionRollup{{OffsetSeconds: 60, ChatCount: 10}},
	}
	sanitizeExtensionPulseForCollectorTruth(&payload)
	if len(payload.Rollups) != 1 {
		t.Fatal("tracked payload must not be sanitized")
	}
}

func TestExtensionPulseRosterEligibleDualEmit(t *testing.T) {
	t.Parallel()
	for _, eligible := range []bool{true, false} {
		payload := emptyExtensionPulse("testchan", false, eligible)
		if payload.RosterEligible != eligible {
			t.Fatalf("RosterEligible = %v, want %v", payload.RosterEligible, eligible)
		}
		if payload.Top500Eligible != eligible {
			t.Fatalf("Top500Eligible = %v, want %v (dual-emit)", payload.Top500Eligible, eligible)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["rosterEligible"] != eligible {
			t.Fatalf("json rosterEligible = %#v", decoded["rosterEligible"])
		}
		if decoded["top500Eligible"] != eligible {
			t.Fatalf("json top500Eligible = %#v", decoded["top500Eligible"])
		}
	}
}
