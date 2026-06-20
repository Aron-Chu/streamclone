package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// WindowScore is the durable window-native ranking read model for one cluster.
type WindowScore struct {
	ClusterID         int64     `json:"clusterId,omitempty"`
	Window            string    `json:"window"`
	Since             time.Time `json:"since"`
	EvidenceCount     int       `json:"evidenceCount"`
	SourceCount       int       `json:"sourceCount"`
	VelocityScore     float64   `json:"velocityScore"`
	CredibilityScore  float64   `json:"credibilityScore"`
	ImpactScore       float64   `json:"impactScore"`
	MomentumScore     float64   `json:"momentumScore"`
	FreshnessScore    float64   `json:"freshnessScore"`
	RankScore         float64   `json:"rankScore"`
	DominantSource    string    `json:"dominantSource,omitempty"`
	RecentSourceDelta int       `json:"recentSourceDelta,omitempty"`
	ComputedAt        time.Time `json:"computedAt"`
	Status            string    `json:"status,omitempty"`
}

// ClusterWindowEvidence aggregates evidence inside a window for scoring.
type ClusterWindowEvidence struct {
	ClusterID       int64
	Category        string
	EvidenceCount   int
	SourceCount     int
	WeightedSum     float64
	LatestAt        time.Time
	DominantSource  string
	HasReddit       bool
	HasStreamerBans bool
	OnlyTwitch      bool
	Trend           *float64
}

func (s *Store) UpsertWindowScore(ctx context.Context, row WindowScore) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO story_window_scores
			(cluster_id, "window", since, evidence_count, source_count,
			 velocity_score, credibility_score, impact_score, momentum_score, freshness_score,
			 rank_score, dominant_source, computed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (cluster_id, "window") DO UPDATE SET
			since = EXCLUDED.since,
			evidence_count = EXCLUDED.evidence_count,
			source_count = EXCLUDED.source_count,
			velocity_score = EXCLUDED.velocity_score,
			credibility_score = EXCLUDED.credibility_score,
			impact_score = EXCLUDED.impact_score,
			momentum_score = EXCLUDED.momentum_score,
			freshness_score = EXCLUDED.freshness_score,
			rank_score = EXCLUDED.rank_score,
			dominant_source = EXCLUDED.dominant_source,
			computed_at = EXCLUDED.computed_at`,
		row.ClusterID, normalizeScoreWindow(row.Window), row.Since, row.EvidenceCount, row.SourceCount,
		row.VelocityScore, row.CredibilityScore, row.ImpactScore, row.MomentumScore, row.FreshnessScore,
		row.RankScore, nullIfEmpty(row.DominantSource), row.ComputedAt,
	)
	return err
}

func (s *Store) WindowScoreForCluster(ctx context.Context, clusterID int64, window string) (*WindowScore, error) {
	window = normalizeScoreWindow(window)
	var row WindowScore
	var dominant *string
	err := s.pool.QueryRow(ctx, `
		SELECT cluster_id, "window", since, evidence_count, source_count,
		       velocity_score, credibility_score, impact_score, momentum_score, freshness_score,
		       rank_score, dominant_source, computed_at
		FROM story_window_scores
		WHERE cluster_id = $1 AND "window" = $2`, clusterID, window).Scan(
		&row.ClusterID, &row.Window, &row.Since, &row.EvidenceCount, &row.SourceCount,
		&row.VelocityScore, &row.CredibilityScore, &row.ImpactScore, &row.MomentumScore, &row.FreshnessScore,
		&row.RankScore, &dominant, &row.ComputedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if dominant != nil {
		row.DominantSource = *dominant
	}
	row.Status = "ready"
	return &row, nil
}

func (s *Store) WindowScoresForClusters(ctx context.Context, clusterIDs []int64, window string) (map[int64]WindowScore, error) {
	out := map[int64]WindowScore{}
	if len(clusterIDs) == 0 {
		return out, nil
	}
	window = normalizeScoreWindow(window)
	rows, err := s.pool.Query(ctx, `
		SELECT cluster_id, "window", since, evidence_count, source_count,
		       velocity_score, credibility_score, impact_score, momentum_score, freshness_score,
		       rank_score, dominant_source, computed_at
		FROM story_window_scores
		WHERE "window" = $1 AND cluster_id = ANY($2)`, window, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row WindowScore
		var dominant *string
		if err := rows.Scan(
			&row.ClusterID, &row.Window, &row.Since, &row.EvidenceCount, &row.SourceCount,
			&row.VelocityScore, &row.CredibilityScore, &row.ImpactScore, &row.MomentumScore, &row.FreshnessScore,
			&row.RankScore, &dominant, &row.ComputedAt,
		); err != nil {
			return nil, err
		}
		if dominant != nil {
			row.DominantSource = *dominant
		}
		row.Status = "ready"
		out[row.ClusterID] = row
	}
	return out, rows.Err()
}

func (s *Store) RecentSourceDelta(ctx context.Context, clusterID int64, since, now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	recentSince := now.Add(-time.Hour)
	if recentSince.Before(since) {
		recentSince = since
	}
	var count int
	err := s.pool.QueryRow(ctx, `
		WITH first_seen AS (
			SELECT source_type, MIN(COALESCE(occurred_at, created_at)) AS first_at
			FROM story_evidence
			WHERE cluster_id = $1
			  AND COALESCE(occurred_at, created_at) >= $2
			GROUP BY source_type
		)
		SELECT COUNT(*)::int
		FROM first_seen
		WHERE first_at >= $3`, clusterID, since, recentSince).Scan(&count)
	return count, err
}

func (s *Store) LatestWindowScoreComputeAt(ctx context.Context, window string) (*time.Time, error) {
	window = normalizeScoreWindow(window)
	var at *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT MAX(computed_at) FROM story_window_scores WHERE "window" = $1`, window).Scan(&at)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return at, err
}

func (s *Store) ClusterWindowEvidence(ctx context.Context, clusterID int64, since time.Time) (*ClusterWindowEvidence, error) {
	var agg ClusterWindowEvidence
	agg.ClusterID = clusterID
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sc.category, ''),
		       COUNT(*)::int,
		       COUNT(DISTINCT se.source_type)::int,
		       COALESCE(SUM(se.weight), 0),
		       MAX(COALESCE(se.occurred_at, se.created_at)),
		       BOOL_OR(se.source_type = 'reddit_thread'),
		       BOOL_OR(se.source_type = 'streamerbans_post'),
		       BOOL_AND(se.source_type = 'twitch_clip')
		FROM story_evidence se
		JOIN story_clusters sc ON sc.id = se.cluster_id
		WHERE se.cluster_id = $1
		  AND COALESCE(se.occurred_at, se.created_at) >= $2
		GROUP BY sc.category`, clusterID, since).Scan(
		&agg.Category, &agg.EvidenceCount, &agg.SourceCount, &agg.WeightedSum, &agg.LatestAt,
		&agg.HasReddit, &agg.HasStreamerBans, &agg.OnlyTwitch,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if agg.EvidenceCount == 0 {
		return nil, nil
	}
	err = s.pool.QueryRow(ctx, `
		SELECT source_type
		FROM story_evidence
		WHERE cluster_id = $1
		  AND COALESCE(occurred_at, created_at) >= $2
		GROUP BY source_type
		ORDER BY COUNT(*) DESC
		LIMIT 1`, clusterID, since).Scan(&agg.DominantSource)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	var trend *float64
	_ = s.pool.QueryRow(ctx, `SELECT trend FROM story_scores WHERE cluster_id = $1`, clusterID).Scan(&trend)
	agg.Trend = trend
	return &agg, nil
}

func (s *Store) DeleteWindowScoresWithoutEvidence(ctx context.Context, window string, since time.Time) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM story_window_scores ws
		WHERE ws."window" = $1
		  AND NOT EXISTS (
		    SELECT 1 FROM story_evidence se
		    WHERE se.cluster_id = ws.cluster_id
		      AND COALESCE(se.occurred_at, se.created_at) >= $2
		  )`, normalizeScoreWindow(window), since)
	return err
}

func (s *Store) LastDirectorySampleAt(ctx context.Context) (*time.Time, error) {
	var at *time.Time
	err := s.pool.QueryRow(ctx, `SELECT MAX(sampled_at) FROM directory_samples`).Scan(&at)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return at, err
}

func (s *Store) DirectorySampleHistoryDays(ctx context.Context) (float64, error) {
	var oldest, newest *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT MIN(sampled_at), MAX(sampled_at) FROM directory_samples`).Scan(&oldest, &newest)
	if err == pgx.ErrNoRows || oldest == nil || newest == nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return newest.Sub(*oldest).Hours() / 24, nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// WindowLabelSince returns the canonical since timestamp for a window label at now (UTC).
func WindowLabelSince(window string, now time.Time) (time.Time, error) {
	window = strings.ToLower(strings.TrimSpace(window))
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	switch window {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), nil
	case "24h", "":
		return now.Add(-24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("invalid pulse wire window %q", window)
	}
}

func normalizeScoreWindow(window string) string {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "today", "7d":
		return strings.ToLower(strings.TrimSpace(window))
	default:
		return "24h"
	}
}

func scoreWindowSince(now time.Time, window string) time.Time {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	switch normalizeScoreWindow(window) {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "7d":
		return now.Add(-7 * 24 * time.Hour)
	default:
		return now.Add(-24 * time.Hour)
	}
}
