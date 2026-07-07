package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamclone/internal/config"
)

func TestOpsProbeAuthMiddlewareHostedRequiresToken(t *testing.T) {
	cfg := config.Config{PulseOpsProbeToken: "probe-secret"}
	mux := http.NewServeMux()
	mux.Handle("/v1/internal/ops/readiness", OpsProbeAuthMiddleware(cfg, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(opsProbeHeader, "probe-secret")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestOpsProbeAuthMiddlewareRequiresTokenEvenWhenNotHosted(t *testing.T) {
	cfg := config.Config{PulseOpsProbeToken: "probe-secret"}
	handler := OpsProbeAuthMiddleware(cfg, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when token configured without header", rec.Code)
	}
}

func TestOpsProbeAuthMiddlewareLocalBypassLoopbackOnly(t *testing.T) {
	cfg := config.Config{PulseAdminLocalBypass: true}
	handler := OpsProbeAuthMiddleware(cfg, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback bypass status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("public IP status = %d, want 401", rec.Code)
	}
}

func TestOpsReadinessRequiresStore(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/ops/readiness?topN=10", nil)
	rec := httptest.NewRecorder()
	h.opsReadiness(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestOpsLaunchSnapshotWithoutStore(t *testing.T) {
	h := (&Handler{}).
		WithPulseHosted(PulseHostedConfig{Hosted: true, MaxActiveChannels: 250}).
		WithPulseRuntime(DefaultPulseRuntimeConfig())
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/ops/launch-snapshot", nil)
	rec := httptest.NewRecorder()
	h.opsLaunchSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var payload OpsLaunchSnapshotResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Health.Caps.ActiveChannels != 250 {
		t.Fatalf("caps = %+v", payload.Health.Caps)
	}
	if !payload.Redis.Available {
		if payload.Redis.Error == "" {
			t.Fatal("expected redis unavailable error")
		}
	}
}

func TestIsLoopbackOrPrivateIP(t *testing.T) {
	if !isLoopbackOrPrivateIP("127.0.0.1") {
		t.Fatal("expected loopback")
	}
	if !isLoopbackOrPrivateIP("10.0.0.2") {
		t.Fatal("expected private")
	}
	if isLoopbackOrPrivateIP("203.0.113.9") {
		t.Fatal("expected public IP to fail")
	}
}
