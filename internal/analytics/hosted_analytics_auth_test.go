package analytics

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
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

func TestHostedAlwaysTrackedUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var req *http.Request
			if method == http.MethodPost {
				req = httptest.NewRequest(method, "/v1/analytics/always-tracked", strings.NewReader(`{"channel":"xqc","track":true}`))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(method, "/v1/analytics/always-tracked", nil)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s status = %d, want 401: %s", method, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestNonHostedAlwaysTrackedAllowsGuestRead(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false},
		collector:   NewCollector(nil, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 50, time.Hour, 30*24*time.Hour, 200),
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/always-tracked", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
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

func TestHostedPortalChannelStreamsAllowsGuest(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/channels/xqc/streams", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("portal channel streams must be public-safe, got 401: %s", rec.Body.String())
	}
}

func TestHostedCorpusInternalGapsRequireAdminToken(t *testing.T) {
	hostedCorpusHandler := func(t *testing.T) http.Handler {
		t.Helper()
		h := &Handler{
			pulseHosted: PulseHostedConfig{Hosted: true},
			appConfig: config.Config{
				AdminArchiveEnabled:      true,
				AdminArchiveRequireToken: true,
				AdminArchiveToken:        "operator-secret",
			},
		}
		r := chi.NewRouter()
		h.CorpusRoutes(r)
		return r
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET internal readiness", method: http.MethodGet, path: "/v1/internal/corpus/readiness"},
		{name: "GET gaps", method: http.MethodGet, path: "/v1/internal/corpus/gaps?vod_id=vod-1"},
		{name: "POST gaps requeue", method: http.MethodPost, path: "/v1/internal/corpus/gaps/requeue", body: `{"segmentKeys":[]}`},
		{name: "GET workers", method: http.MethodGet, path: "/v1/internal/corpus/workers"},
		{name: "POST sync gold status", method: http.MethodPost, path: "/v1/internal/corpus/inventory/vod-1/sync-gold-status", body: `{}`},
	}

	for _, tc := range tests {
		t.Run(tc.name+" unauthenticated", func(t *testing.T) {
			r := hostedCorpusHandler(t)
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated %s %s status = %d, want 401: %s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}

	r := hostedCorpusHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/corpus/gaps/requeue", strings.NewReader(`{"segmentKeys":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(adminArchiveHeader, "operator-secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("authorized operator should not get 401, got %s", rec.Body.String())
	}
}

func TestLocalCorpusInternalRoutesSkipAdminAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false},
		appConfig: config.Config{
			AdminArchiveEnabled:      true,
			AdminArchiveRequireToken: true,
			AdminArchiveToken:        "operator-secret",
		},
	}
	r := chi.NewRouter()
	h.CorpusRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/corpus/gaps?vod_id=vod-local", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("local dev should not require admin token at middleware layer, got 401")
	}
}
