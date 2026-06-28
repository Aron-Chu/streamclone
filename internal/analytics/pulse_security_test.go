package analytics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestRateLimitExceeded(t *testing.T) {
	if !rateLimitExceeded(3, 2) {
		t.Fatal("expected limit exceeded")
	}
	if rateLimitExceeded(2, 2) {
		t.Fatal("expected limit not exceeded at boundary")
	}
	if rateLimitExceeded(1, 0) {
		t.Fatal("zero limit disables enforcement")
	}
}

func TestWatchUnauthorizedHosted(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		collector:   NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10),
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/analytics/channels/xqc/watch", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("guest watch must not return 401, got %d", rec.Code)
	}
}

func TestWatchInvalidBetaKeyHosted(t *testing.T) {
	const validKey = "secret-one"
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{validKey}},
		collector:   NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10),
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/analytics/channels/xqc/watch", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "wrong-key")
	req.RemoteAddr = "203.0.113.10:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("wrong beta key should fall back to guest, got 401")
	}
}

func TestWatchValidBetaKeyNot401Hosted(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		collector:   NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10),
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/analytics/channels/xqc/watch", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("valid beta key must not return 401, got %d", rec.Code)
	}
}

func TestWatchLocalModeNot401WithoutKey(t *testing.T) {
	// Local/non-hosted stacks intentionally allow watch without beta key.
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false, BetaKeys: []string{"secret-one"}},
		collector:   NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10),
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/analytics/channels/xqc/watch", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("local mode watch without key should not 401, got %d", rec.Code)
	}
}

func TestWatchRateLimitHTTP429(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		collector:   NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10),
	}
	h.rateLimiter = &PulseRateLimiter{watchPerMin: 1}
	h.rateLimiter.testAllowFn = func(_ string, limit int) int64 { return int64(limit + 1) }
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/analytics/channels/xqc/watch", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("rate limited valid key must not return 401")
	}
}
