package analytics

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"streamclone/internal/metrics"
)

func (s *Store) GetStreamChatSourceMetadata(ctx context.Context, streamID string) (*StreamChatSourceMetadata, error) {
	if s == nil || streamID == "" {
		return nil, nil
	}
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(ctx, `
		SELECT chat_state, chat_source, source_confidence,
		       chat_coverage_pct, ivr_coverage_pct, live_coverage_pct, gql_coverage_pct,
		       missing_windows_json, source_windows_json, last_source_upgrade_at,
		       COALESCE(chat_source_detail, '')
		FROM analytics_streams
		WHERE stream_id = $1
	`, canonicalID)
	var meta StreamChatSourceMetadata
	var missing, sourceWindows []byte
	var upgraded *time.Time
	err = row.Scan(
		&meta.ChatState, &meta.ChatSource, &meta.SourceConfidence,
		&meta.ChatCoveragePct, &meta.IVRCoveragePct, &meta.LiveCoveragePct, &meta.GQLCoveragePct,
		&missing, &sourceWindows, &upgraded, &meta.ChatSourceDetail,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		meta.MissingWindows = missing
	}
	if len(sourceWindows) > 0 {
		meta.SourceWindows = sourceWindows
	}
	meta.LastSourceUpgrade = upgraded
	return &meta, nil
}

func (s *Store) UpsertStreamChatSourceMetadata(ctx context.Context, streamID string, meta StreamChatSourceMetadata) error {
	if s == nil || streamID == "" {
		return nil
	}
	resolved, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		return err
	}
	missing := meta.MissingWindows
	if len(missing) == 0 {
		missing = []byte("[]")
	}
	sourceWindows := meta.SourceWindows
	if len(sourceWindows) == 0 {
		sourceWindows = []byte("[]")
	}
	_, err = s.db.Exec(ctx, `
		UPDATE analytics_streams SET
			chat_state = $2,
			chat_source = $3,
			source_confidence = $4,
			chat_coverage_pct = $5,
			ivr_coverage_pct = $6,
			live_coverage_pct = $7,
			gql_coverage_pct = $8,
			missing_windows_json = $9::jsonb,
			source_windows_json = $10::jsonb,
			last_source_upgrade_at = COALESCE($11, last_source_upgrade_at),
			chat_source_detail = $12,
			updated_at = now()
		WHERE stream_id = $1
	`, resolved,
		meta.ChatState, meta.ChatSource, meta.SourceConfidence,
		meta.ChatCoveragePct, meta.IVRCoveragePct, meta.LiveCoveragePct, meta.GQLCoveragePct,
		string(missing), string(sourceWindows), meta.LastSourceUpgrade, meta.ChatSourceDetail,
	)
	return err
}

// BulkUpsertProvisionalIVRChatRollups writes IVR provisional rollups without overwriting GQL canonical.
func (s *Store) BulkUpsertProvisionalIVRChatRollups(ctx context.Context, streamID string, rollups []MinuteRollup) error {
	if s == nil || streamID == "" || len(rollups) == 0 {
		return nil
	}
	resolved, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		return err
	}
	streamID = resolved

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, rollup := range rollups {
		if rollup.Emotes == nil {
			rollup.Emotes = map[string]int{}
		}
		emotes, err := json.Marshal(rollup.Emotes)
		if err != nil {
			return err
		}
		batch.Queue(`
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ($1,$2,0,0,0,0,$3,$4,$5,$6::jsonb,$7,$8,$9)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				chat_count = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_count
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.chat_count
					ELSE EXCLUDED.chat_count
				END,
				total_emote_count = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.total_emote_count
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.total_emote_count
					ELSE EXCLUDED.total_emote_count
				END,
				seventv_emote_count = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.seventv_emote_count
					ELSE EXCLUDED.seventv_emote_count
				END,
				emotes_json = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.emotes_json
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.emotes_json
					ELSE EXCLUDED.emotes_json
				END,
				chat_source = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_source
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.chat_source
					ELSE EXCLUDED.chat_source
				END,
				source_confidence = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.source_confidence
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.source_confidence
					ELSE EXCLUDED.source_confidence
				END,
				chat_source_detail = CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_source_detail
					WHEN analytics_minute_rollups.chat_count > 0 AND COALESCE(analytics_minute_rollups.source_confidence,'') = 'verified' THEN analytics_minute_rollups.chat_source_detail
					ELSE EXCLUDED.chat_source_detail
				END,
				updated_at = now()`,
			streamID, rollup.MinuteTS.UTC(), rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount,
			string(emotes), RollupChatSourceIVR, SourceConfidenceProvisional, rollup.ChatSourceDetail,
		)
	}
	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return err
	}
	if err := refreshStreamSummaryObserved(ctx, tx, streamID, "immediate"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	metrics.AnalyticsRollupRowsWrittenTotal.WithLabelValues("ivr_provisional").Add(float64(len(rollups)))
	return nil
}
