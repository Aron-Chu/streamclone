package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHostedStreamDetailUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "unauthorized" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHostedStreamDetailSparseFalseUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1?sparse=false", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHostedStreamTimelineAuthMiddlewareAcceptsBetaKey(t *testing.T) {
	const betaKey = "secret-one"
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{betaKey}},
	}
	called := false
	handler := h.pulseHostedAuthMiddleware(h.pulseHostedStreamTimelineAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1", nil)
	req.Header.Set("X-Streamclone-Beta-Key", betaKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Fatal("handler should run for beta principal")
	}
}

func TestNonHostedStreamTimelineAuthAllowsGuest(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false},
	}
	called := false
	handler := h.pulseHostedStreamTimelineAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Fatal("local mode must not block guest timeline access at middleware layer")
	}
}

func TestHostedStreamDetailResponseOmitsRollupsWhenUnauthorized(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := strings.ToLower(rec.Body.String())
	if strings.Contains(body, `"rollups"`) {
		t.Fatalf("unauthorized response must not include rollups, got %s", rec.Body.String())
	}
}

func TestHostedReplayHeatmapUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/streams/stream-1/replay-heatmap", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHostedChannelLiveUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/channels/ludwig/live", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := strings.ToLower(rec.Body.String())
	if strings.Contains(body, `"rollups"`) {
		t.Fatalf("unauthorized response must not include rollups, got %s", rec.Body.String())
	}
}

func TestHostedChannelLiveSparseFalseUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/channels/ludwig/live?sparse=false", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestNonHostedChannelLiveTimelineAuthAllowsGuest(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false},
	}
	called := false
	handler := h.pulseHostedStreamTimelineAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/channels/ludwig/live", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Fatal("local mode must not block guest channel live at middleware layer")
	}
}
