package heatmap

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

type SignalWeights struct {
	ChatRate          float64 `json:"chatRate"`
	EmoteRate         float64 `json:"emoteRate"`
	ViewerMomentum    float64 `json:"viewerMomentum"`
	ProviderSpike     float64 `json:"providerSpike"`
	TopEmoteDominance float64 `json:"topEmoteDominance"`
	Novelty           float64 `json:"novelty"`
}

type ScoringConfig struct {
	Version                 string        `json:"version"`
	Weights                 SignalWeights `json:"weights"`
	DensityConfidenceWeight float64       `json:"densityConfidenceWeight"` // default 0.20; confidence-only weight, excluded from the score-weights sum
	SmoothingSpan           int           `json:"smoothingSpan"`           // default 3
	SmoothingAlpha          float64       `json:"smoothingAlpha"`          // default 0.5
	SuppressionThreshold    int           `json:"suppressionThreshold"`    // default 20
	SuppressionRadius       int           `json:"suppressionRadius"`       // default 3
	MaxPoints               int           `json:"maxPoints"`               // default 720
	TopRetainPercent        float64       `json:"topRetainPercent"`        // default 0.20
}

func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		Version: "v1",
		Weights: SignalWeights{
			ChatRate:          0.25,
			EmoteRate:         0.20,
			ViewerMomentum:    0.20,
			ProviderSpike:     0.15,
			TopEmoteDominance: 0.10,
			Novelty:           0.10,
		},
		DensityConfidenceWeight: 0.20,
		SmoothingSpan:           3,
		SmoothingAlpha:          0.5,
		SuppressionThreshold:    20,
		SuppressionRadius:       3,
		MaxPoints:               720,
		TopRetainPercent:        0.20,
	}
}

const weightSumTolerance = 0.001

func (c ScoringConfig) Validate() error {
	w := c.Weights
	sum := w.ChatRate + w.EmoteRate + w.ViewerMomentum +
		w.ProviderSpike + w.TopEmoteDominance + w.Novelty
	if math.Abs(sum-1.0) > weightSumTolerance {
		return fmt.Errorf("scoring weights sum to %.4f, must equal 1.0", sum)
	}
	return nil
}

func LoadScoringConfig() (ScoringConfig, error) {
	path := os.Getenv("HEATMAP_SCORING_CONFIG_PATH")
	if path == "" {
		return DefaultScoringConfig(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ScoringConfig{}, fmt.Errorf("load scoring config: %w", err)
	}
	var cfg ScoringConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ScoringConfig{}, fmt.Errorf("parse scoring config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return ScoringConfig{}, err
	}
	return cfg, nil
}
