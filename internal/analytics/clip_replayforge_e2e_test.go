package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// TestAnalyticsExportMomentToReplayForgeJobCreationE2E validates the full
// private-beta e2e path:
//
//	Analytics moment → Export Moment trigger → authenticated POST /v1/jobs →
//	ReplayForge accepts → job_id returned → stored in Job_Mirror.
//
// This is the Phase 7 (RF-P7-002) validation: for an invite account, the
// moment trigger creates a job and mirrors it locally.
//
// Requirements: 6.1, 8.6
func TestAnalyticsExportMomentToReplayForgeJobCreationE2E(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)

	// --- Setup: insert a stream and a candidate (the "moment") ---
	startedAt := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-e2e-rf-1', 'invite_streamer', $1, 'E2E Validation', 'Just Chatting')
	`, startedAt)
	vodID := "vod-e2e-rf-1"
	candidate := ClipCandidate{
		ID:             "cc_e2e_invite",
		Login:          "invite_streamer",
		StreamID:       "stream-e2e-rf-1",
		VodID:          &vodID,
		StreamTitle:    "E2E Validation",
		StreamCategory: "Just Chatting",
		OffsetSeconds:  3600,
		StartSeconds:   3600,
		EndSeconds:     3660,
		Score:          88,
		Confidence:     0.91,
		Reason:         "chat_spike",
		ChatCount:      200,
		EmoteCount:     150,
		ViewerCount:    25000,
		SourceKind:     ClipCandidateSourceRecap,
		SourceStatus:   ClipCandidateSourceAvailable,
		TopEmotes: []ClipCandidateEmote{{
			Provider: "seventv",
			ID:       "7tv-e2e",
			Name:     "Pog",
			Count:    42,
			ImageURL: "https://cdn.example/pog.webp",
		}},
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}

	// --- Mock ReplayForge server ---
	// Captures the request to verify auth token and moment_context payload shape.
	var (
		mu              sync.Mutex
		gotAuth         string
		gotContentType  string
		gotBody         map[string]interface{}
		gotPath         string
		replayForgeHits int
	)
	mockReplayForge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		replayForgeHits++
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		// Simulate ReplayForge accepting the job for an invite account
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "queued",
			"job_id": "rf_e2e_invite_001",
		})
	}))
	defer mockReplayForge.Close()

	// --- Wire up the handler with real HTTP client pointed at mock ---
	authToken := "e2e-beta-auth-token"
	realClient := NewReplayForgeHTTPClient(mockReplayForge.URL, authToken)
	h := &Handler{
		store:       store,
		replayForge: realClient,
	}
	router := chi.NewRouter()
	h.PulseRoutes(router)

	// --- Execute: POST to Export Moment endpoint (invite account) ---
	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_e2e_invite/replayforge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// --- Verify: HTTP response ---
	if rec.Code != http.StatusAccepted {
		t.Fatalf("Export Moment response status = %d, want 202 Accepted: %s", rec.Code, rec.Body.String())
	}

	// --- Verify: Auth token was sent in the request to ReplayForge ---
	mu.Lock()
	defer mu.Unlock()

	if replayForgeHits != 1 {
		t.Fatalf("ReplayForge was called %d times, want exactly 1", replayForgeHits)
	}
	if gotAuth != "Bearer "+authToken {
		t.Fatalf("Auth header = %q, want Bearer token included", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotPath != "/v1/triggers/manual" {
		t.Fatalf("ReplayForge endpoint path = %q, want /v1/triggers/manual", gotPath)
	}

	// --- Verify: moment_context payload shape ---
	momentCtx, ok := gotBody["moment_context"].(map[string]interface{})
	if !ok {
		t.Fatalf("moment_context missing or wrong type in request body: %#v", gotBody)
	}
	// Required fields per spec: channel_login (via Channel), vod_id, start/end, reason, candidate_score
	if gotBody["channel"] != "invite_streamer" {
		t.Fatalf("channel = %v, want invite_streamer", gotBody["channel"])
	}
	if momentCtx["vod_id"] != "vod-e2e-rf-1" {
		t.Fatalf("moment_context.vod_id = %v, want vod-e2e-rf-1", momentCtx["vod_id"])
	}
	if momentCtx["clip_start_seconds"] != float64(3600) {
		t.Fatalf("moment_context.clip_start_seconds = %v, want 3600", momentCtx["clip_start_seconds"])
	}
	if momentCtx["clip_end_seconds"] != float64(3660) {
		t.Fatalf("moment_context.clip_end_seconds = %v, want 3660", momentCtx["clip_end_seconds"])
	}
	if momentCtx["pick_reason"] != "chat_spike" {
		t.Fatalf("moment_context.pick_reason = %v, want chat_spike", momentCtx["pick_reason"])
	}
	if momentCtx["moment_score"] != float64(88) {
		t.Fatalf("moment_context.moment_score = %v, want 88 (server-side score)", momentCtx["moment_score"])
	}
	if momentCtx["candidate_id"] != "cc_e2e_invite" {
		t.Fatalf("moment_context.candidate_id = %v, want cc_e2e_invite", momentCtx["candidate_id"])
	}

	// Verify no tokens leaked in the request payload
	bodyBytes, _ := json.Marshal(gotBody)
	bodyStr := strings.ToLower(string(bodyBytes))
	for _, forbidden := range []string{"bearer", "auth_token", "access_token", "refresh_token", "e2e-beta-auth-token"} {
		if strings.Contains(bodyStr, forbidden) {
			t.Fatalf("request body leaked forbidden value %q: %s", forbidden, bodyStr)
		}
	}

	// --- Verify: Job_Mirror was created in the store ---
	var response ClipCandidateJob
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if response.ReplayForgeJobID != "rf_e2e_invite_001" {
		t.Fatalf("response.ReplayForgeJobID = %q, want rf_e2e_invite_001", response.ReplayForgeJobID)
	}
	if response.Status != ClipCandidateJobQueued {
		t.Fatalf("response.Status = %q, want queued", response.Status)
	}

	// Verify the job is persisted in the store (Job_Mirror)
	stored, err := store.GetClipCandidateJob(ctx, "cc_e2e_invite", "local")
	if err != nil {
		t.Fatalf("get stored job from mirror: %v", err)
	}
	if stored.ReplayForgeJobID != "rf_e2e_invite_001" {
		t.Fatalf("stored Job_Mirror.ReplayForgeJobID = %q, want rf_e2e_invite_001", stored.ReplayForgeJobID)
	}
	if stored.Status != ClipCandidateJobQueued {
		t.Fatalf("stored Job_Mirror.Status = %q, want queued", stored.Status)
	}
	if stored.ReplayForgeState != "queued" {
		t.Fatalf("stored Job_Mirror.ReplayForgeState = %q, want queued", stored.ReplayForgeState)
	}
	if stored.SubmittedAt == nil {
		t.Fatal("stored Job_Mirror.SubmittedAt was nil, want non-nil timestamp")
	}
}
