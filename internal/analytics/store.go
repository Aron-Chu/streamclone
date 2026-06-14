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

	"streamclone/internal/metrics"
	"streamclone/internal/timeseries"
)

type Store struct {
	db         *pgxpool.Pool
	telemetry  timeseries.Writer
	loginCache sync.Map
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

func (s *Store) UpsertLiveStream(ctx context.Context, stream LiveStream, profile UserProfile, seenAt time.Time) error {
	if stream.StartedAt.IsZero() {
		stream.StartedAt = seenAt
	}
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
			stream_id, broadcaster_id, login, display_name, profile_image_url, description,
			title, category, tags, language, thumbnail_url, started_at, last_seen_at,
			current_viewers, peak_viewers, viewer_samples
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$14,1)
		ON CONFLICT (stream_id) DO UPDATE SET
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
			updated_at=now()`,
		stream.ID, broadcasterID, normalizeLogin(stream.Login), displayName, profile.ProfileImageURL, profile.Description,
		stream.Title, stream.GameName, string(tags), stream.Language, stream.ThumbnailURL, stream.StartedAt, seenAt, stream.ViewerCount,
	)
	if err == nil && stream.ID != "" {
		s.loginCache.Store(stream.ID, normalizeLogin(stream.Login))
	}
	return err
}

func (s *Store) CloseStream(ctx context.Context, streamID string, endedAt time.Time) error {
	if streamID == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET ended_at=COALESCE(ended_at, $2), updated_at=now()
		WHERE stream_id=$1`, streamID, endedAt)
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
	if err := refreshStreamSummary(ctx, tx, streamID); err != nil {
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

func (s *Store) LatestStreamByLogin(ctx context.Context, login string) (*StreamRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,'')
		FROM analytics_streams
		WHERE login=$1
		ORDER BY ended_at IS NULL DESC, started_at DESC
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
	rows, err := s.db.Query(ctx, `
		SELECT stream_id, broadcaster_id, login, COALESCE(display_name,''), COALESCE(profile_image_url,''),
			COALESCE(description,''), COALESCE(title,''), COALESCE(category,''), tags, COALESCE(language,''),
			COALESCE(thumbnail_url,''), started_at, ended_at, last_seen_at, current_viewers, avg_viewers,
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,'')
		FROM analytics_streams
		WHERE stream_id=$1`, streamID)
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
			peak_viewers, viewer_samples, chat_messages, total_emote_uses, seventv_emote_uses, COALESCE(vod_id,''), COALESCE(vod_source,'')
		FROM analytics_streams
		WHERE login=$1
		ORDER BY started_at DESC
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
	_, err := s.db.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category, started_at, last_seen_at, tags, peak_viewers
		)
		VALUES ($1, $2, $3, $3, $4, 'Live', $5, $5, '[]'::jsonb, 0)
		ON CONFLICT (stream_id) DO UPDATE SET
			broadcaster_id = CASE
				WHEN COALESCE(analytics_streams.broadcaster_id, '') = '' THEN EXCLUDED.broadcaster_id
				ELSE analytics_streams.broadcaster_id
		END,
			updated_at = now()`,
		streamID, broadcasterID, login, title, startedAt,
	)
	if err == nil {
		s.loginCache.Store(streamID, normalizeLogin(login))
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
		&rec.SevenTVEmoteUses, &rec.VodID, &rec.VodSource,
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
	if id == "" {
		return ""
	}
	// Rollup keys store the local emote service id (MinIO path), not the 7TV provider id.
	return "/emotes/" + id + "/1x.webp"
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

func (s *Store) BulkUpsertMinuteRollups(ctx context.Context, streamID string, rollups []MinuteRollup) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
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
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if err := refreshStreamSummary(ctx, tx, streamID); err != nil {
		result = "error"
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) BulkPatchChatRollups(ctx context.Context, streamID string, rollups []MinuteRollup) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
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
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if err := refreshStreamSummary(ctx, tx, streamID); err != nil {
		result = "error"
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) BulkPatchViewerRollups(ctx context.Context, streamID string, rollups []MinuteRollup) error {
	if streamID == "" || len(rollups) == 0 {
		return nil
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
				viewer_avg=CASE WHEN EXCLUDED.viewer_avg > 0 THEN EXCLUDED.viewer_avg ELSE analytics_minute_rollups.viewer_avg END,
				viewer_max=GREATEST(analytics_minute_rollups.viewer_max, EXCLUDED.viewer_max),
				viewer_latest=CASE WHEN EXCLUDED.viewer_latest > 0 THEN EXCLUDED.viewer_latest ELSE analytics_minute_rollups.viewer_latest END,
				viewer_samples=GREATEST(analytics_minute_rollups.viewer_samples, EXCLUDED.viewer_samples),
				updated_at=now()`,
			streamID, rollup.MinuteTS, rollup.ViewerAvg, rollup.ViewerMax, rollup.ViewerLatest, rollup.ViewerSamples,
		)
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		result = "error"
		return err
	}

	if err := refreshStreamSummary(ctx, tx, streamID); err != nil {
		result = "error"
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		result = "error"
		return err
	}
	s.enqueueRollupTelemetry(ctx, streamID, rollups)
	return nil
}

func (s *Store) enqueueRollupTelemetry(ctx context.Context, streamID string, rollups []MinuteRollup) {
	if s == nil || s.telemetry == nil || streamID == "" || len(rollups) == 0 {
		return
	}
	login := s.streamLogin(ctx, streamID)
	if login == "" {
		return
	}
	out := make([]timeseries.Rollup, 0, len(rollups))
	for _, rollup := range rollups {
		if rollup.MinuteTS.IsZero() {
			continue
		}
		out = append(out, timeseries.Rollup{
			ChannelLogin:      login,
			StreamID:          streamID,
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

func (s *Store) streamLogin(ctx context.Context, streamID string) string {
	if cached, ok := s.loginCache.Load(streamID); ok {
		if login, ok := cached.(string); ok && login != "" {
			return login
		}
	}
	var login string
	err := s.db.QueryRow(ctx, `SELECT login FROM analytics_streams WHERE stream_id=$1`, streamID).Scan(&login)
	if err != nil {
		return ""
	}
	login = normalizeLogin(login)
	if login != "" {
		s.loginCache.Store(streamID, login)
	}
	return login
}
