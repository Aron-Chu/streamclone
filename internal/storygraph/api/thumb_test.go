package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMediaThumbRejectsMissingURL(t *testing.T) {
	h := &Handler{enabled: true}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/pulse-wire/thumb", nil)
	h.mediaThumb(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMediaThumbRejectsDisallowedHost(t *testing.T) {
	h := &Handler{enabled: true}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/pulse-wire/thumb?u=https://evil.example/a.jpg", nil)
	h.mediaThumb(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
