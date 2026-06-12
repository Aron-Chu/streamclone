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

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	services := setupDiagnosticsServices{
		Metadata:  "ready",
		Chat:      serviceStatus(h.probeServiceHealth(ctx, "http://chat:8080/healthz")),
		Video:     serviceStatus(h.probeServiceHealth(ctx, "http://video:8080/healthz")),
		Emote:     serviceStatus(h.probeServiceHealth(ctx, "http://emote:8080/healthz")),
		Analytics: serviceStatus(h.probeServiceHealth(ctx, "http://analytics:8080/healthz")),
		Scraper:   serviceStatus(h.probeServiceHealth(ctx, scraperHealthURL(h.scraperAPIURL))),
		Clipper:   serviceStatus(h.probeServiceHealth(ctx, h.clipperServiceURL+"/v1/twitch/status")),
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
