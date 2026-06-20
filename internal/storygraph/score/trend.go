package score

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"streamclone/internal/social"
	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/store"
)

const trendWindow = time.Hour
const commentSpikeWindow = 6 * time.Hour

// TrendSampler writes trend_snapshots from evidence arrival rate and optional
// social metric refresh (when sources implement MetricRefresher).
type TrendSampler struct {
	enabled bool
	st      *store.Store
	rel     *reliability.Registry
	engine  *Engine
	sources map[string]social.SocialSource
}

func NewTrendSampler(enabled bool, st *store.Store, rel *reliability.Registry, sources map[string]social.SocialSource) *TrendSampler {
	if sources == nil {
		sources = map[string]social.SocialSource{}
	}
	return &TrendSampler{
		enabled: enabled,
		st:      st,
		rel:     rel,
		engine:  New(rel),
		sources: sources,
	}
}

// Sample records a trend snapshot for clusterID and refreshes story_scores when
// enough history exists.
func (t *TrendSampler) Sample(ctx context.Context, clusterID int64) error {
	if !t.enabled || t.st == nil {
		return nil
	}
	since := time.Now().Add(-trendWindow)
	weighted, err := t.st.EvidenceWeightSince(ctx, clusterID, since)
	if err != nil {
		return err
	}
	metricBoost, qualityFactors, err := t.metricBoost(ctx, clusterID)
	if err != nil {
		return err
	}
	trend := normalizeTrend(weighted + metricBoost)
	if err := t.st.InsertTrendSnapshot(ctx, clusterID, time.Now().UTC(), trend); err != nil {
		return err
	}
	if err := t.engine.RefreshTrendVolatility(ctx, t.st, clusterID); err != nil {
		return err
	}
	if len(qualityFactors) > 0 {
		return appendScoreFactors(ctx, t.st, clusterID, qualityFactors)
	}
	return nil
}

func normalizeTrend(raw float64) float64 {
	if raw < 0 {
		raw = 0
	}
	scaled := raw * 20 // four weighted arrivals/hour ≈ trend 80
	if scaled > 100 {
		return 100
	}
	return scaled
}

func (t *TrendSampler) metricBoost(ctx context.Context, clusterID int64) (float64, []string, error) {
	items, err := t.st.ClusterSocialItems(ctx, clusterID)
	if err != nil || len(items) == 0 {
		return 0, nil, err
	}
	bySource := map[string][]store.ClusterSocialItem{}
	for _, item := range items {
		bySource[item.Source] = append(bySource[item.Source], item)
	}
	var boost float64
	qualityFactors := []string{}
	for sourceName, group := range bySource {
		src, ok := t.sources[sourceName]
		if !ok {
			continue
		}
		refresher, ok := src.(social.MetricRefresher)
		if !ok {
			continue
		}
		if err := src.Healthy(ctx); err != nil {
			continue
		}
		ids := make([]string, 0, len(group))
		lookup := map[string]store.ClusterSocialItem{}
		for _, item := range group {
			ids = append(ids, item.ExternalID)
			lookup[item.ExternalID] = item
		}
		fresh, err := refresher.RefreshMetrics(ctx, ids)
		if err != nil || len(fresh) == 0 {
			continue
		}
		now := time.Now().UTC()
		for extID, metrics := range fresh {
			item, ok := lookup[extID]
			if !ok {
				continue
			}
			prev := parseMetrics(item.Metrics)
			if sourceName == "reddit" {
				if factor, ok := t.commentSpikeFactor(ctx, item, metrics, now); ok {
					qualityFactors = append(qualityFactors, factor)
				}
			}
			delta := metricDelta(prev, metrics)
			if delta <= 0 {
				t.recordMetricSnapshot(ctx, item, metrics, now)
				continue
			}
			weight := t.rel.Weight(sourceTypeForSocial(sourceName))
			boost += delta * weight
			updated, err := json.Marshal(metrics)
			if err != nil {
				continue
			}
			t.recordMetricSnapshot(ctx, item, metrics, now)
			_ = t.st.UpdateSocialItemMetrics(ctx, item.ID, updated)
		}
	}
	return boost, dedupeStrings(qualityFactors), nil
}

func (t *TrendSampler) recordMetricSnapshot(ctx context.Context, item store.ClusterSocialItem, metrics map[string]float64, at time.Time) {
	comments, hasComments := metricInt(metrics, "comments")
	var commentsPtr *int
	if hasComments {
		commentsPtr = &comments
	}
	raw, err := json.Marshal(metrics)
	if err != nil {
		return
	}
	_ = t.st.InsertSocialMetricSnapshot(ctx, item.ID, at, item.Source, item.ExternalID, raw, commentsPtr)
}

func (t *TrendSampler) commentSpikeFactor(ctx context.Context, item store.ClusterSocialItem, metrics map[string]float64, at time.Time) (string, bool) {
	current, ok := metricInt(metrics, "comments")
	if !ok {
		return "", false
	}
	history, err := t.st.ListSocialMetricSnapshots(ctx, item.ID, at.Add(-commentSpikeWindow), 12)
	if err != nil {
		return "", false
	}
	if suddenCommentRatio(history, current) < 3 {
		return "", false
	}
	return "sudden_comment_ratio:reddit", true
}

func suddenCommentRatio(history []store.SocialMetricSnapshot, current int) float64 {
	values := make([]int, 0, len(history))
	for _, snap := range history {
		if snap.Comments != nil && *snap.Comments > 0 {
			values = append(values, *snap.Comments)
		}
	}
	if len(values) < 2 || current < 20 {
		return 0
	}
	baseline := averageInts(values)
	if baseline < 5 {
		return 0
	}
	ratio := float64(current) / baseline
	if ratio >= 3 && current-values[len(values)-1] >= 15 {
		return ratio
	}
	return 0
}

func averageInts(values []int) float64 {
	var sum int
	for _, value := range values {
		sum += value
	}
	return float64(sum) / float64(len(values))
}

func metricInt(metrics map[string]float64, key string) (int, bool) {
	value, ok := metrics[key]
	if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return int(math.Round(value)), true
}

func appendScoreFactors(ctx context.Context, st *store.Store, clusterID int64, factors []string) error {
	existing, err := st.ScoresForCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	merged := mergeScoreFactors(existing.Factors, factors)
	if len(merged) == 0 {
		return nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	return st.UpsertScores(ctx, clusterID, store.Scores{
		Trend:      existing.Trend,
		Volatility: existing.Volatility,
		Confidence: existing.Confidence,
		Sentiment:  existing.Sentiment,
		Factors:    raw,
	})
}

func mergeScoreFactors(existing json.RawMessage, next []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	if len(existing) > 0 {
		var parsed []string
		if err := json.Unmarshal(existing, &parsed); err == nil {
			for _, factor := range parsed {
				factor = strings.TrimSpace(factor)
				if factor == "" {
					continue
				}
				if _, ok := seen[factor]; ok {
					continue
				}
				seen[factor] = struct{}{}
				out = append(out, factor)
			}
		}
	}
	for _, factor := range next {
		factor = strings.TrimSpace(factor)
		if factor == "" {
			continue
		}
		if _, ok := seen[factor]; ok {
			continue
		}
		seen[factor] = struct{}{}
		out = append(out, factor)
	}
	return out
}

func dedupeStrings(in []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range in {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func parseMetrics(raw json.RawMessage) map[string]float64 {
	if len(raw) == 0 {
		return map[string]float64{}
	}
	out := map[string]float64{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func metricDelta(prev, next map[string]float64) float64 {
	if len(next) == 0 {
		return 0
	}
	var delta float64
	for key, val := range next {
		old := prev[key]
		if diff := val - old; diff > 0 {
			switch key {
			case "score", "comments", "likes", "views":
				delta += diff
			default:
				delta += diff * 0.25
			}
		}
	}
	if delta > 0 {
		// compress large engagement spikes into trend window units
		return math.Log1p(delta)
	}
	return 0
}

func sourceTypeForSocial(source string) string {
	switch source {
	case "youtube":
		return "youtube_video"
	case "twitchclips", "twitch_clip":
		return "twitch_clip"
	case "reddit":
		return "reddit_thread"
	default:
		return source + "_thread"
	}
}
