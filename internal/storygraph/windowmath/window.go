package windowmath

import (
	"math"
	"strings"
	"time"
)

// RankModelVersion is the active window-native ranking model identifier.
const RankModelVersion = "window-native-v1"

// Input is evidence aggregated inside one ranking window.
type Input struct {
	Window          string
	Since           time.Time
	Now             time.Time
	EvidenceCount   int
	SourceCount     int
	WeightedSum     float64
	LatestAt        time.Time
	DominantSource  string
	Category        string
	Trend           *float64
	HasReddit       bool
	HasStreamerBans bool
	OnlyTwitch      bool
}

// Output is the computed ranking breakdown for one cluster/window.
type Output struct {
	VelocityScore    float64
	CredibilityScore float64
	ImpactScore      float64
	MomentumScore    float64
	FreshnessScore   float64
	RankScore        float64
}

type windowWeights struct {
	velocity    float64
	credibility float64
	impact      float64
	momentum    float64
	freshness   float64
	diversity   float64
}

var profiles = map[string]windowWeights{
	"today": {0.35, 0.15, 0.15, 0.15, 0.20, 0.10},
	"24h":   {0.25, 0.25, 0.20, 0.10, 0.10, 0.10},
	"7d":    {0.10, 0.20, 0.30, 0.15, 0.05, 0.20},
}

// Compute derives window-native rank components and the final rank_score.
func Compute(in Input) Output {
	window := strings.ToLower(strings.TrimSpace(in.Window))
	if window == "" {
		window = "24h"
	}
	w := profiles[window]
	if w == (windowWeights{}) {
		w = profiles["24h"]
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	windowHours := windowDurationHours(window, in.Since, now)
	if windowHours < 1 {
		windowHours = 1
	}

	velocity := clamp01(float64(in.EvidenceCount) / windowHours / 4)
	credibility := clamp01(in.WeightedSum / float64(maxInt(in.EvidenceCount, 1)) / 1.2)
	if in.HasReddit {
		credibility = clamp01(credibility + 0.12)
	}
	if in.HasStreamerBans {
		credibility = clamp01(credibility + 0.15)
	}
	if in.SourceCount >= 2 {
		credibility = clamp01(credibility + 0.10)
	}

	diversity := clamp01(float64(in.SourceCount) / 4)
	impact := clamp01((float64(in.EvidenceCount)*0.35 + diversity*0.65) * categoryImpactBoost(in.Category))
	momentum := 0.0
	if in.Trend != nil {
		momentum = clamp01(*in.Trend / 100)
	}

	freshness := 0.0
	if !in.LatestAt.IsZero() {
		ageHours := now.Sub(in.LatestAt).Hours()
		switch window {
		case "today":
			freshness = clamp01(1 - ageHours/12)
		case "7d":
			freshness = clamp01(1 - ageHours/(7*24))
		default:
			freshness = clamp01(1 - ageHours/24)
		}
	}

	rank := velocity*w.velocity +
		credibility*w.credibility +
		impact*w.impact +
		momentum*w.momentum +
		freshness*w.freshness +
		diversity*w.diversity

	if in.OnlyTwitch && in.SourceCount <= 1 {
		rank *= 0.88
	}
	if window == "today" {
		rank += categoryUrgencyBoost(in.Category)
	}
	rank = clamp01(rank)

	return Output{
		VelocityScore:    roundScore(velocity * 100),
		CredibilityScore: roundScore(credibility * 100),
		ImpactScore:      roundScore(impact * 100),
		MomentumScore:    roundScore(momentum * 100),
		FreshnessScore:   roundScore(freshness * 100),
		RankScore:        roundScore(rank * 100),
	}
}

func windowDurationHours(window string, since, now time.Time) float64 {
	if !since.IsZero() {
		return now.Sub(since).Hours()
	}
	switch window {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return now.Sub(start).Hours()
	case "7d":
		return 7 * 24
	default:
		return 24
	}
}

func categoryImpactBoost(category string) float64 {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "bans", "drama", "news":
		return 1.15
	case "funny":
		return 1.05
	default:
		return 1.0
	}
}

func categoryUrgencyBoost(category string) float64 {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "bans":
		return 0.08
	case "drama", "news":
		return 0.05
	default:
		return 0
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundScore(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
