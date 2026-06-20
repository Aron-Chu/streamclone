package store

import (
	"context"
	"strings"
	"time"
)

// BanEvent is a normalized platform ban/unban record.
type BanEvent struct {
	ID              int64      `json:"id"`
	StreamerLogin   string     `json:"streamerLogin"`
	DisplayName     string     `json:"streamerDisplayName,omitempty"`
	EventType       string     `json:"eventType"`
	Platform        string     `json:"platform"`
	Source          string     `json:"source"`
	SourceItemID    *int64     `json:"sourceItemId,omitempty"`
	Headline        string     `json:"headline"`
	SourceURL       string     `json:"sourceUrl,omitempty"`
	OccurredAt      time.Time  `json:"occurredAt"`
	Confidence      float64    `json:"confidence"`
	PreviewKind           string     `json:"previewKind,omitempty"`
	PreviewURL            string     `json:"previewUrl,omitempty"`
	ThumbnailURL          string     `json:"thumbnailUrl,omitempty"`
	DisplayThumbnailURL   string     `json:"displayThumbnailUrl,omitempty"`
}

// UpsertBanEvent inserts or updates a ban event keyed by source + social item.
func (s *Store) UpsertBanEvent(ctx context.Context, row BanEvent) error {
	if row.StreamerLogin == "" || row.Headline == "" || row.EventType == "" {
		return nil
	}
	if row.Platform == "" {
		row.Platform = "twitch"
	}
	if row.Confidence <= 0 {
		row.Confidence = 0.7
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO platform_ban_events (
			streamer_login, display_name, event_type, platform, source,
			source_item_id, headline, source_url, occurred_at, confidence
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (source, source_item_id) DO UPDATE SET
			streamer_login = EXCLUDED.streamer_login,
			display_name = EXCLUDED.display_name,
			event_type = EXCLUDED.event_type,
			headline = EXCLUDED.headline,
			source_url = EXCLUDED.source_url,
			occurred_at = EXCLUDED.occurred_at,
			confidence = EXCLUDED.confidence,
			ingested_at = now()`,
		row.StreamerLogin, nullIfEmpty(row.DisplayName), row.EventType, row.Platform, row.Source,
		row.SourceItemID, row.Headline, nullIfEmpty(row.SourceURL), row.OccurredAt, row.Confidence,
	)
	return err
}

// ListBanEvents returns ban/unban rows in a time window.
func (s *Store) ListBanEvents(ctx context.Context, since time.Time, limit int) ([]BanEvent, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.streamer_login, COALESCE(b.display_name, ''),
		       b.event_type, b.platform, b.source, b.source_item_id,
		       b.headline, COALESCE(b.source_url, ''), b.occurred_at, b.confidence,
		       COALESCE(si.metrics->>'thumbnail_url', '')
		FROM platform_ban_events b
		LEFT JOIN social_items si ON si.id = b.source_item_id
		WHERE b.occurred_at >= $1
		ORDER BY b.occurred_at DESC, b.id DESC
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BanEvent, 0, limit)
	for rows.Next() {
		var row BanEvent
		var sourceItemID *int64
		var thumb string
		if err := rows.Scan(
			&row.ID, &row.StreamerLogin, &row.DisplayName,
			&row.EventType, &row.Platform, &row.Source, &sourceItemID,
			&row.Headline, &row.SourceURL, &row.OccurredAt, &row.Confidence,
			&thumb,
		); err != nil {
			return nil, err
		}
		row.SourceItemID = sourceItemID
		metricsThumb := strings.TrimSpace(thumb)
		kind, raw, proxied := resolvePreview(metricsThumb, metricsThumb, row.Headline, row.SourceURL)
		row.PreviewKind = kind
		row.ThumbnailURL = raw
		row.PreviewURL = proxied
		row.DisplayThumbnailURL = firstNonEmpty(proxied, displayThumbnailURL(raw, metricsThumb))
		out = append(out, row)
	}
	if out == nil {
		out = []BanEvent{}
	}
	return out, rows.Err()
}
