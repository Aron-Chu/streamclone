package analytics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPrincipalFromRequest(t *testing.T) {
	cfg := PulseHostedConfig{Hosted: true, BetaKeys: []string{"alpha", "beta"}}
	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "alpha")
	id, kind, ok := principalFromRequest(req, cfg)
	if !ok || kind != "beta" || id == "" {
		t.Fatalf("principalFromRequest() = (%q, %q, %v)", id, kind, ok)
	}
	id2, _, ok2 := principalFromRequest(req, cfg)
	if !ok2 || id != id2 {
		t.Fatalf("principal hash not stable: %q vs %q", id, id2)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist", nil)
	req2.Header.Set("X-Streamclone-Beta-Key", "beta")
	idBeta, _, okBeta := principalFromRequest(req2, cfg)
	if !okBeta || idBeta == id {
		t.Fatal("expected distinct principal ids for distinct beta keys")
	}
}

func TestWatchlistUnauthorizedWithoutKey(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
