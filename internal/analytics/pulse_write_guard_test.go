package analytics

import (
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
