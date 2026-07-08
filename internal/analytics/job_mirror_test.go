package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
	"streamclone/internal/jobstate"
)

const testCallbackToken = "callback-secret"

func newJobMirrorTestHandler() (*Handler, http.Handler) {
	h := &Handler{
		appConfig: config.Config{ReplayForgeCallbackToken: testCallbackToken},
	}
	r := chi.NewRouter()
	h.registerJobMirrorCallbackRoutes(r)
	return h, r
}

// newReplayForgeWebhookTestHandler builds a handler whose legacy ReplayForge
// job webhook (POST /v1/internal/replayforge/jobs/{jobID}) is configured with a
// callback token but no store. Auth is enforced before any store access, so an
// unauthenticated request is rejected with 401 and can never reach a mutation
// (used by the P4 property test to prove "no side effects").
func newReplayForgeWebhookTestHandler() (*Handler, http.Handler) {
	h := &Handler{
		appConfig: config.Config{ReplayForgeCallbackToken: testCallbackToken},
	}
	r := chi.NewRouter()
	h.registerReplayForgeWebhookRoutes(r)
	return h, r
}

// postWebhook posts a body to the legacy ReplayForge job webhook for jobID,
// optionally attaching a bearer token.
func postWebhook(t *testing.T, router http.Handler, token, jobID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/"+jobID, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// mirrorSnapshot returns a deterministic JSON snapshot of every entry currently
// held by the Job_Mirror. encoding/json sorts map keys, so equal mirror
// contents always produce byte-for-byte identical output — the P4 property test
// compares this before/after unauthenticated traffic.
func mirrorSnapshot(m *JobMirror) string {
	if m == nil {
		return "null"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, err := json.Marshal(m.entries)
	if err != nil {
		return "error:" + err.Error()
	}
	return string(b)
}

func postCallback(t *testing.T, router http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, jobMirrorCallbackPath, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeCallbackResponse(t *testing.T, rec *httptest.ResponseRecorder) jobMirrorCallbackResponse {
	t.Helper()
	var resp jobMirrorCallbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	return resp
}

func TestJobMirrorCallbackAppliesInSetState(t *testing.T) {
	h, router := newJobMirrorTestHandler()
	rec := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_1","state":"rendering","seq":3,"updated_at":"2026-07-01T12:00:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	resp := decodeCallbackResponse(t, rec)
	if !resp.Applied {
		t.Fatal("expected callback to be applied")
	}
	entry, ok := h.jobMirror().Get("job_1")
	if !ok || entry.State != jobstate.Rendering || entry.Seq != 3 {
		t.Fatalf("mirror not updated as expected: %+v ok=%v", entry, ok)
	}
}

func TestJobMirrorCallbackRejectsOutOfSetState(t *testing.T) {
	h, router := newJobMirrorTestHandler()
	rec := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_2","state":"totally_bogus","seq":1,"updated_at":"2026-07-01T12:00:00Z"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-set state, got %d (%s)", rec.Code, rec.Body.String())
	}
	if _, ok := h.jobMirror().Get("job_2"); ok {
		t.Fatal("out-of-set state must never be applied to the mirror")
	}
}

func TestJobMirrorCallbackRequiresAuth(t *testing.T) {
	h, router := newJobMirrorTestHandler()
	body := `{"job_id":"job_3","state":"queued","seq":1,"updated_at":"2026-07-01T12:00:00Z"}`

	missing := postCallback(t, router, "", body)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", missing.Code)
	}
	invalid := postCallback(t, router, "wrong-token", body)
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", invalid.Code)
	}
	if _, ok := h.jobMirror().Get("job_3"); ok {
		t.Fatal("unauthenticated callback must not mutate the mirror")
	}
}

func TestJobMirrorCallbackUnconfiguredTokenReturns503(t *testing.T) {
	h := &Handler{}
	router := chi.NewRouter()
	h.registerJobMirrorCallbackRoutes(router)
	rec := postCallback(t, router, "anything",
		`{"job_id":"job_4","state":"queued","seq":1}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when callback token unconfigured, got %d", rec.Code)
	}
}

func TestJobMirrorCallbackStaleSeqIsIdempotentNoOp(t *testing.T) {
	_, router := newJobMirrorTestHandler()
	// Apply seq=5 rendering.
	first := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_5","state":"rendering","seq":5,"updated_at":"2026-07-01T12:00:00Z"}`)
	if first.Code != http.StatusOK || !decodeCallbackResponse(t, first).Applied {
		t.Fatalf("first apply failed: %d %s", first.Code, first.Body.String())
	}
	// Stale seq=4 with a different state must be a 200 no-op.
	stale := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_5","state":"complete","seq":4,"updated_at":"2026-07-01T12:10:00Z"}`)
	if stale.Code != http.StatusOK {
		t.Fatalf("expected 200 for stale seq, got %d", stale.Code)
	}
	resp := decodeCallbackResponse(t, stale)
	if resp.Applied {
		t.Fatal("stale seq must be an idempotent no-op")
	}
	if resp.State != jobstate.Rendering || resp.Seq != 5 {
		t.Fatalf("mirror must be unchanged after stale callback, got state=%q seq=%d", resp.State, resp.Seq)
	}
}

func TestJobMirrorCallbackSameStateHigherSeqIsNoOp(t *testing.T) {
	_, router := newJobMirrorTestHandler()
	postCallback(t, router, testCallbackToken,
		`{"job_id":"job_6","state":"transcribing","seq":2,"updated_at":"2026-07-01T12:00:00Z"}`)
	// Same state, higher seq -> design treats as no-op (state already applied).
	same := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_6","state":"transcribing","seq":9,"updated_at":"2026-07-01T12:30:00Z"}`)
	if same.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", same.Code)
	}
	if decodeCallbackResponse(t, same).Applied {
		t.Fatal("same-state callback must be a no-op even with a higher seq")
	}
}

func TestJobMirrorCallbackMissingJobID(t *testing.T) {
	_, router := newJobMirrorTestHandler()
	rec := postCallback(t, router, testCallbackToken,
		`{"job_id":"","state":"queued","seq":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing job_id, got %d", rec.Code)
	}
}

func TestJobMirrorApplyAdvancesOnNewerInSetState(t *testing.T) {
	m := NewJobMirror()
	if _, applied := m.Apply(StatusCallback{JobID: "j", State: jobstate.Queued, Seq: 1}); !applied {
		t.Fatal("expected first in-set callback to apply")
	}
	if _, applied := m.Apply(StatusCallback{JobID: "j", State: jobstate.Rendering, Seq: 2}); !applied {
		t.Fatal("expected newer-seq distinct state to apply")
	}
	entry, ok := m.Get("j")
	if !ok || entry.State != jobstate.Rendering || entry.Seq != 2 {
		t.Fatalf("unexpected final entry: %+v ok=%v", entry, ok)
	}
}
