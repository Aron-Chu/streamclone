package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) WithPulseBackfill(m *PulseBackfillManager) *Handler {
	h.pulseBackfill = m
	return h
}

func (h *Handler) extensionPulseBackfill(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if h.pulseBackfill == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pulse_backfill_unavailable"})
		return
	}

	var req struct {
		StreamID          string `json:"streamId"`
		Mode              string `json:"mode"`
		FromOffsetSeconds int    `json:"fromOffsetSeconds"`
		ToOffsetSeconds   int    `json:"toOffsetSeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	req.StreamID = strings.TrimSpace(req.StreamID)
	if req.StreamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = "missed"
	}

	job, err := h.pulseBackfill.Enqueue(r.Context(), PulseBackfillRequest{
		StreamID:          req.StreamID,
		Login:             login,
		Mode:              req.Mode,
		FromOffsetSeconds: req.FromOffsetSeconds,
		ToOffsetSeconds:   req.ToOffsetSeconds,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrPulseBackfillNoStream {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	statusCode := http.StatusAccepted
	if job.Status == PulseBackfillAlreadyAvailable {
		statusCode = http.StatusOK
	}
	writeJSON(w, statusCode, job)
}

func (h *Handler) extensionPulseBackfillStatus(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(chi.URLParam(r, "jobId"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_job_id"})
		return
	}
	if h.pulseBackfill == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pulse_backfill_unavailable"})
		return
	}
	job := h.pulseBackfill.GetJob(jobID)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) invalidateExtensionPulseCache(ctx context.Context, login string) {
	if h == nil || h.rdb == nil {
		return
	}
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	_ = h.rdb.Del(ctx, extPulseCachePrefix+login).Err()
}
