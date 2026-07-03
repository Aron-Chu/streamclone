package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
	"streamclone/internal/emote/flags"
	"streamclone/internal/emoteimage"
	"streamclone/internal/metrics"
	"streamclone/internal/timeseries"
)

type Store struct {
	db                      *pgxpool.Pool
	telemetry               timeseries.Writer
	metaCache               sync.Map
	archiveProtectRetention bool
	postEnd                 *PostEndDetector
}

type streamMeta struct {
	login     string
	title     string
	category  string
	startedAt time.Time
}

const defaultTimeseriesBackfillBatchSize = 500

type TimeseriesBackfillSummary struct {
	StreamCount   uint64
	RollupCount   uint64
	ExportedCount uint64
}

type RollupWriteOptions struct {
	RefreshSummary     bool
	SummaryRefreshMode string
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) WithTelemetry(writer timeseries.Writer) *Store {
	if s != nil {
		s.telemetry = writer
	}
	return s
}

func (s *Store) WithArchiveProtectRetention(enabled bool) *Store {
	if s != nil {
		s.archiveProtectRetention = enabled
	}
	return s
}

func (s *Store) WithPostEndDetector(detector *PostEndDetector) *Store {
	if s != nil {
		s.postEnd = detector
	}
	return s
}

// Pool exposes the underlying connection pool so sibling stores (such as the
// chat-replay store) can be constructed against the same database without
// re-plumbing a pool through every constructor.
func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

const streamSelectColumns = `
	stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
	COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
	COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
	peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses,
	COALESCE(vod_id,''), COALESCE(vod_source,''),
	COALESCE(NULLIF(canonical_stream_id,''), stream_id), COALESCE(viewer_source,'')`

func (s *Store) ensureStreamStubBeforeSessionResolve(
	ctx context.Context,
	streamID, broadcasterID, login, title, category string,
	startedAt time.Time,
	viewerSource string,
) error {
	if streamID == "" || normalizeLogin(login) == "" {
		return nil
	}
	if strings.TrimSpace(broadcasterID) == "" {
		broadcasterID = "pending"
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if strings.TrimSpace(viewerSource) == "" {
		viewerSource = ViewerSourceUnknown
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, display_name,
			title, category, started_at, last_seen_at, tags, peak_viewers, viewer_source
		)
		VALUES ($1, $1, $2, $3, $3, $4, $5, $6, $6, '[]'::jsonb, 0, $7)
		ON CONFLICT (stream_id) DO NOTHING`,
		streamID, broadcasterID, normalizeLogin(login), nullIfEmpty(title), nullIfEmpty(category), startedAt, viewerSource,
	)
	return err
}

func (s *Store) UpsertLiveStream(ctx context.Context, stream LiveStream, profile UserProfile, seenAt time.Time) error {
	if stream.StartedAt.IsZero() {
		stream.StartedAt = seenAt
	}
	broadcasterID := stream.BroadcasterID
	if broadcasterID == "" {
		broadcasterID = profile.ID
	}
	incomingStreamID := stream.ID
	if err := s.ensureStreamStubBeforeSessionResolve(
		ctx, incomingStreamID, broadcasterID, stream.Login, stream.Title, stream.GameName, stream.StartedAt, ViewerSourceLive,
	); err != nil {
		return err
	}
	resolved, err := s.ResolveOrCreateSession(ctx, SessionResolveInput{
		Login:          stream.Login,
		StreamID:       stream.ID,
		TwitchStreamID: stream.ID,
		StartedAt:      stream.StartedAt,
		Title:          stream.Title,
		Category:       stream.GameName,
		Source:         ViewerSourceLive,
	})
	if err != nil {
		return err
	}
	stream.ID = resolved.CanonicalStreamID
	viewerSource := mergeViewerSources(resolved.ViewerSource, ViewerSourceLive)

	tags, err := json.Marshal(stream.Tags)
	if err != nil {
		return err
	}
	displayName := stream.DisplayName
	if profile.DisplayName != "" {
		displayName = profile.DisplayName
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, display_name, profile_image_url, description,
			title, category, tags, language, thumbnail_url, started_at, last_seen_at,
			current_viewers, peak_viewers, viewer_samples, viewer_source
		)
		VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$14,1,$15)
		ON CONFLICT (stream_id) DO UPDATE SET
			canonical_stream_id=EXCLUDED.canonical_stream_id,
			broadcaster_id=EXCLUDED.broadcaster_id,
			login=EXCLUDED.login,
			display_name=COALESCE(NULLIF(EXCLUDED.display_name,''), analytics_streams.display_name),
			profile_image_url=COALESCE(NULLIF(EXCLUDED.profile_image_url,''), analytics_streams.profile_image_url),
			description=COALESCE(NULLIF(EXCLUDED.description,''), analytics_streams.description),
			title=EXCLUDED.title,
			category=EXCLUDED.category,
			tags=EXCLUDED.tags,
			language=EXCLUDED.language,
			thumbnail_url=EXCLUDED.thumbnail_url,
			last_seen_at=EXCLUDED.last_seen_at,
			ended_at=NULL,
			current_viewers=EXCLUDED.current_viewers,
			peak_viewers=GREATEST(analytics_streams.peak_viewers, EXCLUDED.current_viewers),
			viewer_source=EXCLUDED.viewer_source,
			updated_at=now()`,
		stream.ID, broadcasterID, normalizeLogin(stream.Login), displayName, profile.ProfileImageURL, profile.Description,
		stream.Title, stream.GameName, string(tags), stream.Language, stream.ThumbnailURL, stream.StartedAt, seenAt, stream.ViewerCount,
		viewerSource,
	)
	if err == nil && stream.ID != "" {
		s.metaCache.Store(stream.ID, streamMeta{
			login:     normalizeLogin(stream.Login),
			title:     stream.Title,
			category:  stream.GameName,
			startedAt: stream.StartedAt,
		})
	}
	return err
}

func (s *Store) CloseStream(ctx context.Context, streamID string, endedAt time.Time) error {
	if streamID == "" {
		return nil
	}
	var login string
	_ = s.db.QueryRow(ctx, `SELECT login FROM analytics_streams WHERE stream_id=$1`, streamID).Scan(&login)
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET ended_at=COALESCE(ended_at, $2), updated_at=now()
		WHERE stream_id=$1`, streamID, endedAt)
	if err == nil && s.postEnd != nil {
		go func(id, channel string) {
			_ = s.postEnd.EnqueueIfNeeded(context.Background(), id, channel)
		}(streamID, login)
	}
	return err
}

func (s *Store) UpsertMinuteRollup(ctx context.Context, streamID string, rollup MinuteRollup) error {
	if streamID == "" || rollup.MinuteTS.IsZero() {
		return nil
	}
	started := time.Now()
	result := "success"
	defer func() {
		metrics.AnalyticsRollupWriteDuration.WithLabelValues("upsert", result).Observe(time.Since(started).Seconds())
	}()
	resolvedStreamID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		result = "error"
		return err
	}
	streamID = resolvedStreamID
	if rollup.Emotes == nil {
		rollup.Emotes = map[string]int{}
	}
	emotes, err := json.Marshal(rollup.Emotes)
	if err != nil {
		result = "error"
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		result = "error"
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO analytics_minute_rollups (
			stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json,
			chat_source, source_confidence
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
		ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
			viewer_avg=CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
			viewer_max=GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
			viewer_latest=CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
			viewer_samples=GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
			chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
			total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
			seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
			emotes_json=EXCLUDED.emotes_json,
			chat_source=CASE
				WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_source
				WHEN EXCLUDED.chat_count > 0 THEN EXCLUDED.chat_source
				ELSE analytics_minute_rollups.chat_source
			END,
			source_confidence=CASE
				WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.source_confidence
				WHEN EXCLUDED.chat_count > 0 THEN EXCLUDED.source_confidence
				ELSE analytics_minute_rollups.source_confidence
			END,
			updated_at=now()`,
		streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
		rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
		RollupChatSourceLive, SourceConfidenceVerified,
	)
	if err != nil {
		result = "error"
		return err
	}
	if err := refreshStreamSummaryObserved(ctx, tx, streamID, "single"); err != nil {
		result = "error"
		return err
	}
	if err := upsertMinutePeaksTx(ctx, tx, streamID, []MinuteRollup{rollup}, RollupChatSourceLive, SourceConfidenceVerified); err != nil {
		result = "error"
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	s.enqueueRollupTelemetry(ctx, streamID, []MinuteRollup{rollup})
	return nil
}

func upsertMinutePeaksTx(ctx context.Context, tx pgx.Tx, streamID string, rollups []MinuteRollup, defaultSource, defaultConfidence string) error {
	if tx == nil || streamID == "" || len(rollups) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	queued := 0
	for _, rollup := range rollups {
		if rollup.MinuteTS.IsZero() || rollup.ChatCount <= 0 {
			continue
		}
		if rollup.Emotes == nil {
			rollup.Emotes = map[string]int{}
		}
		emotes, err := json.Marshal(rollup.Emotes)
		if err != nil {
			return err
		}
		chatSource := strings.TrimSpace(rollup.ChatSource)
		if chatSource == "" {
			chatSource = defaultSource
		}
		sourceConfidence := strings.TrimSpace(rollup.SourceConfidence)
		if sourceConfidence == "" {
			sourceConfidence = defaultConfidence
		}
		batch.Queue(`
			INSERT INTO analytics_minute_peaks (
				stream_id, minute_ts, chat_count, total_emote_count, seventv_emote_count,
				emotes_json, chat_source, source_confidence, updated_at
			)
			VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,now())
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				chat_count=GREATEST(analytics_minute_peaks.chat_count, EXCLUDED.chat_count),
				total_emote_count=GREATEST(analytics_minute_peaks.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count=GREATEST(analytics_minute_peaks.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json=CASE
					WHEN EXCLUDED.chat_count >= analytics_minute_peaks.chat_count THEN EXCLUDED.emotes_json
					ELSE analytics_minute_peaks.emotes_json
				END,
				chat_source=CASE
					WHEN EXCLUDED.chat_count >= analytics_minute_peaks.chat_count THEN EXCLUDED.chat_source
					ELSE analytics_minute_peaks.chat_source
				END,
				source_confidence=CASE
					WHEN EXCLUDED.chat_count >= analytics_minute_peaks.chat_count THEN EXCLUDED.source_confidence
					ELSE analytics_minute_peaks.source_confidence
				END,
				updated_at=now()`,
			streamID, rollup.MinuteTS.UTC(), rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount,
			string(emotes), chatSource, sourceConfidence,
		)
		queued++
	}
	if queued == 0 {
		return nil
	}
	br := tx.SendBatch(ctx, batch)
	return br.Close()
}

func refreshStreamSummary(ctx context.Context, tx pgx.Tx, streamID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE analytics_streams s SET
			avg_viewers=CASE
				WHEN COALESCE((
					SELECT MAX(viewer_avg)
					FROM analytics_minute_rollups
					WHERE stream_id=$1
				), 0) > 0 THEN COALESCE((
					SELECT ROUND(AVG(NULLIF(viewer_avg, 0)))::int
					FROM analytics_minute_rollups
					WHERE stream_id=$1 AND viewer_samples > 0
				), s.avg_viewers)
				ELSE s.avg_viewers
			END,
			peak_viewers=GREATEST(s.peak_viewers, COALESCE((
				SELECT MAX(viewer_max)
				FROM analytics_minute_rollups
				WHERE stream_id=$1
			), 0)),
			viewer_samples=COALESCE((
				SELECT SUM(viewer_samples)::int
				FROM analytics_minute_rollups
				WHERE stream_id=$1
			), 0),
			chat_messages=COALESCE((
				SELECT SUM(chat_count)::bigint
				FROM analytics_minute_rollups
				WHERE stream_id=$1
			), 0),
			total_emote_uses=COALESCE((
				SELECT SUM(total_emote_count)::bigint
				FROM analytics_minute_rollups
				WHERE stream_id=$1
			), 0),
			seventv_emote_uses=COALESCE((
				SELECT SUM(seventv_emote_count)::bigint
				FROM analytics_minute_rollups
				WHERE stream_id=$1
			), 0),
			updated_at=now()
		WHERE s.stream_id=$1`, streamID)
	return err
}

func normalizeRollupWriteOptions(opts RollupWriteOptions) RollupWriteOptions {
	if strings.TrimSpace(opts.SummaryRefreshMode) == "" {
		opts.SummaryRefreshMode = "immediate"
	}
	return opts
}

func observeSummaryRefresh(mode string, started time.Time, err error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "immediate"
	}
	result := "success"
	if err != nil {
		result = "error"
	}
	metrics.AnalyticsVODGQLSummaryRefreshTotal.WithLabelValues(mode, result).Inc()
	metrics.AnalyticsVODGQLSummaryRefreshDuration.WithLabelValues(mode, result).Observe(time.Since(started).Seconds())
}

func refreshStreamSummaryObserved(ctx context.Context, tx pgx.Tx, streamID, mode string) error {
	started := time.Now()
	err := refreshStreamSummary(ctx, tx, streamID)
	observeSummaryRefresh(mode, started, err)
	return err
}

func (s *Store) RefreshStreamSummary(ctx context.Context, streamID string) error {
	return s.RefreshStreamSummaryWithMode(ctx, streamID, "manual")
}

func (s *Store) RefreshStreamSummaryWithMode(ctx context.Context, streamID, mode string) error {
	if s == nil || s.db == nil || strings.TrimSpace(streamID) == "" {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := refreshStreamSummaryObserved(ctx, tx, streamID, mode); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) LatestStreamByLogin(ctx context.Context, login string) (*StreamRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,''),
			COALESCE(canonical_stream_id, stream_id), COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE login=$1
		  AND COALESCE(canonical_stream_id, stream_id) = stream_id
		ORDER BY ended_at IS NULL DESC, last_seen_at DESC, started_at DESC
		LIMIT 1`, normalizeLogin(login))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	return scanStream(rows)
}

func (s *Store) LatestStreamsByLogins(ctx context.Context, logins []string) (map[string]*StreamRecord, error) {
	out := make(map[string]*StreamRecord, len(logins))
	if s == nil || len(logins) == 0 {
		return out, nil
	}
	normalized := make([]string, 0, len(logins))
	seen := make(map[string]struct{}, len(logins))
	for _, login := range logins {
		login = normalizeLogin(login)
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		normalized = append(normalized, login)
	}
	if len(normalized) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (login)
			stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,''),
			COALESCE(canonical_stream_id, stream_id), COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE login = ANY($1)
		  AND COALESCE(canonical_stream_id, stream_id) = stream_id
		ORDER BY login, ended_at IS NULL DESC, last_seen_at DESC, started_at DESC`, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		rec, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		out[rec.Login] = rec
	}
	return out, rows.Err()
}

func (s *Store) StreamByID(ctx context.Context, streamID string) (*StreamRecord, error) {
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,''),
			COALESCE(canonical_stream_id, stream_id), COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE stream_id=$1`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	return scanStream(rows)
}

func (s *Store) StreamByVodID(ctx context.Context, vodID string) (*StreamRecord, error) {
	vodID = strings.TrimSpace(vodID)
	if vodID == "" {
		return nil, pgx.ErrNoRows
	}
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,''),
			COALESCE(canonical_stream_id, stream_id), COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE vod_id=$1
		  AND COALESCE(canonical_stream_id, stream_id) = stream_id
		ORDER BY ended_at DESC NULLS LAST, last_seen_at DESC, started_at DESC
		LIMIT 1`, vodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	return scanStream(rows)
}

func (s *Store) GetStreamUpdatedAt(ctx context.Context, streamID string) (time.Time, error) {
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return time.Time{}, err
	}
	var updatedAt time.Time
	err = s.db.QueryRow(ctx,
		`SELECT updated_at FROM analytics_streams WHERE stream_id = $1`,
		canonicalID,
	).Scan(&updatedAt)
	return updatedAt, err
}

func (s *Store) StreamsByLogin(ctx context.Context, login string, limit int) ([]StreamRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,''),
			COALESCE(canonical_stream_id, stream_id), COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE login=$1
		  AND COALESCE(canonical_stream_id, stream_id) = stream_id
		ORDER BY ended_at IS NULL DESC, last_seen_at DESC, started_at DESC
		LIMIT $2`, normalizeLogin(login), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StreamRecord
	for rows.Next() {
		rec, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

func (s *Store) RollupsByStream(ctx context.Context, streamID string) ([]MinuteRollup, error) {
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json,
			COALESCE(chat_source, ''), COALESCE(source_confidence, ''), COALESCE(chat_source_detail, '')
		FROM analytics_minute_rollups
		WHERE stream_id=$1
		ORDER BY minute_ts ASC`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinuteRollup
	for rows.Next() {
		var r MinuteRollup
		var raw []byte
		if err := rows.Scan(&r.MinuteTS, &r.ViewerAvg, &r.ViewerMax, &r.ViewerLatest, &r.ViewerSamples, &r.ChatCount, &r.TotalEmoteCount, &r.SevenTVEmoteCount, &raw, &r.ChatSource, &r.SourceConfidence, &r.ChatSourceDetail); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &r.Emotes)
		}
		if r.Emotes == nil {
			r.Emotes = map[string]int{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecentRollupsByStreamID returns up to limit most-recent minute rollups for an
// already-canonical stream id, ordered ascending by minute. Unlike
// RollupsByStream it skips the canonical-id resolve query and bounds the row
// count, which keeps the public hub aggregate cheap when joining many channels.
func (s *Store) RecentRollupsByStreamID(ctx context.Context, canonicalStreamID string, limit int) ([]MinuteRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	canonicalStreamID = strings.TrimSpace(canonicalStreamID)
	if canonicalStreamID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 240 {
		limit = 60
	}
	rows, err := s.db.Query(ctx, `
		SELECT minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json,
			COALESCE(chat_source, ''), COALESCE(source_confidence, ''), COALESCE(chat_source_detail, '')
		FROM analytics_minute_rollups
		WHERE stream_id=$1
		ORDER BY minute_ts DESC
		LIMIT $2`, canonicalStreamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinuteRollup
	for rows.Next() {
		var r MinuteRollup
		var raw []byte
		if err := rows.Scan(&r.MinuteTS, &r.ViewerAvg, &r.ViewerMax, &r.ViewerLatest, &r.ViewerSamples, &r.ChatCount, &r.TotalEmoteCount, &r.SevenTVEmoteCount, &raw, &r.ChatSource, &r.SourceConfidence, &r.ChatSourceDetail); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &r.Emotes)
		}
		if r.Emotes == nil {
			r.Emotes = map[string]int{}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to ascending (query fetched newest-first for the LIMIT).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// LatestLiveStreamWithRecentRollupsByLogin finds the currently-open stream row
// for a login that is actually receiving recent rollups. Top-N metadata can
// refresh a metadata-only row more recently than the IRC collector row; hub and
// readiness views use this only as a fallback when their preferred stream has no
// fresh rollup signal.
func (s *Store) LatestLiveStreamWithRecentRollupsByLogin(ctx context.Context, login string, since time.Time, limit int) (*StreamRecord, []MinuteRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil, nil
	}
	login = normalizeLogin(login)
	if login == "" {
		return nil, nil, nil
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-15 * time.Minute)
	}
	if limit <= 0 || limit > 240 {
		limit = 60
	}
	row := s.db.QueryRow(ctx, `
		SELECT `+streamSelectColumns+`
		FROM analytics_streams
		JOIN LATERAL (
			SELECT MAX(minute_ts) AS latest_rollup_at
			FROM analytics_minute_rollups
			WHERE stream_id = COALESCE(NULLIF(analytics_streams.canonical_stream_id,''), analytics_streams.stream_id)
			  AND minute_ts >= $2
			  AND (
				chat_count > 0 OR total_emote_count > 0 OR seventv_emote_count > 0 OR viewer_samples > 0
			  )
		) recent ON recent.latest_rollup_at IS NOT NULL
		WHERE login=$1
		  AND ended_at IS NULL
		  AND COALESCE(NULLIF(canonical_stream_id,''), stream_id) = stream_id
		ORDER BY recent.latest_rollup_at DESC, last_seen_at DESC, started_at DESC
		LIMIT 1`, login, since.UTC())
	rec, err := scanStream(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rollups, err := s.RecentRollupsByStreamID(ctx, rec.StreamID, limit)
	if err != nil {
		return rec, nil, err
	}
	return rec, rollupsSince(rollups, since), nil
}

// RecentRollupBucketsByStreamID returns bounded aggregate buckets for a recent
// time window. It is used by the public hub's long-range activity chart so the
// portal can show 24h/7d/month views without returning raw rollups.
func (s *Store) RecentRollupBucketsByStreamID(ctx context.Context, canonicalStreamID string, since time.Time, bucketMinutes, limit int) ([]MinuteRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	canonicalStreamID = strings.TrimSpace(canonicalStreamID)
	if canonicalStreamID == "" {
		return nil, nil
	}
	if bucketMinutes <= 0 {
		bucketMinutes = 1
	}
	if limit <= 0 || limit > 240 {
		limit = 240
	}
	rows, err := s.db.Query(ctx, `
		WITH bucketed AS (
			SELECT
				to_timestamp(floor(extract(epoch from minute_ts) / ($3::double precision * 60)) * ($3::double precision * 60)) AS bucket_ts,
				minute_ts,
				viewer_avg,
				viewer_max,
				viewer_latest,
				viewer_samples,
				chat_count,
				total_emote_count,
				seventv_emote_count,
				chat_source,
				source_confidence
			FROM analytics_minute_rollups
			WHERE stream_id=$1 AND minute_ts >= $2
		)
		SELECT *
		FROM (
			SELECT
				bucket_ts,
				COALESCE(AVG(NULLIF(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_avg ELSE NULL END, 0)), 0)::int AS viewer_avg,
				COALESCE(MAX(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_max ELSE NULL END), 0)::int AS viewer_max,
				COALESCE(MAX(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_latest ELSE NULL END), 0)::int AS viewer_latest,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_samples ELSE 0 END), 0)::int AS viewer_samples,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN chat_count ELSE 0 END), 0)::int AS chat_count,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN total_emote_count ELSE 0 END), 0)::int AS total_emote_count,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN seventv_emote_count ELSE 0 END), 0)::int AS seventv_emote_count
			FROM bucketed
			GROUP BY bucket_ts
			ORDER BY bucket_ts DESC
			LIMIT $4
		) recent
		ORDER BY bucket_ts ASC`, canonicalStreamID, since.UTC(), bucketMinutes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinuteRollup
	for rows.Next() {
		var r MinuteRollup
		if err := rows.Scan(&r.MinuteTS, &r.ViewerAvg, &r.ViewerMax, &r.ViewerLatest, &r.ViewerSamples, &r.ChatCount, &r.TotalEmoteCount, &r.SevenTVEmoteCount); err != nil {
			return nil, err
		}
		r.Emotes = map[string]int{}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AggregateRollupBucketsSince returns bounded all-stream aggregate buckets for
// the public hub's longer activity windows. It intentionally omits emotes_json:
// callers get aggregate chat/viewer/emote counts without raw rollup maps.
func (s *Store) AggregateRollupBucketsSince(ctx context.Context, since time.Time, bucketMinutes, limit int) ([]MinuteRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if bucketMinutes <= 0 {
		bucketMinutes = 1
	}
	if limit <= 0 || limit > 240 {
		limit = 240
	}
	rows, err := s.db.Query(ctx, `
		WITH bucketed AS (
			SELECT
				to_timestamp(floor(extract(epoch from minute_ts) / ($2::double precision * 60)) * ($2::double precision * 60)) AS bucket_ts,
				viewer_avg,
				viewer_max,
				viewer_latest,
				viewer_samples,
				chat_count,
				total_emote_count,
				seventv_emote_count,
				chat_source,
				source_confidence
			FROM analytics_minute_rollups
			WHERE minute_ts >= $1
		)
		SELECT *
		FROM (
			SELECT
				bucket_ts,
				COALESCE(AVG(NULLIF(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_avg ELSE NULL END, 0)), 0)::int AS viewer_avg,
				COALESCE(MAX(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_max ELSE NULL END), 0)::int AS viewer_max,
				COALESCE(MAX(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_latest ELSE NULL END), 0)::int AS viewer_latest,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveViewerRollupPredicate+` THEN viewer_samples ELSE 0 END), 0)::int AS viewer_samples,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN chat_count ELSE 0 END), 0)::int AS chat_count,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN total_emote_count ELSE 0 END), 0)::int AS total_emote_count,
				COALESCE(SUM(CASE WHEN `+sqlPublicLiveChatMinutePredicate+` THEN seventv_emote_count ELSE 0 END), 0)::int AS seventv_emote_count
			FROM bucketed
			GROUP BY bucket_ts
			ORDER BY bucket_ts DESC
			LIMIT $3
		) recent
		ORDER BY bucket_ts ASC`, since.UTC(), bucketMinutes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinuteRollup
	for rows.Next() {
		var r MinuteRollup
		if err := rows.Scan(&r.MinuteTS, &r.ViewerAvg, &r.ViewerMax, &r.ViewerLatest, &r.ViewerSamples, &r.ChatCount, &r.TotalEmoteCount, &r.SevenTVEmoteCount); err != nil {
			return nil, err
		}
		r.Emotes = map[string]int{}
		out = append(out, r)
	}
	return out, rows.Err()
}

// HubProviderBucketCounts holds per-minute non-7TV provider totals for hub activity charts.
type HubProviderBucketCounts struct {
	Twitch int
	BTTV   int
	FFZ    int
}

// AggregateRollupProviderBucketsSince sums twitch/bttv/ffz uses from emotes_json for
// live IRC rollups across the corpus, keyed by bucket start (Unix milliseconds).
func (s *Store) AggregateRollupProviderBucketsSince(ctx context.Context, since time.Time, bucketMinutes, limit int) (map[int64]HubProviderBucketCounts, error) {
	out := map[int64]HubProviderBucketCounts{}
	if s == nil || s.db == nil {
		return out, nil
	}
	if bucketMinutes <= 0 {
		bucketMinutes = 1
	}
	if limit <= 0 || limit > 240 {
		limit = 240
	}
	rows, err := s.db.Query(ctx, `
		WITH bucketed AS (
			SELECT
				to_timestamp(floor(extract(epoch from minute_ts) / ($2::double precision * 60)) * ($2::double precision * 60)) AS bucket_ts,
				emotes_json,
				chat_count,
				chat_source,
				source_confidence,
				viewer_samples
			FROM analytics_minute_rollups
			WHERE minute_ts >= $1
		),
		expanded AS (
			SELECT
				b.bucket_ts,
				split_part(e.key, ':', 1) AS provider,
				e.value::int AS cnt
			FROM bucketed b,
			LATERAL jsonb_each_text(COALESCE(b.emotes_json, '{}'::jsonb)) AS e(key, value)
			WHERE `+sqlPublicLiveChatMinutePredicate+`
				AND split_part(e.key, ':', 1) IN ('twitch', 'bttv', 'ffz')
		),
		aggregated AS (
			SELECT bucket_ts, provider, SUM(cnt)::int AS total
			FROM expanded
			GROUP BY bucket_ts, provider
		),
		recent_buckets AS (
			SELECT DISTINCT bucket_ts
			FROM aggregated
			ORDER BY bucket_ts DESC
			LIMIT $3
		)
		SELECT
			(extract(epoch FROM a.bucket_ts) * 1000)::bigint AS bucket_ms,
			a.provider,
			a.total
		FROM aggregated a
		INNER JOIN recent_buckets rb ON rb.bucket_ts = a.bucket_ts
		ORDER BY a.bucket_ts ASC`, since.UTC(), bucketMinutes, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var bucketMS int64
		var provider string
		var total int
		if err := rows.Scan(&bucketMS, &provider, &total); err != nil {
			return out, err
		}
		entry := out[bucketMS]
		switch strings.ToLower(strings.TrimSpace(provider)) {
		case "twitch":
			entry.Twitch += total
		case "bttv":
			entry.BTTV += total
		case "ffz":
			entry.FFZ += total
		}
		out[bucketMS] = entry
	}
	return out, rows.Err()
}

// TopHistoricalChatMinutesInWindow returns bounded per-stream hot minutes for a
// public hub activity bucket. It prefers analytics_minute_peaks so bucket-clicks
// do not sort raw rollup windows; emotes_json is read server-side only.
func (s *Store) TopHistoricalChatMinutesInWindow(ctx context.Context, start, end time.Time, limit int) ([]hubHistoricalMinuteCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if !end.After(start) {
		return nil, nil
	}
	if limit <= 0 || limit > hubHistoricalCandidateCap {
		limit = hubHistoricalCandidateCap
	}
	out, err := s.topHistoricalChatMinutesFromPeaks(ctx, start, end, limit)
	if err == nil {
		return out, nil
	}
	if !isUndefinedTableError(err) {
		return nil, err
	}
	return s.topHistoricalChatMinutesFromRollups(ctx, start, end, limit)
}

func (s *Store) topHistoricalChatMinutesFromPeaks(ctx context.Context, start, end time.Time, limit int) ([]hubHistoricalMinuteCandidate, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			s.stream_id,
			s.login,
			COALESCE(s.display_name, ''),
			COALESCE(s.profile_image_url, ''),
			COALESCE(s.vod_id, ''),
			s.started_at,
			p.minute_ts,
			p.chat_count,
			p.total_emote_count,
			p.seventv_emote_count,
			COALESCE(p.emotes_json, '{}'::jsonb)
		FROM analytics_minute_peaks p
		JOIN analytics_streams s ON s.stream_id = p.stream_id
		WHERE p.minute_ts >= $1
		  AND p.minute_ts < $2
		  AND p.chat_count > 0
		ORDER BY p.chat_count DESC, p.total_emote_count DESC, p.minute_ts DESC
		LIMIT $3`, start.UTC(), end.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHubHistoricalMinuteCandidates(rows, limit)
}

func (s *Store) topHistoricalChatMinutesFromRollups(ctx context.Context, start, end time.Time, limit int) ([]hubHistoricalMinuteCandidate, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			s.stream_id,
			s.login,
			COALESCE(s.display_name, ''),
			COALESCE(s.profile_image_url, ''),
			COALESCE(s.vod_id, ''),
			s.started_at,
			r.minute_ts,
			r.chat_count,
			r.total_emote_count,
			r.seventv_emote_count,
			COALESCE(r.emotes_json, '{}'::jsonb)
		FROM analytics_minute_rollups r
		JOIN analytics_streams s ON s.stream_id = r.stream_id
		WHERE r.minute_ts >= $1
		  AND r.minute_ts < $2
		  AND r.chat_count > 0
		  AND `+sqlPublicLiveChatMinutePredicate+`
		ORDER BY r.chat_count DESC, r.total_emote_count DESC, r.minute_ts DESC
		LIMIT $3`, start.UTC(), end.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHubHistoricalMinuteCandidates(rows, limit)
}

type historicalCandidateRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanHubHistoricalMinuteCandidates(rows historicalCandidateRows, limit int) ([]hubHistoricalMinuteCandidate, error) {
	out := make([]hubHistoricalMinuteCandidate, 0, limit)
	for rows.Next() {
		var cand hubHistoricalMinuteCandidate
		var rawEmotes []byte
		if err := rows.Scan(
			&cand.StreamID,
			&cand.Login,
			&cand.DisplayName,
			&cand.ProfileImageURL,
			&cand.VodID,
			&cand.StartedAt,
			&cand.MinuteTS,
			&cand.ChatCount,
			&cand.TotalEmoteCount,
			&cand.SevenTVEmoteCount,
			&rawEmotes,
		); err != nil {
			return nil, err
		}
		if len(rawEmotes) > 0 {
			_ = json.Unmarshal(rawEmotes, &cand.Emotes)
		}
		if cand.Emotes == nil {
			cand.Emotes = map[string]int{}
		}
		out = append(out, cand)
	}
	return out, rows.Err()
}

func isUndefinedTableError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func (s *Store) SetStreamVodID(ctx context.Context, streamID, vodID, source string) error {
	if streamID == "" || vodID == "" {
		return nil
	}
	canonicalID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		return err
	}
	streamID = canonicalID
	source = strings.TrimSpace(source)
	if source == "" {
		_, err = s.db.Exec(ctx, `
			UPDATE analytics_streams
			SET vod_id=$2, updated_at=now()
			WHERE stream_id=$1`, streamID, vodID)
	} else {
		_, err = s.db.Exec(ctx, `
			UPDATE analytics_streams
			SET vod_id=$2, vod_source=$3, updated_at=now()
			WHERE stream_id=$1`, streamID, vodID, source)
	}
	if err != nil {
		return err
	}
	_ = s.recordPulseVODAvailable(ctx, streamID, vodID, source)
	return err
}

func (s *Store) recordPulseVODAvailable(ctx context.Context, streamID, vodID, source string) error {
	now := time.Now().UTC()
	var rec StreamRecord
	queryErr := s.db.QueryRow(ctx, `
		SELECT stream_id, COALESCE(login,''), COALESCE(broadcaster_id,'')
		FROM analytics_streams
		WHERE stream_id=$1`, streamID).Scan(&rec.StreamID, &rec.Login, &rec.BroadcasterID)
	if queryErr != nil {
		rec.StreamID = streamID
	}
	return s.RecordPulseVODResolutionAttempt(ctx, PulseVODResolutionAttemptInput{
		StreamID:       rec.StreamID,
		Login:          rec.Login,
		TwitchStreamID: rec.StreamID,
		BroadcasterID:  rec.BroadcasterID,
		CandidateVodID: vodID,
		Source:         source,
		Status:         "available",
		Attempts:       1,
		LastAttemptAt:  &now,
		FinalizedAt:    &now,
	})
}

type PulseVODResolutionAttemptInput struct {
	StreamID           string
	Login              string
	TwitchStreamID     string
	BroadcasterID      string
	CandidateVodID     string
	Source             string
	Status             string
	Attempts           int
	LastAttemptAt      *time.Time
	NextAutoRetryAt    *time.Time
	FinalAfterAt       *time.Time
	FinalizedAt        *time.Time
	ManualRetryAllowed bool
	ErrorCode          string
}

func (s *Store) RecordPulseVODResolutionAttempt(ctx context.Context, in PulseVODResolutionAttemptInput) error {
	if s == nil || s.db == nil || strings.TrimSpace(in.StreamID) == "" || strings.TrimSpace(in.Status) == "" {
		return nil
	}
	if in.Attempts < 0 {
		in.Attempts = 0
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pulse_vod_resolution_attempts (
			stream_id, login, twitch_stream_id, broadcaster_id, candidate_vod_id,
			source, status, attempts, last_attempt_at, next_auto_retry_at,
			final_after_at, finalized_at, manual_retry_allowed, error_code
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		strings.TrimSpace(in.StreamID),
		normalizeLogin(in.Login),
		strings.TrimSpace(in.TwitchStreamID),
		NormalizeBroadcasterID(in.BroadcasterID),
		strings.TrimSpace(in.CandidateVodID),
		strings.TrimSpace(in.Source),
		strings.TrimSpace(in.Status),
		in.Attempts,
		in.LastAttemptAt,
		in.NextAutoRetryAt,
		in.FinalAfterAt,
		in.FinalizedAt,
		in.ManualRetryAllowed,
		strings.TrimSpace(in.ErrorCode),
	)
	return err
}

// VodSourceUnlinked marks a closed stream whose VOD id did not resolve within the
// post-close resolution window. Live-collected minute rollups remain stored under
// the live stream id; a later sync or manual trigger can resolve the VOD and link
// them via SetStreamVodID, which overwrites this marker. See Requirements 19.3/19.4.
const VodSourceUnlinked = "unlinked"

// MarkStreamVodUnlinked records that a closed stream's VOD id could not be
// resolved within the resolution window. It only stamps the marker when the
// stream is still unlinked (no vod_id and not already marked) so it never
// clobbers a real VOD association produced by SetStreamVodID.
func (s *Store) MarkStreamVodUnlinked(ctx context.Context, streamID string) error {
	if streamID == "" {
		return nil
	}
	now := time.Now().UTC()
	var rec StreamRecord
	queryErr := s.db.QueryRow(ctx, `
		SELECT stream_id, COALESCE(login,''), COALESCE(broadcaster_id,'')
		FROM analytics_streams
		WHERE stream_id=$1`, streamID).Scan(&rec.StreamID, &rec.Login, &rec.BroadcasterID)
	if queryErr != nil {
		rec.StreamID = streamID
	}
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET vod_source=$2, updated_at=now()
		WHERE stream_id=$1
		  AND COALESCE(vod_id,'')=''
		  AND COALESCE(vod_source,'') <> $2`, streamID, VodSourceUnlinked)
	if err != nil {
		return err
	}
	_ = s.RecordPulseVODResolutionAttempt(ctx, PulseVODResolutionAttemptInput{
		StreamID:           rec.StreamID,
		Login:              rec.Login,
		TwitchStreamID:     rec.StreamID,
		BroadcasterID:      rec.BroadcasterID,
		Source:             "helix_stream_match",
		Status:             "unavailable",
		LastAttemptAt:      &now,
		FinalizedAt:        &now,
		ManualRetryAllowed: true,
		ErrorCode:          "vod_unavailable",
	})
	return err
}

type SyncCheckpoint struct {
	StreamID        string
	VideoID         string
	Cursor          string
	OffsetSeconds   int
	CommentsFetched int
	SegmentsJSON    string
	FetchMode       string
	UpdatedAt       time.Time
}

func (s *Store) UpsertStreamPlaceholder(ctx context.Context, streamID, broadcasterID, login, title string, startedAt time.Time) error {
	if streamID == "" || login == "" {
		return nil
	}
	if strings.TrimSpace(broadcasterID) == "" {
		broadcasterID = "pending"
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if title == "" {
		title = "Syncing..."
	}
	if err := s.ensureStreamStubBeforeSessionResolve(
		ctx, streamID, broadcasterID, login, title, "Live", startedAt, ViewerSourceUnknown,
	); err != nil {
		return err
	}
	resolved, err := s.ResolveOrCreateSession(ctx, SessionResolveInput{
		Login:         login,
		StreamID:      streamID,
		TTStreamID:    streamID,
		StartedAt:     startedAt,
		Title:         title,
		Source:        ViewerSourceUnknown,
		IsPlaceholder: true,
	})
	if err != nil {
		return err
	}
	streamID = resolved.CanonicalStreamID
	if resolved.MergedFrom != "" {
		_, err = s.db.Exec(ctx, `
			UPDATE analytics_streams
			SET title = COALESCE(NULLIF($2, ''), title),
			    started_at = LEAST(started_at, $3),
			    last_seen_at = GREATEST(last_seen_at, $3),
			    updated_at = now()
			WHERE stream_id = $1`,
			streamID, title, startedAt,
		)
		if err == nil {
			s.metaCache.Store(streamID, streamMeta{
				login:     normalizeLogin(login),
				title:     title,
				startedAt: startedAt,
			})
		}
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, display_name, title, category,
			started_at, last_seen_at, tags, peak_viewers, viewer_source
		)
		VALUES ($1, $1, $2, $3, $3, $4, 'Live', $5, $5, '[]'::jsonb, 0, $6)
		ON CONFLICT (stream_id) DO UPDATE SET
			canonical_stream_id = EXCLUDED.canonical_stream_id,
			broadcaster_id = CASE
				WHEN COALESCE(analytics_streams.broadcaster_id, '') IN ('', 'pending') THEN EXCLUDED.broadcaster_id
				ELSE analytics_streams.broadcaster_id
			END,
			title = COALESCE(NULLIF(EXCLUDED.title, ''), analytics_streams.title),
			updated_at = now()`,
		streamID, broadcasterID, login, title, startedAt, resolved.ViewerSource,
	)
	if err == nil {
		s.metaCache.Store(streamID, streamMeta{
			login:     normalizeLogin(login),
			title:     title,
			startedAt: startedAt,
		})
	}
	return err
}

func (s *Store) GetSyncCheckpoint(ctx context.Context, streamID, videoID string) (*SyncCheckpoint, error) {
	if streamID == "" || videoID == "" {
		return nil, nil
	}
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	streamID = canonicalID
	var cp SyncCheckpoint
	err = s.db.QueryRow(ctx, `
		SELECT stream_id, video_id, COALESCE(cursor, ''), offset_seconds, comments_fetched,
		       COALESCE(segments_json, ''), COALESCE(fetch_mode, ''), updated_at
		FROM analytics_sync_checkpoints
		WHERE stream_id=$1 AND video_id=$2`, streamID, videoID).Scan(
		&cp.StreamID, &cp.VideoID, &cp.Cursor, &cp.OffsetSeconds, &cp.CommentsFetched,
		&cp.SegmentsJSON, &cp.FetchMode, &cp.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cp, nil
}

func (s *Store) UpsertSyncCheckpoint(ctx context.Context, cp SyncCheckpoint) error {
	if cp.StreamID == "" || cp.VideoID == "" {
		return nil
	}
	canonicalID, err := s.ResolveStreamIDForWrite(ctx, cp.StreamID)
	if err != nil {
		return err
	}
	cp.StreamID = canonicalID
	_, err = s.db.Exec(ctx, `
		INSERT INTO analytics_sync_checkpoints (
			stream_id, video_id, cursor, offset_seconds, comments_fetched, segments_json, fetch_mode, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (stream_id, video_id) DO UPDATE SET
			cursor=EXCLUDED.cursor,
			offset_seconds=EXCLUDED.offset_seconds,
			comments_fetched=EXCLUDED.comments_fetched,
			segments_json=EXCLUDED.segments_json,
			fetch_mode=EXCLUDED.fetch_mode,
			updated_at=now()`,
		cp.StreamID, cp.VideoID, cp.Cursor, cp.OffsetSeconds, cp.CommentsFetched, cp.SegmentsJSON, cp.FetchMode,
	)
	return err
}

func (s *Store) DeleteSyncCheckpoint(ctx context.Context, streamID, videoID string) error {
	if streamID == "" || videoID == "" {
		return nil
	}
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return err
	}
	streamID = canonicalID
	_, err = s.db.Exec(ctx, `
		DELETE FROM analytics_sync_checkpoints
		WHERE stream_id=$1 AND video_id=$2`, streamID, videoID)
	return err
}

func (s *Store) PurgeOlderThan(ctx context.Context, cutoff time.Time) error {
	if s.archiveProtectRetention {
		var missing int64
		err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM analytics_streams st
			WHERE st.started_at < $1
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $2
				  AND ae.natural_key = st.stream_id
				  AND ae.export_status = 'confirmed'
			  )`,
			cutoff, archive.ArtifactAnalyticsStream,
		).Scan(&missing)
		if err != nil {
			return err
		}
		if err := archive.BlockIfMissing(archive.ArtifactAnalyticsStream, missing); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(ctx, `DELETE FROM analytics_streams WHERE started_at < $1`, cutoff)
	return err
}

func TopEmotesFromRollups(rollups []MinuteRollup, limit int) []TopEmote {
	if limit <= 0 {
		limit = 50
	}
	counts := map[string]int{}
	for _, rollup := range rollups {
		for key, count := range rollup.Emotes {
			counts[key] += count
		}
	}
	items := make([]TopEmote, 0, len(counts))
	for key, count := range counts {
		name, id, provider := splitEmoteKey(key)
		items = append(items, TopEmote{
			Key:      key,
			Name:     name,
			ID:       id,
			Provider: provider,
			ImageURL: emoteImageURL(provider, id),
			Count:    count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

type streamScanner interface {
	Scan(dest ...any) error
}

func scanStream(row streamScanner) (*StreamRecord, error) {
	var rec StreamRecord
	var tagsRaw []byte
	var endedAt *time.Time
	if err := row.Scan(
		&rec.StreamID, &rec.BroadcasterID, &rec.Login, &rec.DisplayName, &rec.ProfileImageURL,
		&rec.Description, &rec.Title, &rec.Category, &tagsRaw, &rec.Language, &rec.ThumbnailURL,
		&rec.StartedAt, &endedAt, &rec.LastSeenAt, &rec.CurrentViewers, &rec.AvgViewers,
		&rec.PeakViewers, &rec.ViewerSamples, &rec.ChatMessages, &rec.TotalEmoteUses,
		&rec.SevenTVEmoteUses, &rec.VodID, &rec.VodSource, &rec.CanonicalStreamID, &rec.ViewerSource,
	); err != nil {
		return nil, err
	}
	rec.EndedAt = endedAt
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &rec.Tags)
	}
	if rec.Tags == nil {
		rec.Tags = []string{}
	}
	return &rec, nil
}

func normalizeRollup(rollup MinuteRollup, topN int) MinuteRollup {
	if rollup.ViewerSamples > 0 {
		rollup.ViewerAvg = int(math.Round(float64(rollup.ViewerAvg)))
	}
	rollup.Emotes = topNMap(rollup.Emotes, topN)
	return rollup
}

func topNMap(values map[string]int, limit int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	if limit <= 0 || len(values) <= limit {
		return values
	}
	type kv struct {
		k string
		v int
	}
	items := make([]kv, 0, len(values))
	for k, v := range values {
		items = append(items, kv{k: k, v: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v == items[j].v {
			return items[i].k < items[j].k
		}
		return items[i].v > items[j].v
	})
	out := make(map[string]int, limit)
	for _, item := range items[:limit] {
		out[item.k] = item.v
	}
	return out
}

func splitEmoteKey(key string) (name, id, provider string) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) == 3 {
		return parts[2], parts[1], parts[0]
	}
	if len(parts) == 2 {
		return parts[1], parts[1], parts[0]
	}
	return key, "", ""
}

func emoteImageURL(provider, id string) string {
	return emoteimage.URL(provider, id, "1x")
}

// LookupProviderEmoteIDs maps synced emote-service UUIDs to upstream provider ids.
func (s *Store) LookupProviderEmoteIDs(ctx context.Context, localIDs []string) (map[string]string, error) {
	return s.lookupProviderEmoteIDs(ctx, localIDs, nil)
}

// LookupSevenTVProviderEmoteIDs maps synced emote-service UUIDs to 7TV provider ids.
func (s *Store) LookupSevenTVProviderEmoteIDs(ctx context.Context, localIDs []string) (map[string]string, error) {
	return s.lookupProviderEmoteIDs(ctx, localIDs, []string{"seventv", "7tv"})
}

type EmoteMetadata struct {
	ZeroWidth bool
	Animated  bool
}

func (s *Store) LookupEmoteMetadata(ctx context.Context, localIDs []string) (map[string]EmoteMetadata, error) {
	out := map[string]EmoteMetadata{}
	if s == nil || s.db == nil || len(localIDs) == 0 {
		return out, nil
	}
	unique := make([]string, 0, len(localIDs))
	seen := map[string]struct{}{}
	for _, id := range localIDs {
		id = strings.TrimSpace(id)
		if !emoteimage.IsLocalEmoteID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, flags, animated
		FROM emotes
		WHERE id = ANY($1::uuid[])`, unique)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var localID string
		var packedFlags int
		var animated bool
		if err := rows.Scan(&localID, &packedFlags, &animated); err != nil {
			return nil, err
		}
		out[localID] = EmoteMetadata{
			ZeroWidth: flags.IsZeroWidth(packedFlags),
			Animated:  animated || flags.IsAnimated(packedFlags),
		}
	}
	return out, rows.Err()
}

func (s *Store) lookupProviderEmoteIDs(ctx context.Context, localIDs []string, providers []string) (map[string]string, error) {
	out := map[string]string{}
	if s == nil || s.db == nil || len(localIDs) == 0 {
		return out, nil
	}
	unique := make([]string, 0, len(localIDs))
	seen := map[string]struct{}{}
	for _, id := range localIDs {
		id = strings.TrimSpace(id)
		if !emoteimage.IsLocalEmoteID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return out, nil
	}
	query := `
		SELECT id::text, provider_emote_id
		FROM emotes
		WHERE id = ANY($1::uuid[])
		  AND COALESCE(provider_emote_id, '') <> ''`
	args := []any{unique}
	if len(providers) > 0 {
		query += `
		  AND provider = ANY($2::text[])`
		args = append(args, providers)
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var localID, providerID string
		if err := rows.Scan(&localID, &providerID); err != nil {
			return nil, err
		}
		out[localID] = providerID
	}
	return out, rows.Err()
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func (s *Store) EnsureAlwaysTrackedTable(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS analytics_always_tracked (
			login TEXT PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func (s *Store) AddAlwaysTracked(ctx context.Context, login string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO analytics_always_tracked (login)
		VALUES ($1)
		ON CONFLICT (login) DO NOTHING
	`, login)
	return err
}

func (s *Store) RemoveAlwaysTracked(ctx context.Context, login string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM analytics_always_tracked
		WHERE login = $1
	`, login)
	return err
}

func (s *Store) GetAlwaysTracked(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT login FROM analytics_always_tracked ORDER BY login ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logins []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		logins = append(logins, login)
	}
	return logins, nil
}

func (s *Store) SaveGameSegments(ctx context.Context, streamID string, segments []GameSegment) error {
	if streamID == "" {
		return nil
	}
	canonicalID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		return err
	}
	streamID = canonicalID
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM stream_game_segments WHERE stream_id = $1`, streamID)
	if err != nil {
		return err
	}

	for _, seg := range segments {
		_, err = tx.Exec(ctx, `
			INSERT INTO stream_game_segments (stream_id, game_name, box_art_url, offset_seconds, duration_seconds)
			VALUES ($1, $2, $3, $4, $5)
		`, streamID, seg.GameName, seg.BoxArtURL, seg.OffsetSeconds, seg.DurationSeconds)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) GetGameSegments(ctx context.Context, streamID string) ([]GameSegment, error) {
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, stream_id, game_name, COALESCE(box_art_url, ''), offset_seconds, duration_seconds, created_at
		FROM stream_game_segments
		WHERE stream_id = $1
		ORDER BY offset_seconds ASC
	`, canonicalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segments []GameSegment
	for rows.Next() {
		var seg GameSegment
		err := rows.Scan(&seg.ID, &seg.StreamID, &seg.GameName, &seg.BoxArtURL, &seg.OffsetSeconds, &seg.DurationSeconds, &seg.CreatedAt)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	return segments, rows.Err()
}

func (s *Store) BulkUpsertMinuteRollups(ctx context.Context, streamID string, rollups []MinuteRollup, opts ...RollupWriteOptions) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
	}
	options := RollupWriteOptions{RefreshSummary: true, SummaryRefreshMode: "immediate"}
	if len(opts) > 0 {
		options = normalizeRollupWriteOptions(opts[0])
	}
	started := time.Now()
	result := "success"
	defer func() {
		metrics.AnalyticsRollupWriteDuration.WithLabelValues("bulk_upsert", result).Observe(time.Since(started).Seconds())
	}()
	resolvedStreamID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		result = "error"
		return err
	}
	streamID = resolvedStreamID

	tx, err := s.db.Begin(ctx)
	if err != nil {
		result = "error"
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	queued := 0
	for _, rollup := range rollups {
		if rollup.Emotes == nil {
			rollup.Emotes = map[string]int{}
		}
		emotes, err := json.Marshal(rollup.Emotes)
		if err != nil {
			result = "error"
			return err
		}
		chatSource, sourceConfidence, chatSourceDetail := bulkUpsertChatSourceForRollup(rollup)
		batch.Queue(`
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				viewer_avg=CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
				viewer_max=GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
				viewer_latest=CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
				viewer_samples=GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
				chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
				total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json=EXCLUDED.emotes_json,
				chat_source=CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_source
					WHEN EXCLUDED.chat_count > 0 THEN EXCLUDED.chat_source
					ELSE analytics_minute_rollups.chat_source
				END,
				source_confidence=CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.source_confidence
					WHEN EXCLUDED.chat_count > 0 THEN EXCLUDED.source_confidence
					ELSE analytics_minute_rollups.source_confidence
				END,
				chat_source_detail=CASE
					WHEN COALESCE(analytics_minute_rollups.source_confidence,'') = 'canonical' THEN analytics_minute_rollups.chat_source_detail
					WHEN EXCLUDED.chat_count > 0 THEN EXCLUDED.chat_source_detail
					ELSE analytics_minute_rollups.chat_source_detail
				END,
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
			rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
			chatSource, sourceConfidence, chatSourceDetail,
		)
		queued++
	}
	if queued == 0 {
		return nil
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if options.RefreshSummary {
		if err := refreshStreamSummaryObserved(ctx, tx, streamID, options.SummaryRefreshMode); err != nil {
			result = "error"
			return err
		}
	}
	if err := upsertMinutePeaksTx(ctx, tx, streamID, rollups, RollupChatSourceGQL, SourceConfidenceCanonical); err != nil {
		result = "error"
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	metrics.AnalyticsRollupRowsWrittenTotal.WithLabelValues("bulk_upsert").Add(float64(queued))
	metrics.AnalyticsRollupWriteBatchSize.WithLabelValues("bulk_upsert").Observe(float64(queued))
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) BulkPatchChatRollups(ctx context.Context, streamID string, rollups []MinuteRollup, opts ...RollupWriteOptions) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
	}
	options := RollupWriteOptions{RefreshSummary: true, SummaryRefreshMode: "immediate"}
	if len(opts) > 0 {
		options = normalizeRollupWriteOptions(opts[0])
	}
	started := time.Now()
	result := "success"
	defer func() {
		metrics.AnalyticsRollupWriteDuration.WithLabelValues("bulk_patch_chat", result).Observe(time.Since(started).Seconds())
	}()
	resolvedStreamID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		result = "error"
		return err
	}
	streamID = resolvedStreamID

	tx, err := s.db.Begin(ctx)
	if err != nil {
		result = "error"
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	queued := 0
	for _, rollup := range rollups {
		if rollup.Emotes == nil {
			rollup.Emotes = map[string]int{}
		}
		emotes, err := json.Marshal(rollup.Emotes)
		if err != nil {
			result = "error"
			return err
		}
		batch.Queue(`
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ($1,$2,0,0,0,0,$3,$4,$5,$6::jsonb,$7,$8,'')
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
				total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json=EXCLUDED.emotes_json,
				chat_source=EXCLUDED.chat_source,
				source_confidence=EXCLUDED.source_confidence,
				chat_source_detail='',
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount,
			string(emotes), RollupChatSourceGQL, SourceConfidenceCanonical,
		)
		queued++
	}
	if queued == 0 {
		return nil
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if options.RefreshSummary {
		if err := refreshStreamSummaryObserved(ctx, tx, streamID, options.SummaryRefreshMode); err != nil {
			result = "error"
			return err
		}
	}
	if err := upsertMinutePeaksTx(ctx, tx, streamID, rollups, RollupChatSourceGQL, SourceConfidenceCanonical); err != nil {
		result = "error"
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	metrics.AnalyticsRollupRowsWrittenTotal.WithLabelValues("bulk_patch_chat").Add(float64(queued))
	metrics.AnalyticsRollupWriteBatchSize.WithLabelValues("bulk_patch_chat").Observe(float64(queued))
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) BulkPatchViewerRollups(ctx context.Context, streamID string, rollups []MinuteRollup, opts ...RollupWriteOptions) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
	}
	options := RollupWriteOptions{RefreshSummary: true, SummaryRefreshMode: "immediate"}
	if len(opts) > 0 {
		options = normalizeRollupWriteOptions(opts[0])
	}
	started := time.Now()
	result := "success"
	defer func() {
		metrics.AnalyticsRollupWriteDuration.WithLabelValues("bulk_patch_viewer", result).Observe(time.Since(started).Seconds())
	}()
	resolvedStreamID, err := s.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		result = "error"
		return err
	}
	streamID = resolvedStreamID

	tx, err := s.db.Begin(ctx)
	if err != nil {
		result = "error"
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	queued := 0
	for _, rollup := range rollups {
		if rollup.ViewerAvg == 0 && rollup.ViewerMax == 0 && rollup.ViewerLatest == 0 {
			continue
		}
		batch.Queue(`
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json
			)
			VALUES ($1,$2,$3,$4,$5,$6,0,0,0,'{}'::jsonb)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				viewer_avg=CASE
					WHEN analytics_minute_rollups.viewer_samples > EXCLUDED.viewer_samples THEN analytics_minute_rollups.viewer_avg
					WHEN analytics_minute_rollups.viewer_samples > 0 AND EXCLUDED.viewer_samples <= analytics_minute_rollups.viewer_samples THEN analytics_minute_rollups.viewer_avg
					WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg
					ELSE analytics_minute_rollups.viewer_avg END,
				viewer_max=CASE
					WHEN analytics_minute_rollups.viewer_samples > EXCLUDED.viewer_samples THEN analytics_minute_rollups.viewer_max
					WHEN analytics_minute_rollups.viewer_samples > 0 AND EXCLUDED.viewer_samples <= analytics_minute_rollups.viewer_samples THEN analytics_minute_rollups.viewer_max
					ELSE GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max) END,
				viewer_latest=CASE
					WHEN analytics_minute_rollups.viewer_samples > EXCLUDED.viewer_samples THEN analytics_minute_rollups.viewer_latest
					WHEN analytics_minute_rollups.viewer_samples > 0 AND EXCLUDED.viewer_samples <= analytics_minute_rollups.viewer_samples THEN analytics_minute_rollups.viewer_latest
					WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest
					ELSE analytics_minute_rollups.viewer_latest END,
				viewer_samples=CASE
					WHEN analytics_minute_rollups.viewer_samples >= EXCLUDED.viewer_samples THEN analytics_minute_rollups.viewer_samples
					ELSE EXCLUDED.viewer_samples END,
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
		)
		queued++
	}
	if queued == 0 {
		return nil
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if options.RefreshSummary {
		if err := refreshStreamSummaryObserved(ctx, tx, streamID, options.SummaryRefreshMode); err != nil {
			result = "error"
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	metrics.AnalyticsRollupRowsWrittenTotal.WithLabelValues("bulk_patch_viewer").Add(float64(queued))
	metrics.AnalyticsRollupWriteBatchSize.WithLabelValues("bulk_patch_viewer").Observe(float64(queued))
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) BackfillTimeseries(ctx context.Context, batchSize int) (TimeseriesBackfillSummary, error) {
	var summary TimeseriesBackfillSummary
	if s == nil || s.db == nil || s.telemetry == nil {
		return summary, nil
	}
	if batchSize <= 0 {
		batchSize = defaultTimeseriesBackfillBatchSize
	}
	reporter, _ := s.telemetry.(timeseries.BackfillReporter)

	var streamCount, rollupCount int64
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT r.stream_id), COUNT(*)
		FROM analytics_minute_rollups r
		JOIN analytics_streams s ON s.stream_id = r.stream_id
		WHERE COALESCE(s.login, '') <> ''`).Scan(&streamCount, &rollupCount)
	if err != nil {
		if reporter != nil {
			reporter.StartBackfill(0, 0)
			reporter.FinishBackfill(err)
		}
		return summary, err
	}
	if streamCount > 0 {
		summary.StreamCount = uint64(streamCount)
	}
	if rollupCount > 0 {
		summary.RollupCount = uint64(rollupCount)
	}
	if reporter != nil {
		reporter.StartBackfill(summary.StreamCount, summary.RollupCount)
	}
	finish := func(err error) (TimeseriesBackfillSummary, error) {
		if reporter != nil {
			reporter.FinishBackfill(err)
		}
		return summary, err
	}
	if summary.RollupCount == 0 {
		return finish(nil)
	}

	rows, err := s.db.Query(ctx, `
		SELECT
			s.login,
			r.stream_id,
			COALESCE(s.title, ''),
			COALESCE(s.category, ''),
			s.started_at,
			r.minute_ts,
			r.viewer_avg,
			r.viewer_max,
			r.chat_count,
			r.total_emote_count,
			r.seventv_emote_count,
			COALESCE(r.emotes_json, '{}'::jsonb)
		FROM analytics_minute_rollups r
		JOIN analytics_streams s ON s.stream_id = r.stream_id
		WHERE COALESCE(s.login, '') <> ''
		ORDER BY s.started_at ASC, r.stream_id ASC, r.minute_ts ASC`)
	if err != nil {
		return finish(err)
	}
	defer rows.Close()

	batch := make([]timeseries.Rollup, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.telemetry.WriteRollups(ctx, batch); err != nil {
			return err
		}
		exported := uint64(len(batch))
		summary.ExportedCount += exported
		if reporter != nil {
			reporter.AddBackfillProgress(exported)
		}
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var rollup timeseries.Rollup
		var rawEmotes []byte
		if err := rows.Scan(
			&rollup.ChannelLogin,
			&rollup.StreamID,
			&rollup.StreamTitle,
			&rollup.StreamCategory,
			&rollup.StreamStartedAt,
			&rollup.MinuteTS,
			&rollup.ViewerAvg,
			&rollup.ViewerMax,
			&rollup.ChatCount,
			&rollup.TotalEmoteCount,
			&rollup.SevenTVEmoteCount,
			&rawEmotes,
		); err != nil {
			return finish(err)
		}
		rollup.ChannelLogin = normalizeLogin(rollup.ChannelLogin)
		if len(rawEmotes) > 0 {
			if err := json.Unmarshal(rawEmotes, &rollup.Emotes); err != nil {
				return finish(err)
			}
		}
		if rollup.Emotes == nil {
			rollup.Emotes = map[string]int{}
		}
		batch = append(batch, rollup)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return finish(err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return finish(err)
	}
	if err := flush(); err != nil {
		return finish(err)
	}
	return finish(nil)
}

func (s *Store) enqueueRollupTelemetry(ctx context.Context, streamID string, rollups []MinuteRollup) {
	if s == nil || s.telemetry == nil || streamID == "" || len(rollups) == 0 {
		return
	}
	meta, ok := s.streamMeta(ctx, streamID)
	if !ok {
		return
	}
	out := make([]timeseries.Rollup, 0, len(rollups))
	for _, rollup := range rollups {
		if rollup.MinuteTS.IsZero() {
			continue
		}
		out = append(out, timeseries.Rollup{
			ChannelLogin:      meta.login,
			StreamID:          streamID,
			StreamTitle:       meta.title,
			StreamCategory:    meta.category,
			StreamStartedAt:   meta.startedAt,
			MinuteTS:          rollup.MinuteTS,
			ViewerAvg:         rollup.ViewerAvg,
			ViewerMax:         rollup.ViewerMax,
			ChatCount:         rollup.ChatCount,
			TotalEmoteCount:   rollup.TotalEmoteCount,
			SevenTVEmoteCount: rollup.SevenTVEmoteCount,
			Emotes:            rollup.Emotes,
		})
	}
	s.telemetry.EnqueueRollups(out)
}

func (s *Store) streamMeta(ctx context.Context, streamID string) (streamMeta, bool) {
	if cached, ok := s.metaCache.Load(streamID); ok {
		if meta, ok := cached.(streamMeta); ok && meta.login != "" {
			return meta, true
		}
	}
	var login, title, category string
	var startedAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT login, COALESCE(title, ''), started_at, COALESCE(category, '')
		FROM analytics_streams
		WHERE stream_id=$1`, streamID).Scan(&login, &title, &startedAt, &category)
	if err != nil {
		return streamMeta{}, false
	}
	meta := streamMeta{
		login:     normalizeLogin(login),
		title:     title,
		category:  category,
		startedAt: startedAt,
	}
	if meta.login != "" {
		s.metaCache.Store(streamID, meta)
	}
	return meta, meta.login != ""
}
