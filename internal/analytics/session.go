package analytics

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"streamclone/internal/canonicalstream"
)

const (
	ViewerSourceLive     = "live"
	ViewerSourceTT       = "tt"
	ViewerSourceMerged   = "merged"
	ViewerSourceRestored = "restored"
	ViewerSourceUnknown  = "unknown"

	sessionOverlapGrace = 15 * time.Minute
)

// SessionResolveInput describes an incoming stream row before persistence.
type SessionResolveInput struct {
	Login          string
	StreamID       string
	TwitchStreamID string
	TTStreamID     string
	VodID          string
	StartedAt      time.Time
	EndedAt        *time.Time
	Title          string
	Category       string
	Source         string
	IsPlaceholder  bool
}

// SessionResolveResult is the canonical stream id callers should use for writes.
type SessionResolveResult struct {
	CanonicalStreamID string
	MergedFrom        string
	Created           bool
	ViewerSource      string
}

type sessionCandidate struct {
	StreamID       string
	CanonicalID    string
	Login          string
	TwitchStreamID string
	TTStreamID     string
	VodID          string
	StartedAt      time.Time
	EndedAt        *time.Time
	ViewerSource   string
	BroadcasterID  string
	ViewerSamples  int
	ChatMessages   int64
	PeakViewers    int
	Title          string
	IsPlaceholder  bool
}

type canonicalQuerier = canonicalstream.Querier
type canonicalResolution = canonicalstream.Resolution

func windowsOverlap(aStart time.Time, aEnd *time.Time, bStart time.Time, bEnd *time.Time) bool {
	if aStart.IsZero() || bStart.IsZero() {
		return false
	}
	aEndAt := openEnded(aEnd)
	bEndAt := openEnded(bEnd)
	aStart = aStart.Add(-sessionOverlapGrace)
	aEndAt = aEndAt.Add(sessionOverlapGrace)
	bStart = bStart.Add(-sessionOverlapGrace)
	bEndAt = bEndAt.Add(sessionOverlapGrace)
	return aStart.Before(bEndAt) && bStart.Before(aEndAt)
}

func openEnded(endedAt *time.Time) time.Time {
	if endedAt != nil && !endedAt.IsZero() {
		return endedAt.UTC()
	}
	return time.Now().UTC().Add(24 * time.Hour)
}

func normalizeViewerSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case ViewerSourceLive:
		return ViewerSourceLive
	case ViewerSourceTT, "twitchtracker":
		return ViewerSourceTT
	case ViewerSourceMerged:
		return ViewerSourceMerged
	case ViewerSourceRestored:
		return ViewerSourceRestored
	default:
		return ViewerSourceUnknown
	}
}

func mergeViewerSources(existing, incoming string) string {
	existing = normalizeViewerSource(existing)
	incoming = normalizeViewerSource(incoming)
	if existing == ViewerSourceRestored || incoming == ViewerSourceRestored {
		return ViewerSourceRestored
	}
	if existing == incoming {
		return existing
	}
	if existing == ViewerSourceUnknown {
		return incoming
	}
	if incoming == ViewerSourceUnknown {
		return existing
	}
	return ViewerSourceMerged
}

func sessionsMatch(a, b sessionCandidate) bool {
	if normalizeLogin(a.Login) != normalizeLogin(b.Login) {
		return false
	}
	if a.TwitchStreamID != "" && b.TwitchStreamID != "" && a.TwitchStreamID == b.TwitchStreamID {
		return true
	}
	if a.TwitchStreamID != "" && b.TwitchStreamID != "" {
		return false
	}
	if a.TTStreamID != "" && b.TTStreamID != "" && a.TTStreamID == b.TTStreamID {
		return true
	}
	if a.VodID != "" && b.VodID != "" && a.VodID == b.VodID {
		return true
	}
	if a.StreamID != "" && (a.StreamID == b.CanonicalID || a.CanonicalID == b.StreamID) {
		return true
	}
	return windowsOverlap(a.StartedAt, a.EndedAt, b.StartedAt, b.EndedAt)
}

func sessionCandidateScore(c sessionCandidate) int {
	if c.IsPlaceholder {
		return 0
	}
	score := c.ViewerSamples*10 + int(c.ChatMessages)
	if c.BroadcasterID != "" && c.BroadcasterID != "pending" {
		score += 1000
	}
	if c.PeakViewers > 0 {
		score += 100
	}
	if strings.TrimSpace(c.Title) != "" && !isPlaceholderStreamTitle(c.Title) {
		score += 10
	}
	return score
}

func pickCanonicalSession(existing, incoming sessionCandidate) sessionCandidate {
	incomingScore := sessionCandidateScore(incoming)
	existingScore := sessionCandidateScore(existing)
	if incomingScore > existingScore {
		return incoming
	}
	if incomingScore == existingScore && incoming.StreamID != "" && (existing.StreamID == "" || incoming.StreamID < existing.StreamID) {
		return incoming
	}
	return existing
}

func isPlaceholderStreamTitle(title string) bool {
	trimmed := strings.TrimSpace(title)
	return trimmed == "" || trimmed == "Syncing..." || trimmed == "Syncing…"
}

func candidateFromInput(in SessionResolveInput) sessionCandidate {
	twitchID := strings.TrimSpace(in.TwitchStreamID)
	if twitchID == "" && !in.IsPlaceholder {
		twitchID = strings.TrimSpace(in.StreamID)
	}
	ttID := strings.TrimSpace(in.TTStreamID)
	if ttID == "" && in.IsPlaceholder {
		ttID = strings.TrimSpace(in.StreamID)
	}
	streamID := strings.TrimSpace(in.StreamID)
	return sessionCandidate{
		StreamID:       streamID,
		CanonicalID:    streamID,
		Login:          normalizeLogin(in.Login),
		TwitchStreamID: twitchID,
		TTStreamID:     ttID,
		VodID:          strings.TrimSpace(in.VodID),
		StartedAt:      in.StartedAt,
		EndedAt:        in.EndedAt,
		ViewerSource:   normalizeViewerSource(in.Source),
		Title:          strings.TrimSpace(in.Title),
		IsPlaceholder:  in.IsPlaceholder,
	}
}

// ResolveOrCreateSession finds or creates the canonical analytics session for a stream row.
func (s *Store) ResolveOrCreateSession(ctx context.Context, in SessionResolveInput) (SessionResolveResult, error) {
	if s == nil || s.db == nil {
		return SessionResolveResult{}, errors.New("analytics store unavailable")
	}
	streamID := strings.TrimSpace(in.StreamID)
	if streamID == "" || normalizeLogin(in.Login) == "" {
		return SessionResolveResult{}, errors.New("missing stream session identity")
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = time.Now().UTC()
	}

	incoming := candidateFromInput(in)
	candidates, err := s.loadSessionCandidates(ctx, incoming.Login, incoming.StartedAt)
	if err != nil {
		return SessionResolveResult{}, err
	}

	var match *sessionCandidate
	for i := range candidates {
		if !sessionsMatch(incoming, candidates[i]) {
			continue
		}
		copy := candidates[i]
		if match == nil {
			match = &copy
			continue
		}
		winner := pickCanonicalSession(*match, copy)
		loserID := match.StreamID
		if winner.StreamID == match.StreamID {
			loserID = copy.StreamID
		}
		if loserID != winner.StreamID && loserID != "" {
			if err := s.linkSessionAlias(ctx, loserID, winner.StreamID); err != nil {
				return SessionResolveResult{}, err
			}
		}
		*match = winner
	}

	if match == nil {
		source := incoming.ViewerSource
		if err := s.insertSession(ctx, incoming, source); err != nil {
			return SessionResolveResult{}, err
		}
		return SessionResolveResult{
			CanonicalStreamID: streamID,
			Created:           true,
			ViewerSource:      source,
		}, nil
	}

	winner := pickCanonicalSession(*match, incoming)
	mergedFrom := ""
	if match.StreamID != incoming.StreamID {
		loserID := match.StreamID
		if winner.StreamID == match.StreamID {
			loserID = incoming.StreamID
			mergedFrom = incoming.StreamID
		}
		if loserID != winner.StreamID && loserID != "" {
			if err := s.linkSessionAlias(ctx, loserID, winner.StreamID); err != nil {
				return SessionResolveResult{}, err
			}
		}
	}
	source := mergeViewerSources(match.ViewerSource, incoming.ViewerSource)
	if err := s.updateSession(ctx, winner.StreamID, incoming, source); err != nil {
		return SessionResolveResult{}, err
	}
	return SessionResolveResult{
		CanonicalStreamID: winner.StreamID,
		MergedFrom:        mergedFrom,
		ViewerSource:      source,
	}, nil
}

func (s *Store) loadSessionCandidates(ctx context.Context, login string, around time.Time) ([]sessionCandidate, error) {
	windowStart := around.Add(-48 * time.Hour)
	windowEnd := around.Add(48 * time.Hour)
	rows, err := s.db.Query(ctx, `
		SELECT
			st.stream_id,
			COALESCE(NULLIF(st.canonical_stream_id,''), st.stream_id),
			st.login,
			CASE WHEN st.broadcaster_id <> 'pending' THEN st.stream_id ELSE '' END,
			COALESCE(st.vod_id, ''),
			st.started_at,
			st.ended_at,
			COALESCE(st.viewer_source, 'unknown'),
			COALESCE(st.broadcaster_id, ''),
			st.viewer_samples,
			st.chat_messages,
			st.peak_viewers,
			COALESCE(st.title, ''),
			CASE WHEN st.broadcaster_id = 'pending' AND st.viewer_samples = 0 AND st.chat_messages = 0 THEN true ELSE false END
		FROM analytics_streams st
		WHERE st.login = $1
		  AND st.started_at >= $2
		  AND st.started_at <= $3
		ORDER BY st.started_at DESC`,
		normalizeLogin(login), windowStart, windowEnd,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []sessionCandidate
	for rows.Next() {
		var c sessionCandidate
		var endedAt *time.Time
		if err := rows.Scan(
			&c.StreamID, &c.CanonicalID, &c.Login, &c.TwitchStreamID, &c.VodID,
			&c.StartedAt, &endedAt, &c.ViewerSource, &c.BroadcasterID,
			&c.ViewerSamples, &c.ChatMessages, &c.PeakViewers, &c.Title, &c.IsPlaceholder,
		); err != nil {
			return nil, err
		}
		c.EndedAt = endedAt
		c.TTStreamID = c.StreamID
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) insertSession(ctx context.Context, c sessionCandidate, source string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO analytics_stream_sessions (
			canonical_stream_id, login, twitch_stream_id, tt_stream_id, vod_id,
			started_at, ended_at, title, viewer_source, source_confidence
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'resolve')
		ON CONFLICT (canonical_stream_id) DO UPDATE SET
			twitch_stream_id = CASE WHEN EXCLUDED.twitch_stream_id <> '' THEN EXCLUDED.twitch_stream_id ELSE analytics_stream_sessions.twitch_stream_id END,
			tt_stream_id = CASE WHEN EXCLUDED.tt_stream_id <> '' THEN EXCLUDED.tt_stream_id ELSE analytics_stream_sessions.tt_stream_id END,
			vod_id = CASE WHEN EXCLUDED.vod_id <> '' THEN EXCLUDED.vod_id ELSE analytics_stream_sessions.vod_id END,
			viewer_source = EXCLUDED.viewer_source,
			updated_at = now()`,
		c.StreamID, normalizeLogin(c.Login), c.TwitchStreamID, c.TTStreamID, c.VodID,
		c.StartedAt, c.EndedAt, nullIfEmpty(c.Title), source,
	)
	return err
}

func (s *Store) updateSession(ctx context.Context, canonicalID string, c sessionCandidate, source string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_stream_sessions SET
			twitch_stream_id = CASE WHEN $2 <> '' THEN $2 ELSE twitch_stream_id END,
			tt_stream_id = CASE WHEN $3 <> '' THEN $3 ELSE tt_stream_id END,
			vod_id = CASE WHEN $4 <> '' THEN $4 ELSE vod_id END,
			started_at = LEAST(started_at, $5),
			ended_at = COALESCE(ended_at, $6),
			title = COALESCE(NULLIF($7, ''), title),
			viewer_source = $8,
			updated_at = now()
		WHERE canonical_stream_id = $1`,
		canonicalID, c.TwitchStreamID, c.TTStreamID, c.VodID, c.StartedAt, c.EndedAt,
		nullIfEmpty(c.Title), source,
	)
	return err
}

func (s *Store) linkSessionAlias(ctx context.Context, aliasID, canonicalID string) error {
	aliasID = strings.TrimSpace(aliasID)
	canonicalID = strings.TrimSpace(canonicalID)
	if aliasID == "" || canonicalID == "" || aliasID == canonicalID {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockSessionAliasIDs(ctx, tx, aliasID, canonicalID); err != nil {
		return err
	}

	aliasResolved, err := resolveCanonicalStream(ctx, tx, aliasID)
	if err != nil {
		return err
	}
	canonicalResolved, err := resolveCanonicalStream(ctx, tx, canonicalID)
	if err != nil {
		return err
	}
	lockIDs := uniqueStrings(append(append([]string{}, aliasResolved.Path...), canonicalResolved.Path...))
	lockIDs = append(lockIDs, aliasResolved.CanonicalID, canonicalResolved.CanonicalID)
	if err := lockSessionAliasIDs(ctx, tx, lockIDs...); err != nil {
		return err
	}

	aliasResolved, err = resolveCanonicalStream(ctx, tx, aliasID)
	if err != nil {
		return err
	}
	canonicalResolved, err = resolveCanonicalStream(ctx, tx, canonicalID)
	if err != nil {
		return err
	}

	if aliasResolved.CycleDetected || canonicalResolved.CycleDetected {
		cycleIDs := append([]string{}, aliasResolved.Cycle...)
		cycleIDs = append(cycleIDs, canonicalResolved.Cycle...)
		targetID := canonicalResolved.CanonicalID
		if targetID == "" || canonicalResolved.CycleDetected {
			var err error
			targetID, err = s.pickCanonicalStreamID(ctx, tx, uniqueStrings(cycleIDs), canonicalID)
			if err != nil {
				return err
			}
		}
		if err := s.repairSessionAliasCycleTx(ctx, tx, uniqueStrings(cycleIDs), targetID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	targetID := canonicalResolved.CanonicalID
	if targetID == "" {
		targetID = canonicalID
	}
	losingIDs := uniqueStrings(append(aliasResolved.Path, aliasResolved.CanonicalID))
	var filtered []string
	for _, id := range losingIDs {
		if id != "" && id != targetID {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		if err := s.flattenSessionAliasTx(ctx, tx, aliasID, targetID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := s.mergeSessionAliasesTx(ctx, tx, filtered, targetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) rekeyStreamChildRows(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	if fromID == "" || toID == "" || fromID == toID {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO analytics_minute_rollups (
			stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		)
		SELECT $2, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		FROM analytics_minute_rollups
		WHERE stream_id = $1
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
		fromID, toID,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM analytics_minute_rollups WHERE stream_id = $1`, fromID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE stream_game_segments SET stream_id = $2 WHERE stream_id = $1 AND NOT EXISTS (
			SELECT 1 FROM stream_game_segments existing
			WHERE existing.stream_id = $2 AND existing.offset_seconds = stream_game_segments.offset_seconds
		)`, fromID, toID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM stream_game_segments WHERE stream_id = $1`, fromID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO analytics_sync_checkpoints (
			stream_id, video_id, cursor, offset_seconds, comments_fetched, segments_json, fetch_mode, updated_at
		)
		SELECT $2, video_id, cursor, offset_seconds, comments_fetched, segments_json, fetch_mode, updated_at
		FROM analytics_sync_checkpoints
		WHERE stream_id = $1
		ON CONFLICT (stream_id, video_id) DO UPDATE SET
			cursor = CASE
				WHEN EXCLUDED.comments_fetched > analytics_sync_checkpoints.comments_fetched THEN EXCLUDED.cursor
				WHEN EXCLUDED.comments_fetched = analytics_sync_checkpoints.comments_fetched
				 AND EXCLUDED.offset_seconds >= analytics_sync_checkpoints.offset_seconds THEN EXCLUDED.cursor
				ELSE analytics_sync_checkpoints.cursor END,
			offset_seconds = GREATEST(analytics_sync_checkpoints.offset_seconds, EXCLUDED.offset_seconds),
			comments_fetched = GREATEST(analytics_sync_checkpoints.comments_fetched, EXCLUDED.comments_fetched),
			segments_json = CASE
				WHEN EXCLUDED.segments_json <> ''
				 AND (EXCLUDED.comments_fetched > analytics_sync_checkpoints.comments_fetched
				  OR (EXCLUDED.comments_fetched = analytics_sync_checkpoints.comments_fetched
				   AND EXCLUDED.offset_seconds >= analytics_sync_checkpoints.offset_seconds))
				THEN EXCLUDED.segments_json
				ELSE analytics_sync_checkpoints.segments_json END,
			fetch_mode = CASE
				WHEN EXCLUDED.fetch_mode <> ''
				 AND (EXCLUDED.comments_fetched > analytics_sync_checkpoints.comments_fetched
				  OR (EXCLUDED.comments_fetched = analytics_sync_checkpoints.comments_fetched
				   AND EXCLUDED.offset_seconds >= analytics_sync_checkpoints.offset_seconds))
				THEN EXCLUDED.fetch_mode
				ELSE analytics_sync_checkpoints.fetch_mode END,
			updated_at = GREATEST(analytics_sync_checkpoints.updated_at, EXCLUDED.updated_at)`,
		fromID, toID,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM analytics_sync_checkpoints WHERE stream_id = $1`, fromID)
	return err
}

func (s *Store) ResolveCanonicalStreamID(ctx context.Context, streamID string) (string, error) {
	if s == nil || s.db == nil || strings.TrimSpace(streamID) == "" {
		return streamID, nil
	}
	resolved, err := resolveCanonicalStream(ctx, s.db, streamID)
	if err != nil {
		return streamID, err
	}
	if resolved.CanonicalID == "" {
		return streamID, nil
	}
	return resolved.CanonicalID, nil
}

func (s *Store) SetStreamViewerSource(ctx context.Context, streamID, source string) error {
	if streamID == "" {
		return nil
	}
	source = normalizeViewerSource(source)
	_, err := s.db.Exec(ctx, `
		UPDATE analytics_streams
		SET viewer_source = $2, updated_at = now()
		WHERE stream_id = $1`, streamID, source)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		UPDATE analytics_stream_sessions
		SET viewer_source = $2, updated_at = now()
		WHERE canonical_stream_id = $1`, streamID, source)
	return err
}

func nullIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.TrimSpace(value)
}

// ResolveStreamIDForWrite maps an alias stream id to its canonical id before writes.
func (s *Store) ResolveStreamIDForWrite(ctx context.Context, streamID string) (string, error) {
	canonical, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return streamID, fmt.Errorf("resolve canonical stream: %w", err)
	}
	return canonical, nil
}

func resolveCanonicalStream(ctx context.Context, q canonicalQuerier, streamID string) (canonicalResolution, error) {
	return canonicalstream.Resolve(ctx, q, streamID)
}

func stableCanonicalID(ids []string) string {
	return canonicalstream.StableID(ids)
}

func uniqueStrings(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func lockSessionAliasIDs(ctx context.Context, tx pgx.Tx, ids ...string) error {
	ids = uniqueStrings(ids)
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pickCanonicalStreamID(ctx context.Context, q canonicalQuerier, ids []string, preferred string) (string, error) {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return strings.TrimSpace(preferred), nil
	}
	rows, err := q.Query(ctx, `
		SELECT
			stream_id,
			COALESCE(NULLIF(canonical_stream_id,''), stream_id),
			login,
			CASE WHEN broadcaster_id <> 'pending' THEN stream_id ELSE '' END,
			COALESCE(vod_id, ''),
			started_at,
			ended_at,
			COALESCE(viewer_source, 'unknown'),
			COALESCE(broadcaster_id, ''),
			viewer_samples,
			chat_messages,
			peak_viewers,
			COALESCE(title, ''),
			CASE WHEN broadcaster_id = 'pending' AND viewer_samples = 0 AND chat_messages = 0 THEN true ELSE false END
		FROM analytics_streams
		WHERE stream_id = ANY($1)`, ids)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var winner *sessionCandidate
	for rows.Next() {
		var c sessionCandidate
		var endedAt *time.Time
		if err := rows.Scan(
			&c.StreamID, &c.CanonicalID, &c.Login, &c.TwitchStreamID, &c.VodID,
			&c.StartedAt, &endedAt, &c.ViewerSource, &c.BroadcasterID,
			&c.ViewerSamples, &c.ChatMessages, &c.PeakViewers, &c.Title, &c.IsPlaceholder,
		); err != nil {
			return "", err
		}
		c.EndedAt = endedAt
		c.TTStreamID = c.StreamID
		if winner == nil {
			copy := c
			winner = &copy
			continue
		}
		picked := pickCanonicalSession(*winner, c)
		*winner = picked
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if winner != nil && winner.StreamID != "" {
		return winner.StreamID, nil
	}
	return stableCanonicalID(append(ids, preferred)), nil
}

func (s *Store) repairSessionAliasCycleTx(ctx context.Context, tx pgx.Tx, cycleIDs []string, targetID string) error {
	cycleIDs = uniqueStrings(cycleIDs)
	targetID = strings.TrimSpace(targetID)
	if len(cycleIDs) == 0 || targetID == "" {
		return nil
	}
	if err := s.ensureSessionForStreamTx(ctx, tx, targetID); err != nil {
		return err
	}
	var losing []string
	for _, id := range cycleIDs {
		if id != targetID {
			losing = append(losing, id)
		}
	}
	return s.mergeSessionAliasesTx(ctx, tx, losing, targetID)
}

func (s *Store) mergeSessionAliasesTx(ctx context.Context, tx pgx.Tx, losingIDs []string, targetID string) error {
	losingIDs = uniqueStrings(losingIDs)
	targetID = strings.TrimSpace(targetID)
	if len(losingIDs) == 0 || targetID == "" {
		return nil
	}
	if err := s.ensureSessionForStreamTx(ctx, tx, targetID); err != nil {
		return err
	}
	for _, losingID := range losingIDs {
		if losingID == targetID {
			continue
		}
		if err := s.rekeyStreamChildRows(ctx, tx, losingID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE analytics_stream_aliases
			SET canonical_stream_id = $2
			WHERE canonical_stream_id = $1`, losingID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM analytics_stream_aliases
			WHERE alias_stream_id = $1 OR (alias_stream_id = $2 AND canonical_stream_id = $1)`,
			targetID, losingID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO analytics_stream_aliases (alias_stream_id, canonical_stream_id, alias_kind)
			VALUES ($1, $2, 'dedupe')
			ON CONFLICT (alias_stream_id) DO UPDATE SET canonical_stream_id = EXCLUDED.canonical_stream_id`,
			losingID, targetID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE analytics_streams
			SET canonical_stream_id = $2, updated_at = now()
			WHERE stream_id = $1`, losingID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM analytics_streams WHERE stream_id = $1 AND stream_id <> $2`, losingID, targetID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) flattenSessionAliasTx(ctx context.Context, tx pgx.Tx, aliasID, targetID string) error {
	aliasID = strings.TrimSpace(aliasID)
	targetID = strings.TrimSpace(targetID)
	if aliasID == "" || targetID == "" || aliasID == targetID {
		return nil
	}
	if err := s.ensureSessionForStreamTx(ctx, tx, targetID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO analytics_stream_aliases (alias_stream_id, canonical_stream_id, alias_kind)
		VALUES ($1, $2, 'dedupe')
		ON CONFLICT (alias_stream_id) DO UPDATE SET canonical_stream_id = EXCLUDED.canonical_stream_id`,
		aliasID, targetID,
	)
	return err
}

func (s *Store) ensureSessionForStreamTx(ctx context.Context, tx pgx.Tx, streamID string) error {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil
	}
	targetID := streamID
	if resolved, err := resolveCanonicalStream(ctx, tx, streamID); err == nil && strings.TrimSpace(resolved.CanonicalID) != "" {
		targetID = strings.TrimSpace(resolved.CanonicalID)
	}
	var canonicalExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM analytics_streams WHERE stream_id = $1)`, targetID,
	).Scan(&canonicalExists); err != nil {
		return err
	}
	if !canonicalExists {
		return fmt.Errorf("canonical stream row %s missing from analytics_streams", targetID)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO analytics_stream_sessions (
			canonical_stream_id, login, twitch_stream_id, tt_stream_id, vod_id,
			started_at, ended_at, title, category, viewer_source, source_confidence
		)
		SELECT
			stream_id,
			login,
			CASE WHEN broadcaster_id <> 'pending' THEN stream_id ELSE '' END,
			CASE WHEN broadcaster_id = 'pending' THEN stream_id ELSE '' END,
			COALESCE(vod_id, ''),
			started_at,
			ended_at,
			title,
			category,
			COALESCE(viewer_source, 'unknown'),
			'alias_repair'
		FROM analytics_streams
		WHERE stream_id = $1
		ON CONFLICT (canonical_stream_id) DO NOTHING`, targetID)
	return err
}
