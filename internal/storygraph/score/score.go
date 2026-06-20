package score

import (
	"context"
	"math"

	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/store"
)

// Engine computes Phase 1 confidence and Phase 2 trend/volatility from evidence.
type Engine struct {
	rel *reliability.Registry
}

func New(rel *reliability.Registry) *Engine {
	return &Engine{rel: rel}
}

// ComputeVolatility returns the mean absolute delta between consecutive trend
// samples on a 0..100 scale. Returns nil when fewer than two samples exist.
func ComputeVolatility(trends []float64) *float64 {
	if len(trends) < 2 {
		return nil
	}
	var sum float64
	for i := 1; i < len(trends); i++ {
		sum += math.Abs(trends[i] - trends[i-1])
	}
	v := sum / float64(len(trends)-1)
	return &v
}

// RefreshConfidence updates story_scores.confidence from evidence counts.
func (e *Engine) RefreshConfidence(ctx context.Context, st *store.Store, clusterID int64) error {
	receipts, err := st.ReceiptsForCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	conf := "single_source"
	if len(receipts) >= 2 {
		conf = "corroborated"
	}
	if len(receipts) >= 4 {
		conf = "widely_reported"
	}
	existing, err := st.ScoresForCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	c := conf
	return st.UpsertScores(ctx, clusterID, store.Scores{
		Trend:      existing.Trend,
		Volatility: existing.Volatility,
		Confidence: &c,
		Sentiment:  nil,
		Factors:    existing.Factors,
	})
}

// RefreshTrendVolatility reads recent trend_snapshots and updates story_scores
// trend and volatility. Both stay null until at least two snapshots exist.
func (e *Engine) RefreshTrendVolatility(ctx context.Context, st *store.Store, clusterID int64) error {
	snaps, err := st.ListTrendSnapshots(ctx, clusterID, 24)
	if err != nil {
		return err
	}
	trends := make([]float64, 0, len(snaps))
	for _, snap := range snaps {
		trends = append(trends, snap.Trend)
	}
	if len(trends) < 2 {
		return nil
	}
	latest := trends[len(trends)-1]
	vol := ComputeVolatility(trends)
	existing, err := st.ScoresForCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	return st.UpsertScores(ctx, clusterID, store.Scores{
		Trend:      &latest,
		Volatility: vol,
		Confidence: existing.Confidence,
		Sentiment:  nil,
		Factors:    existing.Factors,
	})
}
