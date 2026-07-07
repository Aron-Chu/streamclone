package analytics

import (
	"net"
	"net/http"
	"os"
	"strings"

	"streamclone/internal/config"
)

const opsProbeHeader = "X-Ops-Probe-Token"

func loadOpsProbeToken(cfg config.Config) string {
	if t := strings.TrimSpace(cfg.PulseOpsProbeToken); t != "" {
		return t
	}
	file := strings.TrimSpace(cfg.PulseOpsProbeTokenFile)
	if file == "" {
		return ""
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func opsProbeClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return ip
}

func isLoopbackOrPrivateIP(raw string) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// OpsProbeAuthMiddleware gates operator-only launch/readiness routes.
// Fail closed: when PULSE_OPS_PROBE_TOKEN is configured, every caller must
// present X-Ops-Probe-Token. Without a configured token, only loopback/private
// clients may pass when PULSE_ADMIN_LOCAL_BYPASS=true (local dev).
func OpsProbeAuthMiddleware(cfg config.Config, _ bool) func(http.Handler) http.Handler {
	token := loadOpsProbeToken(cfg)
	localBypass := cfg.PulseAdminLocalBypass
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimSpace(r.Header.Get(opsProbeHeader))
			if token != "" {
				if got == token {
					next.ServeHTTP(w, r)
					return
				}
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if localBypass && isLoopbackOrPrivateIP(opsProbeClientIP(r)) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	}
}
