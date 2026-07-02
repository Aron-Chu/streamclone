package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type corpusGapsListResponse struct {
	Gaps  []CorpusGoldGap `json:"gaps"`
	Count int             `json:"count"`
}

type corpusGapsRequeueRequest struct {
	SegmentKeys []string `json:"segmentKeys"`
}

type corpusGapsRequeueResponse struct {
	Requeued int `json:"requeued"`
}

type corpusWorkersResponse struct {
	Workers []CorpusGoldWorkerLease `json:"workers"`
}

func (h *Handler) registerCorpusGapRoutes(r chi.Router) {
	r.Get("/v1/internal/corpus/gaps", h.getCorpusGoldGaps)
	r.Post("/v1/internal/corpus/gaps/requeue", h.postCorpusGoldGapsRequeue)
	r.Get("/v1/internal/corpus/workers", h.getCorpusGoldWorkers)
	r.Post("/v1/internal/corpus/inventory/{vod_id}/sync-gold-status", h.postSyncTop500GoldStatus)
}

func (h *Handler) getCorpusGoldGaps(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store unavailable"})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	gaps, err := h.store.ListCorpusGoldGaps(r.Context(), limit, r.URL.Query().Get("vod_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, corpusGapsListResponse{Gaps: gaps, Count: len(gaps)})
}

func (h *Handler) postCorpusGoldGapsRequeue(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store unavailable"})
		return
	}
	var req corpusGapsRequeueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	n, err := h.store.RequeueCorpusGoldSegments(r.Context(), req.SegmentKeys)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, corpusGapsRequeueResponse{Requeued: n})
}

func (h *Handler) getCorpusGoldWorkers(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store unavailable"})
		return
	}
	workers, err := h.store.ListCorpusGoldWorkerLeases(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, corpusWorkersResponse{Workers: workers})
}

func (h *Handler) postSyncTop500GoldStatus(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store unavailable"})
		return
	}
	vodID := strings.TrimSpace(chi.URLParam(r, "vod_id"))
	updated, err := h.store.SyncTop500GoldStatusFromSegments(r.Context(), vodID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vodId": vodID, "updated": updated})
}
