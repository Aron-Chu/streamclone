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
	if cfg.EmoteDictionaryTTL != 24*time.Hour {
		t.Fatalf("EmoteDictionaryTTL = %s, want 24h", cfg.EmoteDictionaryTTL)
	}
	if cfg.EmoteDictionaryLegacyJitter != 24*time.Hour {
		t.Fatalf("EmoteDictionaryLegacyJitter = %s, want 24h", cfg.EmoteDictionaryLegacyJitter)
	}
	if cfg.EmoteDictionaryLegacyBatchSize != 100 {
		t.Fatalf("EmoteDictionaryLegacyBatchSize = %d, want 100", cfg.EmoteDictionaryLegacyBatchSize)
	}
	if cfg.EmoteDictionaryLegacyBatchPause != 50*time.Millisecond {
		t.Fatalf("EmoteDictionaryLegacyBatchPause = %s, want 50ms", cfg.EmoteDictionaryLegacyBatchPause)
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

func TestValidateEmoteObjectStorage(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "local only", cfg: Config{}},
		{
			name: "secondary complete",
			cfg: Config{
				EmoteObjectSecondaryEnabled:   true,
				EmoteObjectSecondaryEndpoint:  "https://objects.example.test",
				EmoteObjectSecondaryBucket:    "emotes",
				EmoteObjectSecondaryAccessKey: "access",
				EmoteObjectSecondarySecretKey: "secret",
				EmoteObjectDualWrite:          true,
			},
		},
		{
			name:    "dual write without secondary",
			cfg:     Config{EmoteObjectDualWrite: true},
			wantErr: true,
		},
		{
			name: "secondary missing credentials",
			cfg: Config{
				EmoteObjectSecondaryEnabled:  true,
				EmoteObjectSecondaryEndpoint: "https://objects.example.test",
				EmoteObjectSecondaryBucket:   "emotes",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateEmoteObjectStorage()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateEmoteObjectStorage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
