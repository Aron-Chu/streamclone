package analytics

import "net/http"

func (h *Handler) requirePulseWrite(w http.ResponseWriter) bool {
	if h == nil || !h.pulseRuntimeConfig().ReadOnlyMode {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error": "read_only_mode",
	})
	return false
}
