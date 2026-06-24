package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

func TestAdminPulseHealthRequiresAdminAuth(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	r := chi.NewRouter()
	h.AdminPulseRoutes(r, config.Config{
		AdminArchiveRequireToken: true,
		AdminArchiveToken:        "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/pulse/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/admin/pulse/health", nil)
	req.Header.Set(adminArchiveHeader, "secret")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status with token = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminPulseHealthAcceptsAccessJWT(t *testing.T) {
	old := cfAccessValidator
	t.Cleanup(func() { cfAccessValidator = old })
	cfAccessValidator = func(_, _, _ string) (string, bool) {
		return "op@example.com", true
	}

	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	r := chi.NewRouter()
	h.AdminPulseRoutes(r, config.Config{
		PulseCFAccessTeamDomain: "team.cloudflareaccess.com",
		PulseCFAccessAud:        "aud-test",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/pulse/health", nil)
	req.Header.Set(cfAccessJWTHeader, "fake.jwt.token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status with jwt = %d, want 200", rec.Code)
	}
}

func TestAdminPulseRegistryRequiresAuth(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	r := chi.NewRouter()
	h.AdminPulseRoutes(r, config.Config{
		AdminArchiveRequireToken: true,
		AdminArchiveToken:        "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/pulse/registry", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("registry without token = %d, want 401", rec.Code)
	}
}

func TestAdminPulseRegistryPayloadShape(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	payload := h.adminPulseRegistryPayload()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"active", "max", "trackedChannels", "alwaysTracked"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("registry missing %q", key)
		}
	}
}

func TestAdminPulseJobsPayloadShape(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	payload := h.adminPulseJobsPayload()
	raw, _ := json.Marshal(payload)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	for _, key := range []string{"active", "max", "jobs"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("jobs missing %q", key)
		}
	}
}

func TestCollectorEvictChannel(t *testing.T) {
	c := &Collector{
		maxTracked: 10,
		tracked:    map[string]*trackedChannel{},
	}
	c.tracked["xqc"] = &trackedChannel{login: "xqc", refCounts: map[string]int{}}
	active, evicted := c.EvictChannel("xqc")
	if !evicted || active != 0 {
		t.Fatalf("evict = %v active = %d", evicted, active)
	}
}
