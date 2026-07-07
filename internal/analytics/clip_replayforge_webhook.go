package analytics

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type replayForgeWebhookResponse struct {
	Updated int `json:"updated"`
}

func (h *Handler) registerReplayForgeWebhookRoutes(r chi.Router) {
	r.Route("/v1/internal/replayforge", func(r chi.Router) {
		r.Post("/jobs/{jobID}", h.receiveReplayForgeJobWebhook)
	})
}

func (h *Handler) receiveReplayForgeJobWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.requireReplayForgeWebhook(w, r) {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "jobID"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_job_id"})
		return
	}
	var payload ReplayForgeJobStatusResponse
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if payload.Job == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_job"})
		return
	}
	if bodyID, _ := payload.Job["id"].(string); strings.TrimSpace(bodyID) != "" && strings.TrimSpace(bodyID) != jobID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job_id_mismatch"})
		return
	}
	payload.Job["id"] = jobID
	if payload.State() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_state"})
		return
	}
	updated, err := h.store.UpdateClipCandidateJobsByReplayForgeID(r.Context(), jobID, payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "clip_job_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_job_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, replayForgeWebhookResponse{Updated: updated})
}

func (h *Handler) requireReplayForgeWebhook(w http.ResponseWriter, r *http.Request) bool {
	token := ""
	if h != nil {
		token = strings.TrimSpace(h.appConfig.ReplayForgeCallbackToken)
	}
	if token == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "replayforge_webhook_unconfigured"})
		return false
	}
	got := bearerTokenFromRequest(r)
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}
