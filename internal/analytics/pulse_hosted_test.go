package analytics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPulseHostedAuthMiddlewareAllowsGuest(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{
			Hosted:   true,
			BetaKeys: []string{"secret-one"},
		},
	}
	called := false
	var gotPrincipal PulsePrincipal
	mw := h.pulseHostedAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		p, ok := pulsePrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		gotPrincipal = p
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without key, got %d", rec.Code)
	}
	if !called {
		t.Fatal("handler should run for guest principal")
	}
	if gotPrincipal.Kind != "guest" || gotPrincipal.ID == "" {
		t.Fatalf("guest principal = %#v", gotPrincipal)
	}
}

func TestPulseHostedAuthMiddlewareAcceptsBetaKey(t *testing.T) {
	const validKey = "secret-one"
	h := &Handler{
		pulseHosted: PulseHostedConfig{
			Hosted:   true,
			BetaKeys: []string{validKey},
		},
	}
	var gotPrincipal PulsePrincipal
	mw := h.pulseHostedAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := pulsePrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		gotPrincipal = p
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
	req.Header.Set("X-Streamclone-Beta-Key", validKey)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with key, got %d", rec.Code)
	}
	if gotPrincipal.Kind != "beta" {
		t.Fatalf("principal kind = %q, want beta", gotPrincipal.Kind)
	}
}

func TestPulseBetaKeyMiddlewareDelegatesToHostedAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	called := false
	mw := h.pulseBetaKeyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without key, got %d", rec.Code)
	}
	if !called {
		t.Fatal("handler should run for guest principal")
	}
}

func TestPulseHostedAuthorizedAcceptsValidBetaKey(t *testing.T) {
	const validKey = "secret-one"
	h := &Handler{
		pulseHosted: PulseHostedConfig{
			Hosted:   true,
			BetaKeys: []string{validKey},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Streamclone-Beta-Key", validKey)
	if !h.pulseHosted.authorized(req) {
		t.Fatal("authorized() should accept valid beta key")
	}
}

func TestPulseHostedGuestPrincipalStablePerIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.42:443"
	p1 := guestPulsePrincipal(req)
	p2 := guestPulsePrincipal(req)
	if p1.ID != p2.ID || p1.Kind != "guest" {
		t.Fatalf("guest principal not stable: %#v vs %#v", p1, p2)
	}
}

func TestParsePulseBetaKeys(t *testing.T) {
	got := ParsePulseBetaKeys(" a , b , ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ParsePulseBetaKeys() = %#v", got)
	}
}

func TestBetaKeyRequiredDisabled(t *testing.T) {
	cfg := PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}
	if cfg.BetaKeyRequired() {
		t.Fatal("BetaKeyRequired should be false")
	}
}
