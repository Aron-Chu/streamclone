package score

import (
	"context"
	"time"

	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/store"
)

// WindowEngine recomputes story_window_scores for today, 24h, and 7d.
type WindowEngine struct {
	store *store.Store
}

func NewWindowEngine(st *store.Store, _ *reliability.Registry) *WindowEngine {
	return &WindowEngine{store: st}
}

// RecomputeAll refreshes window scores for clusters with evidence in the last 7 days.
func (e *WindowEngine) RecomputeAll(ctx context.Context, now time.Time) error {
	if e.store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ids, err := e.store.ListSampleableClusterIDs(ctx)
	if err != nil {
		return err
	}
	windows := []string{"today", "24h", "7d"}
	for _, window := range windows {
		since, err := store.WindowLabelSince(window, now)
		if err != nil {
			return err
		}
		if err := e.store.DeleteWindowScoresWithoutEvidence(ctx, window, since); err != nil {
			return err
		}
		for _, clusterID := range ids {
			agg, err := e.store.ClusterWindowEvidence(ctx, clusterID, since)
			if err != nil {
				return err
			}
			if agg == nil || agg.EvidenceCount == 0 {
				continue
			}
			out := ComputeWindowScore(WindowScoreInput{
				Window:          window,
				Since:           since,
				Now:             now,
				EvidenceCount:   agg.EvidenceCount,
				SourceCount:     agg.SourceCount,
				WeightedSum:     agg.WeightedSum,
				LatestAt:        agg.LatestAt,
				DominantSource:  agg.DominantSource,
				Category:        agg.Category,
				Trend:           agg.Trend,
				HasReddit:       agg.HasReddit,
				HasStreamerBans: agg.HasStreamerBans,
				OnlyTwitch:      agg.OnlyTwitch,
			})
			if err := e.store.UpsertWindowScore(ctx, store.WindowScore{
				ClusterID:        clusterID,
				Window:           window,
				Since:            since,
				EvidenceCount:    agg.EvidenceCount,
				SourceCount:      agg.SourceCount,
				VelocityScore:    out.VelocityScore,
				CredibilityScore: out.CredibilityScore,
				ImpactScore:      out.ImpactScore,
				MomentumScore:    out.MomentumScore,
				FreshnessScore:   out.FreshnessScore,
				RankScore:        out.RankScore,
				DominantSource:   agg.DominantSource,
				ComputedAt:       now,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
