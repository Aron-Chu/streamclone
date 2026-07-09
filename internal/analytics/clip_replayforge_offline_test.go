package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

// Integration tests in this file that call setupClipCandidateStore require
// INTEGRATION=1 and a running Postgres instance. Unit tests that exercise
// the HTTP client and redaction chokepoint run without external dependencies.

// ---------------------------------------------------------------------------
// RF-P7-012: Validate ReplayForge-offline behavior from Streamclone
//
// These tests confirm that when ReplayForge is down (connection refused,
// timeout, or simply unconfigured), Streamclone's core features remain
// usable with honest degraded states — no stack traces, no fabricated
// mirrored state, and no leaked topology/secrets.
//
// Requirement 1.5 (streamclone-pulse independence) is validated transitively:
// if core extension health and routes stay up when ReplayForge is absent,
// then no optional surface hard-depends on it.
// ---------------------------------------------------------------------------

// TestExtensionHealthStaysHealthyWithReplayForgeDown confirms that
// /v1/extension/health continues to return 200 OK even when ReplayForge is
// unreachable. This validates Requirement 1.5: Streamclone (and by extension
// StreamPulse) operates without requiring ReplayForge.
func TestExtensionHealthStaysHealthyWithReplayForgeDown(t *testing.T) {
	// Set up an httptest server and immediately close it to simulate
	// ReplayForge being unreachable (connection refused).
	rf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rfURL := rf.URL
	rf.Close() // now unreachable

	// Build a handler with a ReplayForge client pointing at the dead server.
	client := NewReplayForgeHTTPClient(rfURL, "test-token")
	h := &Handler{replayForge: client}
	h = h.WithPulseRuntime(DefaultPulseRuntimeConfig())

	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("extension health status = %d, want 200 with ReplayForge down", rec.Code)
	}
	var body ExtensionHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if !body.OK {
		t.Fatal("expected ok=true with ReplayForge down")
	}
}

// TestExtensionHealthStaysHealthyWithReplayForgeUnconfigured confirms that
// /v1/extension/health returns 200 OK when no ReplayForge client is configured
// at all (nil client). This is the default state for users who haven't set up
// Clip Studio.
func TestExtensionHealthStaysHealthyWithReplayForgeUnconfigured(t *testing.T) {
	h := &Handler{replayForge: nil}
	h = h.WithPulseRuntime(DefaultPulseRuntimeConfig())

	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("extension health status = %d, want 200 with no ReplayForge", rec.Code)
	}
	var body ExtensionHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if !body.OK {
		t.Fatal("expected ok=true with nil ReplayForge client")
	}
}

// TestClipTriggerReturns502WithHonestErrorWhenReplayForgeDown confirms that
// the Export Moment trigger (POST /v1/pulse/clips/{id}/replayforge) returns
// 502 (bad gateway) with an honest error code — NOT 500, NOT a stack trace —
// when ReplayForge is unreachable.
func TestClipTriggerReturns502WithHonestErrorWhenReplayForgeDown(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-offline-val-1', 'xqc', $1, 'Offline test', 'Just Chatting')
	`, startedAt)
	vodID := "vod-offline-val-1"
	candidate := ClipCandidate{
		ID:            "cc_offline_val_1",
		Login:         "xqc",
		StreamID:      "stream-offline-val-1",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}

	// Simulate a dead ReplayForge: start a server and immediately shut it down.
	rf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rfURL := rf.URL
	rf.Close()

	client := NewReplayForgeHTTPClient(rfURL, "test-token")
	h := &Handler{store: store, replayForge: client}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_offline_val_1/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("clip trigger status = %d, want 502: %s", rec.Code, rec.Body.String())
	}

	// Verify it's an honest error, not a stack trace.
	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] != "replayforge_unavailable" {
		t.Fatalf("error = %q, want replayforge_unavailable", errResp["error"])
	}
	// The response should NOT contain raw connection error topology.
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "dial tcp") || strings.Contains(bodyStr, "connection refused") {
		t.Fatalf("response leaked raw connection error: %s", bodyStr)
	}
}

// TestClipTriggerReturns503WhenReplayForgeUnconfigured confirms that the
// trigger route returns 503 with "replayforge_unconfigured" when no client
// is set up at all.
func TestClipTriggerReturns503WhenReplayForgeUnconfigured(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-offline-val-2', 'xqc', $1, 'Unconfig test', 'Just Chatting')
	`, startedAt)
	vodID := "vod-offline-val-2"
	candidate := ClipCandidate{
		ID:            "cc_offline_val_2",
		Login:         "xqc",
		StreamID:      "stream-offline-val-2",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}

	h := &Handler{store: store, replayForge: nil}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_offline_val_2/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("clip trigger status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] != "replayforge_unconfigured" {
		t.Fatalf("error = %q, want replayforge_unconfigured", errResp["error"])
	}
}

// TestClipJobRefreshReturns502WhenReplayForgeDown confirms that polling a
// mirrored job status (GET /v1/pulse/clips/{id}/replayforge) returns 502
// with an honest error when ReplayForge is unreachable — the mirror shows
// last known state with the error flagged.
func TestClipJobRefreshReturns502WhenReplayForgeDown(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-offline-val-3', 'xqc', $1, 'Refresh offline', 'Just Chatting')
	`, startedAt)
	vodID := "vod-offline-val-3"
	candidate := ClipCandidate{
		ID:            "cc_offline_val_3",
		Login:         "xqc",
		StreamID:      "stream-offline-val-3",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	// Pre-seed a mirrored job (as if a previous successful trigger happened).
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "local"),
		CandidateID:      candidate.ID,
		PrincipalID:      "local",
		PrincipalKind:    "guest",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_offline_val_3",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_offline_val_3"},
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}

	// Simulate dead ReplayForge.
	rf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rfURL := rf.URL
	rf.Close()

	client := NewReplayForgeHTTPClient(rfURL, "test-token")
	h := &Handler{store: store, replayForge: client}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips/cc_offline_val_3/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("refresh status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] != "replayforge_unavailable" {
		t.Fatalf("error = %q, want replayforge_unavailable", errResp["error"])
	}

	// Verify the mirrored job was updated with the error code (stale marker).
	got, err := store.GetClipCandidateJob(ctx, candidate.ID, "local")
	if err != nil {
		t.Fatalf("get stored job: %v", err)
	}
	if got.ErrorCode != "replayforge_unavailable" {
		t.Fatalf("stored job errorCode = %q, want replayforge_unavailable", got.ErrorCode)
	}
	if got.LastCheckedAt == nil {
		t.Fatal("expected lastCheckedAt to be set (stale marker)")
	}
	// The raw connection error should be redacted when sanitized for display.
	sanitized := sanitizeReplayForgeStatusText(got.ErrorMessage)
	if strings.Contains(sanitized, "://") || strings.Contains(sanitized, "dial tcp") {
		t.Fatalf("stored error message leaked topology: %q (sanitized: %q)", got.ErrorMessage, sanitized)
	}
}

// TestReplayForgeWebhookRejectsWithoutCallbackTokenHonestly verifies that
// when ReplayForge is conceptually "down" (or attacker), the callback webhook
// rejects unauthenticated requests with 401 — not 500 — and never mutates
// the mirror.
func TestReplayForgeWebhookRejectsWithoutCallbackTokenHonestly(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-offline-val-4', 'xqc', $1, 'Webhook auth', 'Just Chatting')
	`, startedAt)
	vodID := "vod-offline-val-4"
	candidate := ClipCandidate{
		ID:            "cc_offline_val_4",
		Login:         "xqc",
		StreamID:      "stream-offline-val-4",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "principal-w"),
		CandidateID:      candidate.ID,
		PrincipalID:      "principal-w",
		PrincipalKind:    "beta",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_webhook_val_4",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_webhook_val_4"},
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}

	h := &Handler{
		store: store,
		appConfig: config.Config{
			ReplayForgeCallbackToken: "real-secret",
		},
		pulseHosted: PulseHostedConfig{Hosted: true},
	}
	r := chi.NewRouter()
	h.registerReplayForgeWebhookRoutes(r)

	// No auth header → 401.
	body := `{"job":{"id":"rf_webhook_val_4","state":"ready","artifact_available":1}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_val_4", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d, want 401: %s", rec.Code, rec.Body.String())
	}

	// Wrong token → 401.
	wrongReq := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_val_4", strings.NewReader(body))
	wrongReq.Header.Set("Authorization", "Bearer wrong-secret")
	wrongRec := httptest.NewRecorder()
	r.ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-auth status = %d, want 401: %s", wrongRec.Code, wrongRec.Body.String())
	}

	// Mirror should be unchanged.
	got, err := store.GetClipCandidateJob(ctx, candidate.ID, "principal-w")
	if err != nil {
		t.Fatalf("get job after unauthenticated attempts: %v", err)
	}
	if got.Status != ClipCandidateJobQueued || got.ReplayForgeState != "queued" {
		t.Fatalf("mirror was mutated by unauthenticated request: %+v", got)
	}
}

// TestSanitizeReplayForgeStatusTextRedactsSecretsAndTopology covers the
// offline/degraded error surface: every ReplayForge error string persisted
// on a Clip_Job and later serialized to a client must be redacted of
// connection topology and secrets.
func TestSanitizeReplayForgeStatusTextRedactsSecretsAndTopology(t *testing.T) {
	cases := []struct {
		name string
		in   string
		leak string // non-empty means: assert this substring is absent
		want string // used only when leak == ""
	}{
		{
			name: "connection refused url collapses to redacted",
			in:   `Post "http://host.docker.internal:8095/v1/triggers/manual": dial tcp 127.0.0.1:8095: connect: connection refused`,
			want: "redacted",
		},
		{
			name: "filesystem path collapses to redacted",
			in:   "/tmp/clips/raw.mp4 not found",
			want: "redacted",
		},
		{
			name: "windows path collapses to redacted",
			in:   `C:\Users\clips\final.mp4`,
			want: "redacted",
		},
		{
			name: "benign state preserved",
			in:   "queued",
			want: "queued",
		},
		{
			name: "http error code preserved",
			in:   "replayforge_http_502",
			want: "replayforge_http_502",
		},
		{
			name: "empty string preserved",
			in:   "",
			want: "",
		},
		{
			name: "bearer token stripped",
			in:   "replayforge rejected: bearer abcdefghijklmnopqrstuvwxyz012345",
			leak: "abcdefghijklmnopqrstuvwxyz012345",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeReplayForgeStatusText(tc.in)
			if tc.leak != "" {
				if strings.Contains(got, tc.leak) {
					t.Fatalf("sanitized output leaked %q: %q", tc.leak, got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("sanitizeReplayForgeStatusText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTriggerManualUnreachableReplayForgeErrorIsRedactable exercises the
// ReplayForge HTTP client against an unreachable endpoint and confirms the
// returned error once routed through the status-text chokepoint leaks
// neither the endpoint URL nor any token.
func TestTriggerManualUnreachableReplayForgeErrorIsRedactable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	baseURL := srv.URL
	srv.Close() // make the endpoint unreachable

	client := NewReplayForgeHTTPClient(baseURL, "super-secret-auth-token-value-1234567890")
	_, err := client.TriggerManual(context.Background(), ReplayForgeTriggerRequest{
		Channel: "xqc",
		Title:   "Good bit",
	})
	if err == nil {
		t.Fatal("expected TriggerManual to fail against an unreachable ReplayForge")
	}
	sanitized := sanitizeReplayForgeStatusText(err.Error())
	if strings.Contains(sanitized, "super-secret-auth-token") {
		t.Fatalf("sanitized error leaked auth token: %q", sanitized)
	}
	if strings.Contains(sanitized, "://") {
		t.Fatalf("sanitized error leaked endpoint URL: %q", sanitized)
	}
}

// TestGetJobUnreachableReplayForgeErrorIsRedactable exercises the GetJob path
// (reconciliation/refresh) against an unreachable endpoint.
func TestGetJobUnreachableReplayForgeErrorIsRedactable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	baseURL := srv.URL
	srv.Close()

	client := NewReplayForgeHTTPClient(baseURL, "another-secret-token-999")
	_, err := client.GetJob(context.Background(), "rf_test_123")
	if err == nil {
		t.Fatal("expected GetJob to fail against an unreachable ReplayForge")
	}
	sanitized := sanitizeReplayForgeStatusText(err.Error())
	if strings.Contains(sanitized, "another-secret-token") {
		t.Fatalf("sanitized error leaked auth token: %q", sanitized)
	}
	if strings.Contains(sanitized, "://") {
		t.Fatalf("sanitized error leaked endpoint URL: %q", sanitized)
	}
}

// TestClipCandidateJobForFailureDoesNotFabricateMirroredState proves that
// when the Export Moment trigger cannot reach ReplayForge, the failure record
// carries NO ReplayForge job id or mirrored state — it's an honest local
// failure with a redacted error message.
func TestClipCandidateJobForFailureDoesNotFabricateMirroredState(t *testing.T) {
	vodID := "v123456789"
	candidate := ClipCandidate{
		ID:           "cc_offline_fab_1",
		Login:        "xqc",
		StreamID:     "stream-offline-fab-1",
		VodID:        &vodID,
		StartSeconds: 100,
		EndSeconds:   160,
		Reason:       "chat_spike",
		SourceStatus: ClipCandidateSourceAvailable,
	}
	principal := PulsePrincipal{ID: "principal-a", Kind: "beta"}
	state := ClipCandidateState{}

	connErr := `Post "http://host.docker.internal:8095/v1/triggers/manual": dial tcp 127.0.0.1:8095: connect: connection refused`

	h := &Handler{}
	job := h.clipCandidateJobForFailure(candidate, principal, state, ClipCandidateJobFailed, "replayforge_unavailable", connErr)

	if job.Status != ClipCandidateJobFailed {
		t.Fatalf("status = %q, want %q", job.Status, ClipCandidateJobFailed)
	}
	if job.ErrorCode != "replayforge_unavailable" {
		t.Fatalf("errorCode = %q, want replayforge_unavailable", job.ErrorCode)
	}
	if job.ReplayForgeJobID != "" {
		t.Fatalf("fabricated mirrored job id %q for a down upstream", job.ReplayForgeJobID)
	}
	if job.ReplayForgeState != "" {
		t.Fatalf("fabricated mirrored state %q for a down upstream", job.ReplayForgeState)
	}
	// The message is redacted by the store chokepoint before display.
	if got := sanitizeReplayForgeStatusText(job.ErrorMessage); got != "redacted" {
		t.Fatalf("persisted error message = %q, want %q (connection error must not leak topology)", got, "redacted")
	}
}
