package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPulseReadOnlyModeRejectsMutations(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(PulseRuntimeConfig{
		Configured:   true,
		ReadOnlyMode: true,
	})
	rec := httptest.NewRecorder()
	if h.requirePulseWrite(rec) {
		t.Fatal("expected read-only write guard to reject mutation")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestPulseReadOnlyModeAllowsReads(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(PulseRuntimeConfig{Configured: true})
	rec := httptest.NewRecorder()
	if !h.requirePulseWrite(rec) {
		t.Fatal("expected non-read-only guard to allow mutation")
	}
}

func TestHostedUserStateAllowsGuestPrincipal(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/bookmarks", nil)
	ctx := context.WithValue(req.Context(), pulsePrincipalCtxKey{}, PulsePrincipal{ID: "guest-id", Kind: "guest"})
	rec := httptest.NewRecorder()

	principal, ok := h.requireHostedUserStatePrincipal(rec, req.WithContext(ctx))
	if !ok {
		t.Fatalf("expected guest principal to be allowed, status=%d", rec.Code)
	}
	if principal.ID != "guest-id" || principal.Kind != "guest" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestHostedUserStateAllowsBetaPrincipal(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/bookmarks", nil)
	ctx := context.WithValue(req.Context(), pulsePrincipalCtxKey{}, PulsePrincipal{ID: "beta-id", Kind: "beta"})
	rec := httptest.NewRecorder()

	principal, ok := h.requireHostedUserStatePrincipal(rec, req.WithContext(ctx))
	if !ok {
		t.Fatalf("expected beta principal to be allowed, status=%d", rec.Code)
	}
	if principal.ID != "beta-id" || principal.Kind != "beta" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestHostedUserStateRejectsMissingPrincipal(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/bookmarks", nil)
	rec := httptest.NewRecorder()

	if _, ok := h.requireHostedUserStatePrincipal(rec, req); ok {
		t.Fatal("expected missing principal to be rejected")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireHostedNonGuestPrincipalRejectsGuest(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/always-tracked", nil)
	ctx := context.WithValue(req.Context(), pulsePrincipalCtxKey{}, guestPulsePrincipal(req))
	if _, ok := h.requireHostedNonGuestPrincipal(rec, req.WithContext(ctx)); ok {
		t.Fatal("expected guest principal to be rejected")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
