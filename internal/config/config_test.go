package config

import (
	"testing"
	"time"
)

func TestLoadCoreDefaults(t *testing.T) {
	t.Setenv("SCRAPER_API_URL", "")
	t.Setenv("FIRECRAWL_API_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ScraperAPIURL != defaultScraperAPIURL {
		t.Fatalf("ScraperAPIURL = %q, want %q", cfg.ScraperAPIURL, defaultScraperAPIURL)
	}
	if cfg.StreamcloneProfile != "core" {
		t.Fatalf("StreamcloneProfile = %q, want core", cfg.StreamcloneProfile)
	}
	if cfg.MetaCacheTTL != 30*time.Second {
		t.Fatalf("MetaCacheTTL = %s, want 30s", cfg.MetaCacheTTL)
	}
	if cfg.EmoteRenderOnChatObserved != true {
		t.Fatal("EmoteRenderOnChatObserved default = false, want true")
	}
}

func TestLoadScraperFirecrawlAlias(t *testing.T) {
	t.Setenv("SCRAPER_API_URL", "")
	t.Setenv("FIRECRAWL_API_URL", "http://custom-scraper:9000")
	t.Setenv("FIRECRAWL_API_KEY", "secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ScraperAPIURL != "http://custom-scraper:9000" {
		t.Fatalf("ScraperAPIURL = %q", cfg.ScraperAPIURL)
	}
	if cfg.ScraperAPIKey != "secret" {
		t.Fatalf("ScraperAPIKey = %q", cfg.ScraperAPIKey)
	}
}
