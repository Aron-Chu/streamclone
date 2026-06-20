package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"
)

type setupDiagnosticsServices struct {
	Metadata  string `json:"metadata"`
	Chat      string `json:"chat"`
	Video     string `json:"video"`
	Emote     string `json:"emote"`
	Analytics string `json:"analytics"`
	Scraper   string `json:"scraper"`
	Clipper   string `json:"clipper"`
	Pulse     string `json:"pulse"`
}

type setupDiagnosticsResponse struct {
	Profile   string                   `json:"profile"`
	ImageTag  string                   `json:"imageTag"`
	Services  setupDiagnosticsServices `json:"services"`
	Healthy   bool                     `json:"healthy"`
	UpdatedAt int64                    `json:"updatedAt"`
}

func (h *Handler) setupDiagnostics(w http.ResponseWriter, r *http.Request) {
	profile := h.streamcloneProfile
	if profile == "" {
		profile = "core"
	}

	ctx, cancel := context.WithTimeout(r.Context(), setupProbeBudget)
	defer cancel()

	statuses := h.probeSetupServices(ctx, map[string]string{
		"chat":      "http://chat:8080/healthz",
		"video":     "http://video:8080/healthz",
		"emote":     "http://emote:8080/healthz",
		"analytics": "http://analytics:8080/healthz",
		"scraper":   scraperHealthURL(h.scraperAPIURL),
		"clipper":   h.clipperServiceURL + "/healthz",
	})
	services := setupDiagnosticsServices{
		Metadata:  "ready",
		Chat:      statuses["chat"],
		Video:     statuses["video"],
		Emote:     statuses["emote"],
		Analytics: statuses["analytics"],
		Scraper:   statuses["scraper"],
		Clipper:   statuses["clipper"],
		Pulse:     h.pulseServiceReady(ctx),
	}

	healthy := services.Chat == "ready" &&
		services.Video == "ready" &&
		services.Emote == "ready" &&
		services.Analytics == "ready"

	writeJSON(w, http.StatusOK, setupDiagnosticsResponse{
		Profile:   profile,
		ImageTag:  strings.TrimSpace(os.Getenv("IMAGE_TAG")),
		Services:  services,
		Healthy:   healthy,
		UpdatedAt: time.Now().Unix(),
	})
}
