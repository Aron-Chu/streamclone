package analytics

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/jobstate"
	"streamclone/internal/redact"
)

// jobMirrorCallbackPath is the authed idempotent Status_Callback endpoint
// (ReplayForge → Streamclone). It matches the design interface table row
// `POST /v1/clipper/callback`.
const jobMirrorCallbackPath = "/v1/clipper/callback"

// jobMirrorCallbackResponse is the redacted acknowledgement returned for every
// accepted callback (applied or idempotent no-op). State is always a
// Job_State_Set member, but it is still routed through the redaction chokepoint
// per the mirror/log/display invariant (Requirement 1.7).
type jobMirrorCallbackResponse struct {
	JobID   string `json:"jobId"`
	State   string `json:"state"`
	Seq     int64  `json:"seq"`
	Applied bool   `json:"applied"`
}

// jobMirror lazily initialises and returns the process Job_Mirror. Handlers are
// constructed via struct literals in tests, so this tolerates a zero-value
// Handler.
func (h *Handler) jobMirror() *JobMirror {
	h.jobMirrorOnce.Do(func() {
		h.jobMirrorStore = NewJobMirror()
	})
	return h.jobMirrorStore
}

func (h *Handler) registerJobMirrorCallbackRoutes(r chi.Router) {
	r.Post(jobMirrorCallbackPath, h.receiveJobMirrorStatusCallback)
}

// receiveJobMirrorStatusCallback is the idempotent authed Status_Callback
// handler that owns the Job_Mirror read model
// (spec auto-clipper-replayforge-productization, RF-P1-008,
// Requirements 2.2, 2.4). Semantics follow the design pseudocode:
//
//	if !ValidAuth(r)        -> 401
//	if !InStateSet(state)   -> 400
//	if seq <= cur.seq || state == cur.state -> 200 idempotent no-op
//	else apply -> 200
//
// Unauthenticated rejection is required here; the dedicated unauth-reject
// property (P4) lands with RF-P1-009, and reconciliation (RF-P1-010) will reuse
// JobMirror.Get/Apply without changing this contract.
func (h *Handler) receiveJobMirrorStatusCallback(w http.ResponseWriter, r *http.Request) {
	if !h.validJobMirrorCallbackAuth(w, r) {
		return // 401 (or 503 when unconfigured) already written; mirror untouched
	}

	var cb StatusCallback
	if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	cb.JobID = strings.TrimSpace(cb.JobID)
	cb.State = strings.TrimSpace(cb.State)
	if cb.JobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_job_id"})
		return
	}

	// Reject states outside the Job_State_Set: they are never applied to or
	// displayed from the mirror (Requirement 2.2, Property 2).
	if !jobstate.InSet(cb.State) {
		slog.Warn(redact.Log("job_mirror callback rejected: state not in Job_State_Set",
			"job_id", cb.JobID, "state", cb.State))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}

	entry, applied := h.jobMirror().Apply(cb)
	// Both an applied update and an idempotent no-op return 200 OK
	// (Requirement 2.4, Property 5). On a no-op for a never-seen job the entry
	// is zero, so fall back to the callback's job id for the acknowledgement.
	jobID := entry.JobID
	if jobID == "" {
		jobID = cb.JobID
	}
	writeJSON(w, http.StatusOK, jobMirrorCallbackResponse{
		JobID:   jobID,
		State:   redact.Display(entry.State),
		Seq:     entry.Seq,
		Applied: applied,
	})
}

// validJobMirrorCallbackAuth enforces the server-side callback bearer token
// (REPLAYFORGE_CALLBACK_TOKEN). The browser never carries this token; it is a
// server-to-server credential. Missing/invalid tokens are rejected with 401 and
// no mirror mutation; an unconfigured server token yields 503 (the endpoint is
// not set up), matching the existing ReplayForge webhook behaviour.
func (h *Handler) validJobMirrorCallbackAuth(w http.ResponseWriter, r *http.Request) bool {
	token := ""
	if h != nil {
		token = strings.TrimSpace(h.appConfig.ReplayForgeCallbackToken)
	}
	if token == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "callback_unconfigured"})
		return false
	}
	got := bearerTokenFromRequest(r)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}
