package ingest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"streamclone/internal/config"
)

// ScraperReady reports whether the shared scraper service responds healthy.
// Analytics and Pulse Wire ingest share this service; callers should defer
// heavy browser scrapes when it is unavailable.
func ScraperReady(ctx context.Context, cfg config.Config) bool {
	base := strings.TrimSpace(cfg.ScraperAPIURL)
	if base == "" {
		return false
	}
	healthURL := strings.TrimRight(base, "/")
	if strings.HasSuffix(healthURL, "/v2/scrape") {
		healthURL = strings.TrimSuffix(healthURL, "/v2/scrape")
	}
	healthURL += "/health"

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
