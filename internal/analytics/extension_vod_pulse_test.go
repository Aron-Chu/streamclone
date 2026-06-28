package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestExtensionPulseVodUnknownReturnsMissing(t *testing.T) {
	h := &Handler{store: nil}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/vods/2806037629", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("vodId", "2806037629")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.extensionPulseVod(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"coverageStatus":"error"`) {
		t.Fatalf("body = %s, want coverageStatus error when store nil", rec.Body.String())
	}
}

func TestExtensionPulseVodInvalidID(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/vods/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("vodId", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.extensionPulseVod(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestExtensionVodCoverageStatusMessages(t *testing.T) {
	status, msg := extensionVodCoverageStatus(ExtensionCoverage{State: "syncing"}, 0, true)
	if status != "syncing" {
		t.Fatalf("status = %q, want syncing", status)
	}
	if !strings.Contains(msg, "syncing") {
		t.Fatalf("message = %q", msg)
	}

	status, msg = extensionVodCoverageStatus(ExtensionCoverage{State: "ready"}, 5, false)
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
	if msg != "" {
		t.Fatalf("message = %q, want empty for ready", msg)
	}

	status, msg = extensionVodCoverageStatus(ExtensionCoverage{State: "unknown"}, 0, false)
	if status != "missing" {
		t.Fatalf("status = %q, want missing", status)
	}
	if !strings.Contains(msg, "indexed") {
		t.Fatalf("message = %q", msg)
	}
}

func TestExtensionPulseVodNeverReturnsBare404String(t *testing.T) {
	h := &Handler{store: nil}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/vods/2806037629", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("vodId", "2806037629")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.extensionPulseVod(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "vod_pulse 404") {
		t.Fatalf("body must not contain bare vod_pulse 404: %s", body)
	}
	if rec.Header().Get("Content-Type") == "" {
		t.Fatalf("expected JSON content type")
	}
}
