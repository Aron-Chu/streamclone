package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetupDiagnostics(t *testing.T) {
	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{Profile: "core"})
	srv := httptest.NewServer(http.HandlerFunc(h.setupDiagnostics))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.setupDiagnostics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp setupDiagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Profile != "core" {
		t.Fatalf("profile = %q", resp.Profile)
	}
	if resp.Services.Metadata != "ready" {
		t.Fatalf("metadata = %q", resp.Services.Metadata)
	}
}

func TestSetupDiagnosticsSlowOptionalServicesReturnFast(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(slow.Close)

	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{
		Profile:           "full",
		ClipperServiceURL: slow.URL,
	})
	h.scraperAPIURL = slow.URL + "/v2/scrape"

	req := httptest.NewRequest(http.MethodGet, "/v1/setup/diagnostics", nil)
	rec := httptest.NewRecorder()
	started := time.Now()
	h.setupDiagnostics(rec, req)
	elapsed := time.Since(started)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("setup diagnostics took %s, want under 1.5s", elapsed)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp setupDiagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Services.Scraper != "offline" || resp.Services.Clipper != "offline" {
		t.Fatalf("services = %+v, want optional services offline on timeout", resp.Services)
	}
}
