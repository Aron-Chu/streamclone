package store

import (
	"context"
	"encoding/json"
)

// ClipThumbRepairRow is a twitch clip social item missing Helix thumbnail metadata.
type ClipThumbRepairRow struct {
	ID         int64
	ExternalID string
	Metrics    json.RawMessage
}

// ListClipsNeedingThumbnailRepair returns clip rows that lack ready Helix thumbnails.
func (s *Store) ListClipsNeedingThumbnailRepair(ctx context.Context, limit int) ([]ClipThumbRepairRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, external_id, metrics
		FROM social_items
		WHERE source = 'twitch_clip'
		  AND kind = 'clip'
		  AND (
		    COALESCE(metrics->>'thumbnail_url', '') = ''
		    OR COALESCE(metrics->>'thumbnail_source', '') <> 'helix'
		    OR COALESCE(metrics->>'thumbnail_status', '') <> 'ready'
		  )
		ORDER BY COALESCE(created_at_src, ingested_at) DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClipThumbRepairRow
	for rows.Next() {
		var row ClipThumbRepairRow
		if err := rows.Scan(&row.ID, &row.ExternalID, &row.Metrics); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// RedditThumbRepairRow is a reddit post missing preview metadata.
type RedditThumbRepairRow struct {
	ID  int64
	URL string
}

// ListRedditNeedingThumbnailRepair returns reddit posts without stored thumbnails.
func (s *Store) ListRedditNeedingThumbnailRepair(ctx context.Context, limit int) ([]RedditThumbRepairRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT si.id, si.url
		FROM social_items si
		WHERE si.source = 'reddit'
		  AND si.kind = 'post'
		  AND COALESCE(si.metrics->>'thumbnail_url', '') = ''
		ORDER BY COALESCE(si.created_at_src, si.ingested_at) DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RedditThumbRepairRow
	for rows.Next() {
		var row RedditThumbRepairRow
		if err := rows.Scan(&row.ID, &row.URL); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// RedditMetricsRepairRow is a reddit post missing engagement or body metadata.
type RedditMetricsRepairRow struct {
	ID      int64
	URL     string
	Metrics json.RawMessage
}

// ListRedditNeedingMetricsRepair returns reddit posts with zero score or comments.
func (s *Store) ListRedditNeedingMetricsRepair(ctx context.Context, limit int) ([]RedditMetricsRepairRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT si.id, si.url, si.metrics
		FROM social_items si
		WHERE si.source = 'reddit'
		  AND si.kind = 'post'
		  AND (
		    COALESCE(si.metrics->>'score', '0') IN ('0', '0.0', '')
		    OR COALESCE(si.metrics->>'comments', '0') IN ('0', '0.0', '')
		    OR COALESCE(si.metrics->>'selftext', '') = ''
		  )
		ORDER BY COALESCE(si.created_at_src, si.ingested_at) DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RedditMetricsRepairRow
	for rows.Next() {
		var row RedditMetricsRepairRow
		if err := rows.Scan(&row.ID, &row.URL, &row.Metrics); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CountRedditNeedingThumbnailRepair counts reddit posts missing thumbnails.
func (s *Store) CountRedditNeedingThumbnailRepair(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM social_items
		WHERE source = 'reddit'
		  AND kind = 'post'
		  AND COALESCE(metrics->>'thumbnail_url', '') = ''`).Scan(&count)
	return count, err
}
