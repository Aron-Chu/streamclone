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
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
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

func (s *Store) UpsertLiveStream(ctx context.Context, stream LiveStream, profile UserProfile, seenAt time.Time) error {
	if stream.StartedAt.IsZero() {
		stream.StartedAt = seenAt
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
	broadcasterID := stream.BroadcasterID
	if broadcasterID == "" {
		broadcasterID = profile.ID
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
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
		ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
			viewer_avg=CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
			viewer_max=GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
			viewer_latest=CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
			viewer_samples=GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
			chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
			total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
			seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
			emotes_json=EXCLUDED.emotes_json,
			updated_at=now()`,
		streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
		rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
	)
	if err != nil {
		result = "error"
		return err
	}
	if err := refreshStreamSummaryObserved(ctx, tx, streamID, "single"); err != nil {
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

func (s *Store) GetStreamUpdatedAt(ctx context.Context, streamID string) (time.Time, error) {
	var updatedAt time.Time
	err := s.db.QueryRow(ctx,
		`SELECT updated_at FROM analytics_streams WHERE stream_id = $1`,
		streamID,
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
	rows, err := s.db.Query(ctx, `
		SELECT minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		FROM analytics_minute_rollups
		WHERE stream_id=$1
		ORDER BY minute_ts ASC`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinuteRollup
	for rows.Next() {
		var r MinuteRollup
		var raw []byte
		if err := rows.Scan(&r.MinuteTS, &r.ViewerAvg, &r.ViewerMax, &r.ViewerLatest, &r.ViewerSamples, &r.ChatCount, &r.TotalEmoteCount, &r.SevenTVEmoteCount, &raw); err != nil {
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

func (s *Store) SetStreamVodID(ctx context.Context, streamID, vodID, source string) error {
	if streamID == "" || vodID == "" {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		_, err := s.db.Exec(ctx, `
			UPDATE analytics_streams
			SET vod_id=$2, updated_at=now()
			WHERE stream_id=$1`, streamID, vodID)
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET vod_id=$2, vod_source=$3, updated_at=now()
		WHERE stream_id=$1`, streamID, vodID, source)
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
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET vod_source=$2, updated_at=now()
		WHERE stream_id=$1
		  AND COALESCE(vod_id,'')=''
		  AND COALESCE(vod_source,'') <> $2`, streamID, VodSourceUnlinked)
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
	var cp SyncCheckpoint
	err := s.db.QueryRow(ctx, `
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
	_, err := s.db.Exec(ctx, `
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
	_, err := s.db.Exec(ctx, `
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
	rows, err := s.db.Query(ctx, `
		SELECT id, stream_id, game_name, COALESCE(box_art_url, ''), offset_seconds, duration_seconds, created_at
		FROM stream_game_segments
		WHERE stream_id = $1
		ORDER BY offset_seconds ASC
	`, streamID)
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
				chat_count, total_emote_count, seventv_emote_count, emotes_json
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				viewer_avg=CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
				viewer_max=GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
				viewer_latest=CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
				viewer_samples=GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
				chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
				total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json=EXCLUDED.emotes_json,
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
			rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
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
				chat_count, total_emote_count, seventv_emote_count, emotes_json
			)
			VALUES ($1,$2,0,0,0,0,$3,$4,$5,$6::jsonb)
			ON CONFLICT (stream_id, minute_ts) DO UPDATE SET
				chat_count=GREATEST(analytics_minute_rollups.chat_count, EXCLUDED.chat_count),
				total_emote_count=GREATEST(analytics_minute_rollups.total_emote_count, EXCLUDED.total_emote_count),
				seventv_emote_count=GREATEST(analytics_minute_rollups.seventv_emote_count, EXCLUDED.seventv_emote_count),
				emotes_json=EXCLUDED.emotes_json,
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ChatCount, rollup.TotalEmoteCount, rollup.SevenTVEmoteCount, string(emotes),
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
