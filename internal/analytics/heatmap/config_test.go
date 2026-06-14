package heatmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultScoringConfigValid(t *testing.T) {
	cfg := DefaultScoringConfig()
	if cfg.Version != "v1" {
		t.Fatalf("version = %q, want v1", cfg.Version)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if cfg.DensityConfidenceWeight != 0.20 {
		t.Errorf("DensityConfidenceWeight = %v, want 0.20", cfg.DensityConfidenceWeight)
	}
	if cfg.SmoothingSpan != 3 || cfg.SmoothingAlpha != 0.5 {
		t.Errorf("smoothing = (%d, %v), want (3, 0.5)", cfg.SmoothingSpan, cfg.SmoothingAlpha)
	}
	if cfg.SuppressionThreshold != 20 || cfg.SuppressionRadius != 3 {
		t.Errorf("suppression = (%d, %d), want (20, 3)", cfg.SuppressionThreshold, cfg.SuppressionRadius)
	}
	if cfg.MaxPoints != 720 || cfg.TopRetainPercent != 0.20 {
		t.Errorf("decimation = (%d, %v), want (720, 0.20)", cfg.MaxPoints, cfg.TopRetainPercent)
	}
}

func TestValidateWeightSum(t *testing.T) {
	// DensityConfidenceWeight is excluded from the score-weights sum.
	cfg := DefaultScoringConfig()
	cfg.DensityConfidenceWeight = 0.99
	if err := cfg.Validate(); err != nil {
		t.Fatalf("density weight must not affect score-weights sum: %v", err)
	}

	// Within tolerance still valid.
	cfg = DefaultScoringConfig()
	cfg.Weights.Novelty += 0.0005
	if err := cfg.Validate(); err != nil {
		t.Errorf("within-tolerance sum rejected: %v", err)
	}

	// Outside tolerance rejected.
	cfg = DefaultScoringConfig()
	cfg.Weights.Novelty += 0.05
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for weights not summing to 1.0")
	}
}

func TestLoadScoringConfigDefault(t *testing.T) {
	t.Setenv("HEATMAP_SCORING_CONFIG_PATH", "")
	cfg, err := LoadScoringConfig()
	if err != nil {
		t.Fatalf("LoadScoringConfig: %v", err)
	}
	if cfg.Version != "v1" {
		t.Errorf("version = %q, want v1 (default)", cfg.Version)
	}
}

func TestLoadScoringConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scoring.json")
	const body = `{
		"version": "v2",
		"weights": {
			"chatRate": 0.30,
			"emoteRate": 0.20,
			"viewerMomentum": 0.20,
			"providerSpike": 0.10,
			"topEmoteDominance": 0.10,
			"novelty": 0.10
		},
		"densityConfidenceWeight": 0.25,
		"smoothingSpan": 5,
		"smoothingAlpha": 0.4,
		"suppressionThreshold": 25,
		"suppressionRadius": 4,
		"maxPoints": 600,
		"topRetainPercent": 0.15
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("HEATMAP_SCORING_CONFIG_PATH", path)

	cfg, err := LoadScoringConfig()
	if err != nil {
		t.Fatalf("LoadScoringConfig: %v", err)
	}
	if cfg.Version != "v2" {
		t.Errorf("version = %q, want v2", cfg.Version)
	}
	if cfg.Weights.ChatRate != 0.30 {
		t.Errorf("chatRate = %v, want 0.30", cfg.Weights.ChatRate)
	}
	if cfg.MaxPoints != 600 {
		t.Errorf("maxPoints = %d, want 600", cfg.MaxPoints)
	}
}

func TestLoadScoringConfigRejectsInvalidSum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	const body = `{
		"version": "bad",
		"weights": {
			"chatRate": 0.50,
			"emoteRate": 0.20,
			"viewerMomentum": 0.20,
			"providerSpike": 0.15,
			"topEmoteDominance": 0.10,
			"novelty": 0.10
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("HEATMAP_SCORING_CONFIG_PATH", path)

	if _, err := LoadScoringConfig(); err == nil {
		t.Error("expected error for invalid weight sum in config file")
	}
}

func TestLoadScoringConfigMissingFile(t *testing.T) {
	t.Setenv("HEATMAP_SCORING_CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.json"))
	if _, err := LoadScoringConfig(); err == nil {
		t.Error("expected error for missing config file")
	}
}
