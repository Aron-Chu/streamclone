package analytics

import (
	"net/http"
	"time"
)

func (h *Handler) listActiveSyncs(w http.ResponseWriter, r *http.Request) {
	if h.syncService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync_unavailable"})
		return
	}
	syncs := h.syncService.ListActiveSyncs(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":      syncs,
		"syncs":     syncs,
		"updatedAt": time.Now().Unix(),
	})
}

func (h *Handler) trackingSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.collector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, h.collector.TrackingSnapshot())
}
