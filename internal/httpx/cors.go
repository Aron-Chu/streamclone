package httpx

import (
	"net/http"
	"strings"
)

// Portal origins served from Cloudflare Pages and production marketing site.
var pulsePortalOrigins = []string{
	"https://streampulse.stream",
	"https://www.streampulse.stream",
	"http://localhost:5173",
	"http://127.0.0.1:5173",
}

func pulsePortalOriginAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, allowed := range pulsePortalOrigins {
		if origin == allowed {
			return true
		}
	}
	if strings.HasSuffix(origin, ".pages.dev") {
		return true
	}
	return false
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if pulsePortalOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Streamclone-Beta-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CORSForOrigin(origin string) func(http.Handler) http.Handler {
	if origin == "" {
		origin = "http://localhost:8090"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqOrigin := r.Header.Get("Origin")
			if reqOrigin == origin {
				w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Streamclone-Beta-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
