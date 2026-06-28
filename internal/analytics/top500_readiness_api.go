package analytics

import (
	"net/http"
	"strconv"
)

func (h *Handler) top100Readiness(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	topN := DefaultTop500MetadataTopN
	if raw := r.URL.Query().Get("topN"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			topN = parsed
		}
	}
	admissionEnabled := h.corpusRuntimeConfig().LiveAdmissionEnabled
	report, err := h.buildTop100ReadinessReport(r.Context(), topN, admissionEnabled)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}
