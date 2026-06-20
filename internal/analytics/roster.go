package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TrackedStreamer is one row in tracked_streamers.
type TrackedStreamer struct {
	TwitchUserID    string
	Login           string
	DisplayName     string
	PriorityTier    string
	LastSeenLiveAt  *time.Time
	LastRank        int
	IsAlwaysTracked bool
	ArchivePolicy   string
}

type metadataStreamItem struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	Viewers     int    `json:"viewers"`
}

type metadataStreamsResponse struct {
	Items []metadataStreamItem `json:"items"`
}

// RosterSyncer refreshes the Tier-0 tracked streamer roster from metadata top streams.
type RosterSyncer struct {
	db              *pgxpool.Pool
	metadataURL     string
	httpClient      *http.Client
	topN            int
	alwaysTracked   map[string]bool
	archiveExporter rosterArchiveExporter
}

type rosterArchiveExporter interface {
	ExportTopRoster(ctx context.Context, payload []byte) error
}

func NewRosterSyncer(db *pgxpool.Pool, metadataURL string, topN int, always []string) *RosterSyncer {
	if topN <= 0 {
		topN = 200
	}
	tracked := map[string]bool{}
	for _, login := range always {
		if login = normalizeLogin(login); login != "" {
			tracked[login] = true
		}
	}
	return &RosterSyncer{
		db:            db,
		metadataURL:   strings.TrimRight(metadataURL, "/"),
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		topN:          topN,
		alwaysTracked: tracked,
	}
}

func (r *RosterSyncer) WithArchiveExporter(exporter rosterArchiveExporter) *RosterSyncer {
	if r != nil {
		r.archiveExporter = exporter
	}
	return r
}

func (r *RosterSyncer) SyncOnce(ctx context.Context) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("roster syncer unavailable")
	}
	items, err := r.fetchTopStreams(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for rank, item := range items {
		login := normalizeLogin(item.Login)
		if login == "" {
			continue
		}
		tier := tierForRank(rank + 1)
		if r.alwaysTracked[login] {
			tier = "P0"
		}
		_, err := r.db.Exec(ctx, `
			INSERT INTO tracked_streamers (
				twitch_user_id, login, display_name, priority_tier, last_seen_live_at,
				last_rank, is_always_tracked, archive_policy, updated_at
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'standard',now())
			ON CONFLICT (login) DO UPDATE SET
				twitch_user_id = COALESCE(NULLIF(EXCLUDED.twitch_user_id,''), tracked_streamers.twitch_user_id),
				display_name = COALESCE(NULLIF(EXCLUDED.display_name,''), tracked_streamers.display_name),
				priority_tier = CASE
					WHEN tracked_streamers.is_always_tracked THEN 'P0'
					WHEN EXCLUDED.priority_tier < tracked_streamers.priority_tier THEN EXCLUDED.priority_tier
					ELSE tracked_streamers.priority_tier
				END,
				last_seen_live_at = EXCLUDED.last_seen_live_at,
				last_rank = EXCLUDED.last_rank,
				is_always_tracked = tracked_streamers.is_always_tracked OR EXCLUDED.is_always_tracked,
				updated_at = now()`,
			strings.TrimSpace(item.ID), login, strings.TrimSpace(item.DisplayName), tier, now, rank+1, r.alwaysTracked[login],
		)
		if err != nil {
			return err
		}
	}
	for login := range r.alwaysTracked {
		_, err := r.db.Exec(ctx, `
			INSERT INTO tracked_streamers (login, priority_tier, is_always_tracked, archive_policy, updated_at)
			VALUES ($1,'P0',true,'standard',now())
			ON CONFLICT (login) DO UPDATE SET
				priority_tier='P0', is_always_tracked=true, updated_at=now()`, login)
		if err != nil {
			return err
		}
	}
	if r.archiveExporter != nil {
		payload, _ := json.Marshal(map[string]any{
			"updatedAt": now,
			"topN":      r.topN,
			"items":     items,
		})
		_ = r.archiveExporter.ExportTopRoster(ctx, payload)
	}
	return nil
}

func tierForRank(rank int) string {
	switch {
	case rank <= 50:
		return "P1"
	case rank <= 200:
		return "P2"
	default:
		return "P3"
	}
}

func (r *RosterSyncer) fetchTopStreams(ctx context.Context) ([]metadataStreamItem, error) {
	if r.metadataURL == "" {
		return nil, fmt.Errorf("metadata service url not configured")
	}
	url := fmt.Sprintf("%s/v1/streams?limit=%d", r.metadataURL, r.topN)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("metadata streams status %d", resp.StatusCode)
	}
	var page metadataStreamsResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (r *RosterSyncer) ListLiveTracked(ctx context.Context, limit int) ([]TrackedStreamer, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.Query(ctx, `
		SELECT twitch_user_id, login, display_name, priority_tier, last_seen_live_at,
			COALESCE(last_rank, 0), is_always_tracked, archive_policy
		FROM tracked_streamers
		WHERE last_seen_live_at IS NOT NULL
		  AND last_seen_live_at > now() - interval '15 minutes'
		ORDER BY last_rank ASC NULLS LAST, login ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackedStreamer
	for rows.Next() {
		var row TrackedStreamer
		if err := rows.Scan(
			&row.TwitchUserID, &row.Login, &row.DisplayName, &row.PriorityTier, &row.LastSeenLiveAt,
			&row.LastRank, &row.IsAlwaysTracked, &row.ArchivePolicy,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func StartRosterWorker(ctx context.Context, syncer *RosterSyncer, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if syncer == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func() {
			if err := syncer.SyncOnce(ctx); err != nil {
				if log != nil {
					log.Warn("tier-0 roster sync failed", "err", err)
				}
				return
			}
			if log != nil {
				log.Info("tier-0 roster sync completed")
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
