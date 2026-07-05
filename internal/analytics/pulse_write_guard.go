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

func pulsePrincipalCanWriteUserState(principal PulsePrincipal) bool {
	switch principal.Kind {
	case "beta", "device", "user", "guest":
		return principal.ID != ""
	default:
		return false
	}
}

func (h *Handler) requireHostedUserStatePrincipal(w http.ResponseWriter, r *http.Request) (PulsePrincipal, bool) {
	principal, ok := pulsePrincipalFromContext(r.Context())
	if ok && pulsePrincipalCanWriteUserState(principal) {
		return principal, true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"error": "unauthorized",
		"hint":  "Set X-Streamclone-Beta-Key or Authorization to save private Pulse data.",
	})
	return PulsePrincipal{}, false
}

func pulsePrincipalCanUsePrivateClips(principal PulsePrincipal) bool {
	switch principal.Kind {
	case "beta", "device", "user":
		return principal.ID != ""
	default:
		return false
	}
}

func (h *Handler) requireHostedPrivateClipsPrincipal(w http.ResponseWriter, r *http.Request) (PulsePrincipal, bool) {
	principal, ok := pulsePrincipalFromContext(r.Context())
	if ok && pulsePrincipalCanUsePrivateClips(principal) {
		return principal, true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"error": "unauthorized",
		"hint":  "Set X-Streamclone-Beta-Key or Authorization to use private Pulse clips.",
	})
	return PulsePrincipal{}, false
}
