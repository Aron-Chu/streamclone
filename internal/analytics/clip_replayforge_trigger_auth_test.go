package analytics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// TestPulseClipReplayForgeTriggerRejectsUnauthenticatedWithoutForwarding proves
// the Export Moment trigger (POST /v1/pulse/clips/{id}/replayforge) is an
// authenticated surface: in hosted mode an unauthenticated caller is rejected
// with 401 and ReplayForge is never contacted, so no Auth_Token can be
// forwarded on behalf of an anonymous request.
//
// Feature: auto-clipper-replayforge-productization, Property 14: Moment trigger creates a job carrying its context and records the id
// **Validates: Requirement 6.1**
func TestPulseClipReplayForgeTriggerRejectsUnauthenticatedWithoutForwarding(t *testing.T) {
	fake := &fakeReplayForgeClient{}
	h := &Handler{
		replayForge: fake,
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	// No X-Streamclone-Beta-Key / Authorization → guest principal → rejected.
	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_secure/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated trigger status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("ReplayForge contacted %d times for an unauthenticated trigger; the Auth_Token must never be forwarded for an anonymous caller", fake.calls)
	}
}

// TestPulseClipReplayForgeTriggerAuthenticatedScopesJobAndForwardsBearerIntegration
// exercises the full authenticated trigger path against the REAL
// ReplayForgeHTTPClient (not the fake), asserting that:
//
//	(a) an authenticated hosted caller (beta key) is accepted (202);
//	(b) the created Clip_Job is scoped to that caller's principal id
//	    (requested_by = principal.ID, derived server-side, never client-supplied);
//	(c) the ReplayForge create request carries the Auth_Token only in the
//	    Authorization: Bearer header; and
//	(d) the outbound moment_context body contains no token/secret shapes.
//
// Feature: auto-clipper-replayforge-productization, Property 14: Moment trigger creates a job carrying its context and records the id
// **Validates: Requirements 6.1, 6.2**
func TestPulseClipReplayForgeTriggerAuthenticatedScopesJobAndForwardsBearerIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-auth-1', 'xqc', $1, 'Auth stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-auth-1"
	candidate := ClipCandidate{
		ID:            "cc_auth_scope",
		Login:         "xqc",
		StreamID:      "stream-rf-auth-1",
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

	// Capture what the real HTTP client sends upstream to ReplayForge.
	const authToken = "rf-secret-token"
	var gotAuth string
	var gotRawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		gotRawBody, _ = io.ReadAll(req.Body)
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "job_id": "rf_auth_1"})
	}))
	defer srv.Close()

	const betaKey = "secret-one"
	h := &Handler{
		store:       store,
		replayForge: NewReplayForgeHTTPClient(srv.URL, authToken),
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{betaKey}},
	}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_auth_scope/replayforge", nil)
	req.Header.Set(pulseBetaKeyHeader, betaKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("authenticated trigger status = %d, want 202: %s", rec.Code, rec.Body.String())
	}

	// (c) Auth_Token forwarded only via the Authorization header.
	if gotAuth != "Bearer "+authToken {
		t.Fatalf("outbound Authorization = %q, want bearer token", gotAuth)
	}

	// (d) The outbound moment_context body must never carry a token/secret shape.
	if len(gotRawBody) == 0 {
		t.Fatal("ReplayForge received an empty request body")
	}
	lowerBody := strings.ToLower(string(gotRawBody))
	for _, marker := range []string{authToken, "authorization", "bearer", "auth_token", "access_token", "refresh_token", "token=", "requested_by"} {
		if strings.Contains(lowerBody, strings.ToLower(marker)) {
			t.Fatalf("outbound moment_context body leaked secret/ownership marker %q: %s", marker, gotRawBody)
		}
	}

	// (b) The created job is scoped to the authenticated caller's principal id.
	principalID := hashPulseBetaKey(betaKey)
	job, err := store.GetClipCandidateJob(ctx, candidate.ID, principalID)
	if err != nil {
		t.Fatalf("get job for authenticated principal: %v", err)
	}
	if job.PrincipalID != principalID {
		t.Fatalf("job.PrincipalID = %q, want caller principal %q", job.PrincipalID, principalID)
	}
	if job.ID != newClipCandidateJobID(candidate.ID, principalID) {
		t.Fatalf("job.ID = %q, want deterministic id scoped to caller", job.ID)
	}
	if job.Status != ClipCandidateJobQueued || job.ReplayForgeJobID != "rf_auth_1" {
		t.Fatalf("job = %+v, want queued rf_auth_1", job)
	}

	// A different principal must not see the caller's job (ownership isolation).
	if other, err := store.GetClipCandidateJob(ctx, candidate.ID, "someone-else"); err == nil && other.ReplayForgeJobID != "" {
		t.Fatalf("job leaked across principals: %+v", other)
	}

	// (b, cont.) The API response body never exposes principal identifiers.
	assertJSONOmitsForbiddenFields(t, rec.Body.Bytes(), []string{
		"principalId", "principalKind", authToken, "access_token", "token=secret",
	})
}

// TestPulseClipReplayForgeTriggerRecordsExistingJobIDInMirrorIntegration covers
// the duplicate-suppression path: when ReplayForge already has an active job for
// the source it responds with only `existing_job_id` (no `job_id`, per
// Requirement 2.10). Streamclone must still record that returned Clip_Job id in
// the Job_Mirror for the owning principal (Requirement 6.2), and the mirrored id
// must be the ReplayForge-returned identifier — not empty — and must stay scoped
// to the caller's principal.
//
// Feature: auto-clipper-replayforge-productization, Property 14: Moment trigger creates a job carrying its context and records the id
// **Validates: Requirement 6.2**
func TestPulseClipReplayForgeTriggerRecordsExistingJobIDInMirrorIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 30, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-dup-1', 'xqc', $1, 'Dup stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-dup-1"
	candidate := ClipCandidate{
		ID:            "cc_dup_scope",
		Login:         "xqc",
		StreamID:      "stream-rf-dup-1",
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

	// ReplayForge dedups: returns the existing job id only (no job_id field).
	const existingJobID = "rf_existing_9"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate", "existing_job_id": existingJobID})
	}))
	defer srv.Close()

	const betaKey = "secret-one"
	h := &Handler{
		store:       store,
		replayForge: NewReplayForgeHTTPClient(srv.URL, "rf-secret-token"),
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{betaKey}},
	}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_dup_scope/replayforge", nil)
	req.Header.Set(pulseBetaKeyHeader, betaKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("dedup trigger status = %d, want 202: %s", rec.Code, rec.Body.String())
	}

	// The returned (existing) Clip_Job id is recorded in the mirror for the owner.
	principalID := hashPulseBetaKey(betaKey)
	job, err := store.GetClipCandidateJob(ctx, candidate.ID, principalID)
	if err != nil {
		t.Fatalf("get job for owning principal: %v", err)
	}
	if job.ReplayForgeJobID != existingJobID {
		t.Fatalf("job.ReplayForgeJobID = %q, want returned existing id %q", job.ReplayForgeJobID, existingJobID)
	}
	if job.PrincipalID != principalID {
		t.Fatalf("job.PrincipalID = %q, want caller principal %q", job.PrincipalID, principalID)
	}

	// A different principal must not see the caller's mirrored job.
	if other, err := store.GetClipCandidateJob(ctx, candidate.ID, "someone-else"); err == nil && other.ReplayForgeJobID != "" {
		t.Fatalf("mirrored job leaked across principals: %+v", other)
	}
}
