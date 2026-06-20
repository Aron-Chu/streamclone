package score

import (
	"time"

	"streamclone/internal/storygraph/windowmath"
)

const RankModelVersion = windowmath.RankModelVersion

// WindowScoreInput is evidence aggregated inside one ranking window.
type WindowScoreInput struct {
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

// WindowScoreOutput is the computed ranking breakdown for one cluster/window.
type WindowScoreOutput struct {
	VelocityScore    float64
	CredibilityScore float64
	ImpactScore      float64
	MomentumScore    float64
	FreshnessScore   float64
	RankScore        float64
}

// ComputeWindowScore derives window-native rank components and the final rank_score.
func ComputeWindowScore(in WindowScoreInput) WindowScoreOutput {
	out := windowmath.Compute(windowmath.Input{
		Window:          in.Window,
		Since:           in.Since,
		Now:             in.Now,
		EvidenceCount:   in.EvidenceCount,
		SourceCount:     in.SourceCount,
		WeightedSum:     in.WeightedSum,
		LatestAt:        in.LatestAt,
		DominantSource:  in.DominantSource,
		Category:        in.Category,
		Trend:           in.Trend,
		HasReddit:       in.HasReddit,
		HasStreamerBans: in.HasStreamerBans,
		OnlyTwitch:      in.OnlyTwitch,
	})
	return WindowScoreOutput{
		VelocityScore:    out.VelocityScore,
		CredibilityScore: out.CredibilityScore,
		ImpactScore:      out.ImpactScore,
		MomentumScore:    out.MomentumScore,
		FreshnessScore:   out.FreshnessScore,
		RankScore:        out.RankScore,
	}
}
