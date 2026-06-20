package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterStreamLifecycleExempt(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/v1/stream/start", "/v1/stream/stop", "/v1/stream/keepalive"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s should bypass rate limit, got %d", path, rec.Code)
		}
	}
}

func TestClientIPUsesForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	if got := clientIP(req); got != "203.0.113.9" {
		t.Fatalf("clientIP() = %q, want 203.0.113.9", got)
	}
}

func TestRateLimiterBurstAndRefill(t *testing.T) {
	rl := NewRateLimiter(100, 3)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d within burst should be allowed", i)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("4th request should be denied")
	}
	if !rl.allow("5.6.7.8") {
		t.Fatal("different ip should have its own bucket")
	}
	time.Sleep(20 * time.Millisecond)
	if !rl.allow("1.2.3.4") {
		t.Fatal("bucket should refill after wait")
	}
}
