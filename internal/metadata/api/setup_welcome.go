package api

import (
	"context"
	"net/http"
	"strings"
	"time"
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
}

type setupWelcomeResponse struct {
	Profile       string               `json:"profile"`
	Services      setupWelcomeServices `json:"services"`
	Incomplete    bool                 `json:"incomplete"`
	ShowWelcome   bool                 `json:"showWelcome"`
	SetupGuideURL string               `json:"setupGuideUrl"`
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

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	services := setupWelcomeServices{
		Scraper: serviceStatus(h.probeServiceHealth(ctx, scraperHealthURL(h.scraperAPIURL))),
		Clipper: serviceStatus(h.probeServiceHealth(ctx, h.clipperServiceURL+"/v1/twitch/status")),
	}

	incomplete := false
	if profileHasScraper(profile) && services.Scraper == "offline" {
		incomplete = true
	}
	if profileHasClipper(profile) && services.Clipper == "offline" {
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

func (h *Handler) probeServiceHealth(ctx context.Context, rawURL string) bool {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(250 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			continue
		}
		resp, err := h.http.Do(req)
		if err != nil {
			continue
		}
		ok := resp.StatusCode >= 200 && resp.StatusCode < 300
		resp.Body.Close()
		if ok {
			return true
		}
	}
	return false
}
