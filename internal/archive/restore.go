package archive

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RestoreResult summarizes a restore operation.
type RestoreResult struct {
	StreamID     string
	RollupCount  int
	StreamLoaded bool
}

// Restorer rebuilds analytics tables from archived blobs.
type Restorer struct {
	blob BlobStore
	db   *pgxpool.Pool
}

func NewRestorer(blob BlobStore, db *pgxpool.Pool) *Restorer {
	return &Restorer{blob: blob, db: db}
}

func (r *Restorer) RestoreStream(ctx context.Context, streamID string) (RestoreResult, error) {
	if r == nil || r.blob == nil || r.db == nil {
		return RestoreResult{}, fmt.Errorf("archive restorer is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return RestoreResult{}, fmt.Errorf("stream id is required")
	}
	result := RestoreResult{StreamID: streamID}

	sessionRaw, err := r.blob.Get(ctx, StreamSessionBlobKey(streamID))
	if err != nil {
		return result, fmt.Errorf("fetch stream session blob: %w", err)
	}
	sessionJSON, err := Gunzip(sessionRaw)
	if err != nil {
		return result, fmt.Errorf("gunzip stream session: %w", err)
	}
	var session StreamExportData
	if err := json.Unmarshal(sessionJSON, &session); err != nil {
		return result, fmt.Errorf("parse stream session: %w", err)
	}
	if err := r.upsertStream(ctx, session); err != nil {
		return result, err
	}
	result.StreamLoaded = true

	rollupRaw, err := r.blob.Get(ctx, RollupsBlobKey(streamID))
	if err != nil {
		return result, fmt.Errorf("fetch rollups blob: %w", err)
	}
	rollupJSONL, err := Gunzip(rollupRaw)
	if err != nil {
		return result, fmt.Errorf("gunzip rollups: %w", err)
	}
	count, err := r.upsertRollups(ctx, streamID, rollupJSONL)
	if err != nil {
		return result, err
	}
	result.RollupCount = count
	return result, nil
}

func (r *Restorer) upsertStream(ctx context.Context, session StreamExportData) error {
	canonical := session.CanonicalStreamID
	if canonical == "" {
		canonical = session.StreamID
	}
	viewerSource := session.ViewerSource
	if viewerSource == "" {
		viewerSource = "restored"
	}
	lastSeen := session.StartedAt
	if session.EndedAt != nil && !session.EndedAt.IsZero() {
		lastSeen = session.EndedAt.UTC()
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category,
			started_at, ended_at, last_seen_at, vod_id, viewer_source, canonical_stream_id,
			avg_viewers, peak_viewers, viewer_samples, tags
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'[]'::jsonb)
		ON CONFLICT (stream_id) DO UPDATE SET
			broadcaster_id = COALESCE(NULLIF(EXCLUDED.broadcaster_id,''), analytics_streams.broadcaster_id),
			login = EXCLUDED.login,
			display_name = COALESCE(NULLIF(EXCLUDED.display_name,''), analytics_streams.display_name),
			title = COALESCE(NULLIF(EXCLUDED.title,''), analytics_streams.title),
			category = COALESCE(NULLIF(EXCLUDED.category,''), analytics_streams.category),
			started_at = EXCLUDED.started_at,
			ended_at = COALESCE(EXCLUDED.ended_at, analytics_streams.ended_at),
			vod_id = COALESCE(NULLIF(EXCLUDED.vod_id,''), analytics_streams.vod_id),
			viewer_source = EXCLUDED.viewer_source,
			canonical_stream_id = EXCLUDED.canonical_stream_id,
			avg_viewers = GREATEST(analytics_streams.avg_viewers, EXCLUDED.avg_viewers),
			peak_viewers = GREATEST(analytics_streams.peak_viewers, EXCLUDED.peak_viewers),
			viewer_samples = GREATEST(analytics_streams.viewer_samples, EXCLUDED.viewer_samples),
			updated_at = now()`,
		session.StreamID, session.BroadcasterID, normalizeLogin(session.Login),
		session.DisplayName, session.Title, session.Category,
		session.StartedAt, session.EndedAt, lastSeen, session.VodID, viewerSource, canonical,
		session.AvgViewers, session.PeakViewers, session.ViewerSamples,
	)
	return err
}

func (r *Restorer) upsertRollups(ctx context.Context, streamID string, jsonl []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(jsonl))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var rollup RollupExportLine
		if err := json.Unmarshal(line, &rollup); err != nil {
			return count, err
		}
		emotes, _ := json.Marshal(rollup.Emotes)
		_, err := r.db.Exec(ctx, `
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				viewer_avg = CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
				viewer_max = GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
				viewer_latest = CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
				viewer_samples = GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
				chat_count = GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
				total_emote_count = GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count = GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json = EXCLUDED.emotes_json,
				updated_at = now()`,
			streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
			rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, scanner.Err()
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

// VerifyStream checks manifest + blob presence for a stream export.
func VerifyStream(ctx context.Context, manifest *ManifestStore, blob BlobStore, streamID string) error {
	if manifest == nil || blob == nil {
		return fmt.Errorf("verify: manifest and blob store required")
	}
	status, err := manifest.ExportStatus(ctx, ArtifactAnalyticsStream, streamID)
	if err != nil {
		return fmt.Errorf("manifest lookup: %w", err)
	}
	if status != StatusConfirmed {
		return fmt.Errorf("manifest status %q is not confirmed", status)
	}
	if _, err := blob.Get(ctx, RollupsBlobKey(streamID)); err != nil {
		return fmt.Errorf("rollups blob missing: %w", err)
	}
	return nil
}
