package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
