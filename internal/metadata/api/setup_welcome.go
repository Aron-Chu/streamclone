package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	setupProbeRequestTimeout = 500 * time.Millisecond
	setupProbeRetryDelay     = 100 * time.Millisecond
	setupProbeBudget         = 1300 * time.Millisecond
	scraperReadyCacheTTL     = 15 * time.Second
)

type SetupWelcomeOptions struct {
	Profile               string
	DevTokenImportEnabled bool
	OAuthClientID         string
	OAuthClientSecret     string
	ClipperServiceURL     string
}

type setupWelcomeServices struct {
	Scraper string `json:"scraper"`
	Clipper string `json:"clipper"`
	Pulse   string `json:"pulse"`
}

type setupWelcomeResponse struct {
	Profile       string               `json:"profile"`
	Services      setupWelcomeServices `json:"services"`
	Incomplete    bool                 `json:"incomplete"`
	ShowWelcome   bool                 `json:"showWelcome"`
	SetupGuideURL string               `json:"setupGuideUrl"`
}

type setupPulseTimeseriesStatus struct {
	Enabled       bool   `json:"enabled"`
	Configured    bool   `json:"configured"`
	State         string `json:"state"`
	BackfillState string `json:"backfillState,omitempty"`
}

func (h *Handler) WithSetupWelcome(opts SetupWelcomeOptions) *Handler {
	profile := strings.ToLower(strings.TrimSpace(opts.Profile))
	if profile == "" {
		profile = "core"
	}
	h.streamcloneProfile = profile
	h.devTokenImportEnabled = opts.DevTokenImportEnabled
	h.oauthClientID = strings.TrimSpace(opts.OAuthClientID)
	h.oauthClientSecret = strings.TrimSpace(opts.OAuthClientSecret)
	clipperURL := strings.TrimSpace(opts.ClipperServiceURL)
	if clipperURL == "" {
		clipperURL = "http://clipper:8095"
	}
	h.clipperServiceURL = strings.TrimRight(clipperURL, "/")
	return h
}

func (h *Handler) setupWelcome(w http.ResponseWriter, r *http.Request) {
	profile := h.streamcloneProfile
	if profile == "" {
		profile = "core"
	}

	ctx, cancel := context.WithTimeout(r.Context(), setupProbeBudget)
	defer cancel()

	statuses := h.probeSetupServices(ctx, map[string]string{
		"scraper": scraperHealthURL(h.scraperAPIURL),
		"clipper": h.clipperServiceURL + "/v1/twitch/status",
	})
	services := setupWelcomeServices{
		Scraper: statuses["scraper"],
		Clipper: statuses["clipper"],
		Pulse:   h.pulseServiceReady(ctx),
	}

	incomplete := false
	if profileHasScraper(profile) && services.Scraper == "offline" {
		incomplete = true
	}
	if profileHasClipper(profile) && services.Clipper == "offline" {
		incomplete = true
	}
	if profileHasPulse(profile) && services.Pulse == "offline" {
		incomplete = true
	}

	writeJSON(w, http.StatusOK, setupWelcomeResponse{
		Profile:       profile,
		Services:      services,
		Incomplete:    incomplete,
		ShowWelcome:   false,
		SetupGuideURL: "https://github.com/Aron-Chu/streamclone/blob/master/docs/install-desktop.md",
	})
}

func serviceStatus(ok bool) string {
	if ok {
		return "ready"
	}
	return "offline"
}

func profileHasScraper(profile string) bool {
	return profile == "scraper" || profile == "full"
}

func profileHasClipper(profile string) bool {
	return profile == "clipper" || profile == "full"
}

func profileHasPulse(profile string) bool {
	return profile == "pulse"
}

func (h *Handler) pulseServiceReady(ctx context.Context) string {
	grafanaTargets := []string{
		"http://host.docker.internal:3000/api/health",
		"http://grafana:3000/api/health",
	}
	influxTargets := []string{
		"http://host.docker.internal:18086/health",
		"http://influxdb:8086/health",
	}
	if !h.probeAnyServiceHealth(ctx, grafanaTargets) {
		return "offline"
	}
	if !h.probeAnyServiceHealth(ctx, influxTargets) {
		return "offline"
	}
	if !h.pulseTimeseriesReady(ctx) {
		return "offline"
	}
	return "ready"
}

func (h *Handler) pulseTimeseriesReady(ctx context.Context) bool {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(setupProbeRetryDelay):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, setupProbeRequestTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, "http://analytics:8080/v1/analytics/timeseries/status", nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := h.http.Do(req)
		if err != nil {
			cancel()
			continue
		}
		var status setupPulseTimeseriesStatus
		decodeErr := json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()
		cancel()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && decodeErr == nil &&
			status.Enabled && status.Configured && status.State == "ready" &&
			(status.BackfillState == "" || status.BackfillState == "completed") {
			return true
		}
	}
	return false
}

func scraperHealthURL(scrapeURL string) string {
	base := strings.TrimSpace(scrapeURL)
	if base == "" {
		return "http://scraper:8000/health"
	}
	if idx := strings.Index(base, "/v2/"); idx > 0 {
		return strings.TrimRight(base[:idx], "/") + "/health"
	}
	return "http://scraper:8000/health"
}

func (h *Handler) probeSetupServices(ctx context.Context, targets map[string]string) map[string]string {
	statuses := make(map[string]string, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, rawURL := range targets {
		wg.Add(1)
		go func(name, rawURL string) {
			defer wg.Done()
			status := serviceStatus(h.probeServiceHealth(ctx, rawURL))
			mu.Lock()
			statuses[name] = status
			mu.Unlock()
		}(name, rawURL)
	}
	wg.Wait()
	return statuses
}

func (h *Handler) probeAnyServiceHealth(ctx context.Context, targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	results := make(chan bool, len(targets))
	for _, rawURL := range targets {
		go func(rawURL string) {
			results <- h.probeServiceHealth(ctx, rawURL)
		}(rawURL)
	}
	for range targets {
		if <-results {
			return true
		}
	}
	return false
}

func (h *Handler) probeServiceHealth(ctx context.Context, rawURL string) bool {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(setupProbeRetryDelay):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, setupProbeRequestTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := h.http.Do(req)
		if err != nil {
			cancel()
			continue
		}
		ok := resp.StatusCode >= 200 && resp.StatusCode < 300
		resp.Body.Close()
		cancel()
		if ok {
			return true
		}
	}
	return false
}

// scraperServiceReady reports whether the optional Analytics scraper is reachable.
// When REDDIT_PROVIDER=off, LSF auto-enables via scraper once this returns true.
func (h *Handler) scraperServiceReady(ctx context.Context) bool {
	if h.scraperAPIKey == "" {
		return false
	}
	h.scraperReadyMu.RLock()
	if !h.scraperReadyAt.IsZero() && time.Since(h.scraperReadyAt) < scraperReadyCacheTTL {
		ready := h.scraperReadyCached
		h.scraperReadyMu.RUnlock()
		return ready
	}
	h.scraperReadyMu.RUnlock()

	ready := h.probeServiceHealth(ctx, scraperHealthURL(h.scraperAPIURL))

	h.scraperReadyMu.Lock()
	h.scraperReadyAt = time.Now()
	h.scraperReadyCached = ready
	h.scraperReadyMu.Unlock()
	return ready
}
