package analytics

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"streamclone/internal/config"
)

const adminArchiveHeader = "X-Admin-Archive-Token"

type adminRateLimiter struct {
	mu     sync.Mutex
	last   map[string][]time.Time
	limit  int
	window time.Duration
}

func newAdminRateLimiter(limit int, window time.Duration) *adminRateLimiter {
	return &adminRateLimiter{last: map[string][]time.Time{}, limit: limit, window: window}
}

func (r *adminRateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	times := r.last[key]
	var kept []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.last[key] = kept
		return false
	}
	kept = append(kept, now)
	r.last[key] = kept
	return true
}

func loadAdminArchiveToken(cfg config.Config) string {
	if t := strings.TrimSpace(cfg.AdminArchiveToken); t != "" {
		return t
	}
	file := strings.TrimSpace(cfg.AdminArchiveTokenFile)
	if file == "" {
		return ""
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// AdminArchiveAuthMiddleware gates /v1/admin/archive routes.
func AdminArchiveAuthMiddleware(cfg config.Config, next http.Handler) http.Handler {
	token := loadAdminArchiveToken(cfg)
	limiter := newAdminRateLimiter(10, time.Minute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.AdminArchiveEnabled {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if cfg.AdminArchiveRequireToken {
			got := strings.TrimSpace(r.Header.Get(adminArchiveHeader))
			if token == "" || got != token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if !limiter.allow(got) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
