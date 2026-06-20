package api

import (
	"net/http"

	"streamclone/internal/storygraph/cluster"
)

func (h *Handler) reclassify(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.ReclassifyRecentCategories(r.Context(), 200, cluster.ClassifyCategory)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "updated": n})
}
