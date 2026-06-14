package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetupWelcomeCoreProfile(t *testing.T) {
	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{
		Profile: "core",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/setup/welcome", nil)
	rec := httptest.NewRecorder()
	h.setupWelcome(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp setupWelcomeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Profile != "core" {
		t.Fatalf("profile = %q, want core", resp.Profile)
	}
	if resp.Services.Scraper != "offline" {
		t.Fatalf("scraper = %q, want offline without running service", resp.Services.Scraper)
	}
	if resp.Services.Clipper != "offline" {
		t.Fatalf("clipper = %q, want offline without running service", resp.Services.Clipper)
	}
	if resp.Incomplete {
		t.Fatalf("core profile should be complete when optional services are not required")
	}
	if resp.ShowWelcome {
		t.Fatalf("showWelcome should be false")
	}
}

func TestSetupWelcomeScraperProfileOffline(t *testing.T) {
	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{
		Profile: "scraper",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/setup/welcome", nil)
	rec := httptest.NewRecorder()
	h.setupWelcome(rec, req)

	var resp setupWelcomeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Profile != "scraper" {
		t.Fatalf("profile = %q, want scraper", resp.Profile)
	}
	if !resp.Incomplete {
		t.Fatalf("scraper profile should be incomplete when scraper is offline")
	}
}

func TestSetupWelcomeSlowOptionalServicesReturnFast(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/v1/setup/welcome", nil)
	rec := httptest.NewRecorder()
	started := time.Now()
	h.setupWelcome(rec, req)
	elapsed := time.Since(started)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("setup welcome took %s, want under 1.5s", elapsed)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp setupWelcomeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Services.Scraper != "offline" || resp.Services.Clipper != "offline" {
		t.Fatalf("services = %+v, want optional services offline on timeout", resp.Services)
	}
	if !resp.Incomplete {
		t.Fatalf("full profile should be incomplete when optional services time out")
	}
}

func TestScraperHealthURL(t *testing.T) {
	if got := scraperHealthURL("http://scraper:8000/v2/scrape"); got != "http://scraper:8000/health" {
		t.Fatalf("got %q", got)
	}
	if got := scraperHealthURL(""); got != "http://scraper:8000/health" {
		t.Fatalf("empty got %q", got)
	}
}
