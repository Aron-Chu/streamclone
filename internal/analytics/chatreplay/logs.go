package chatreplay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/archive"
)

type UnifiedQueryParams struct {
	Channel    string
	StreamID   string
	Q          string
	User       string
	SenderHash string
	Limit      int
	Cursor     string
}

type UnifiedQueryResult struct {
	Entries    []UnifiedLogEntry
	NextCursor string
}

func clampLimit(limit int) int {
	if limit <= 0 {
		limit = DefaultPageLimit
	}
	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}
	return limit
}

// ListChannelChatLogs returns synced streams with VOD chat coverage for a channel login.
func (s *Store) ListChannelChatLogs(ctx context.Context, login string) ([]ChannelChatLogStream, error) {
	if s == nil || s.db == nil || login == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT s.stream_id,
		       COALESCE(s.title, ''),
		       s.started_at,
		       s.ended_at,
		       COUNT(v.id) AS message_count,
		       COALESCE(MIN(v.offset_seconds), 0),
		       COALESCE(MAX(v.offset_seconds), 0)
		FROM analytics_streams s
		JOIN analytics_vod_chat_messages v ON v.stream_id = s.stream_id
		WHERE LOWER(s.login) = LOWER($1)
		GROUP BY s.stream_id, s.title, s.started_at, s.ended_at
		ORDER BY s.started_at DESC`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChannelChatLogStream, 0)
	for rows.Next() {
		var row ChannelChatLogStream
		var endedAt *time.Time
		if err := rows.Scan(&row.StreamID, &row.Title, &row.StartedAt, &endedAt, &row.MessageCount, &row.FirstOffset, &row.LastOffset); err != nil {
			return nil, err
		}
		row.EndedAt = endedAt
		row.Source = "vod"
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) InsertLiveMessage(ctx context.Context, msg LiveChatMessage) error {
	if s == nil || s.db == nil || msg.Channel == "" || msg.MessageID == "" {
		return nil
	}
	frags, err := marshalFrags(asEmoteFrags(msg.Fragments))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO live_chat_messages (channel, login, display_name, message_id, text, fragments, ts)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (channel, message_id) DO NOTHING`,
		strings.ToLower(msg.Channel), msg.Login, msg.DisplayName, msg.MessageID, msg.Text, frags, msg.TS.UTC(),
	)
	return err
}

func (s *Store) InsertModEvent(ctx context.Context, ev ChatModEvent) error {
	if s == nil || s.db == nil || ev.Channel == "" || ev.Kind == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO chat_mod_events (channel, kind, actor_login, target_login, duration_sec, reason, message_id, text_preview, ts)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,0),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`,
		strings.ToLower(ev.Channel), ev.Kind, ev.ActorLogin, ev.TargetLogin, ev.DurationSec, ev.Reason, ev.MessageID, ev.TextPreview, ev.TS.UTC(),
	)
	return err
}

func asEmoteFrags(frags []EmoteFrag) []EmoteFrag {
	if frags == nil {
		return []EmoteFrag{}
	}
	return frags
}

// QueryUnified returns a merged page of VOD, live, and mod events for the logs viewer.
func (s *Store) QueryUnified(ctx context.Context, params UnifiedQueryParams) (UnifiedQueryResult, error) {
	if s == nil || s.db == nil || params.Channel == "" {
		return UnifiedQueryResult{}, nil
	}
	limit := clampLimit(params.Limit)
	streamID := strings.TrimSpace(params.StreamID)
	isLive := strings.EqualFold(streamID, "live")
	isAll := streamID == "" || strings.EqualFold(streamID, "all")

	var entries []UnifiedLogEntry
	var nextCursor string
	var err error

	switch {
	case isLive:
		entries, nextCursor, err = s.queryLiveMessages(ctx, params, limit)
	case !isAll:
		vod, err := s.Query(ctx, QueryParams{
			StreamID:   streamID,
			Limit:      limit,
			Cursor:     params.Cursor,
			Q:          params.Q,
			User:       params.User,
			SenderHash: params.SenderHash,
		})
		if err != nil {
			return UnifiedQueryResult{}, err
		}
		streamMeta, _ := s.streamMetaForID(ctx, streamID)
		for _, msg := range vod.Messages {
			entry := vodToUnified(msg)
			if streamMeta != nil {
				entry.StreamID = streamMeta.StreamID
				entry.StreamTitle = streamMeta.Title
				started := streamMeta.StartedAt
				entry.StreamStartedAt = &started
			}
			entries = append(entries, entry)
		}
		nextCursor = vod.NextCursor
	default:
		entries, nextCursor, err = s.queryChannelVOD(ctx, params, limit)
	}

	if err != nil {
		return UnifiedQueryResult{}, err
	}

	modEvents, err := s.queryModEvents(ctx, params.Channel, "", limit)
	if err != nil {
		return UnifiedQueryResult{}, err
	}
	entries = mergeUnifiedTimeline(entries, modEvents, 0)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return UnifiedQueryResult{Entries: entries, NextCursor: nextCursor}, nil
}

func vodToUnified(msg VODChatMessage) UnifiedLogEntry {
	login := msg.CommenterLogin
	if login == "" {
		login = strings.ToLower(msg.DisplayName)
	}
	return UnifiedLogEntry{
		Kind:          "message",
		ID:            msg.ID,
		TS:            msg.MinuteTS,
		OffsetSeconds: msg.OffsetSeconds,
		DisplayName:   msg.DisplayName,
		Login:         login,
		SenderHash:    msg.SenderHash,
		MessageID:     msg.MessageID,
		Text:          msg.Text,
		EmoteFrags:    msg.EmoteFrags,
		Source:        "vod",
		StreamID:      msg.StreamID,
	}
}

type streamMeta struct {
	StreamID  string
	Title     string
	StartedAt time.Time
}

func (s *Store) streamMetaForID(ctx context.Context, streamID string) (*streamMeta, error) {
	if s == nil || s.db == nil || streamID == "" {
		return nil, nil
	}
	var meta streamMeta
	err := s.db.QueryRow(ctx, `
		SELECT stream_id, COALESCE(title, ''), started_at
		FROM analytics_streams
		WHERE stream_id = $1`, streamID).Scan(&meta.StreamID, &meta.Title, &meta.StartedAt)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *Store) queryChannelVOD(ctx context.Context, params UnifiedQueryParams, limit int) ([]UnifiedLogEntry, string, error) {
	login := strings.ToLower(strings.TrimSpace(params.Channel))
	args := []any{login}
	where := []string{"LOWER(s.login) = LOWER($1)"}

	if params.SenderHash != "" {
		args = append(args, params.SenderHash)
		where = append(where, fmt.Sprintf("v.sender_hash = $%d", len(args)))
	}
	if params.User != "" {
		args = append(args, params.User)
		where = append(where, fmt.Sprintf("(v.display_name ILIKE $%d OR v.commenter_login ILIKE $%d)", len(args), len(args)))
	}
	if params.Q != "" {
		args = append(args, "%"+params.Q+"%")
		where = append(where, fmt.Sprintf("v.text ILIKE $%d", len(args)))
	}
	if params.Cursor != "" {
		curStarted, curStreamID, curOffset, curID, err := decodeChannelCursor(params.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("chatreplay: invalid cursor: %w", err)
		}
		args = append(args, curStarted, curStreamID, curOffset, curID)
		where = append(where, fmt.Sprintf(
			"(s.started_at, v.stream_id, v.offset_seconds, v.id) > ($%d, $%d, $%d, $%d)",
			len(args)-3, len(args)-2, len(args)-1, len(args),
		))
	}

	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT v.id, v.stream_id, v.minute_ts, v.message_id, v.display_name, COALESCE(v.commenter_login,''), v.sender_hash, v.text, v.emote_frags, v.offset_seconds, v.synced_at,
		       s.started_at, COALESCE(s.title, '')
		FROM analytics_vod_chat_messages v
		JOIN analytics_streams s ON s.stream_id = v.stream_id
		WHERE %s
		ORDER BY s.started_at ASC, v.offset_seconds ASC, v.id ASC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	entries := make([]UnifiedLogEntry, 0, limit)
	for rows.Next() {
		msg, startedAt, title, err := scanChannelVODRow(rows)
		if err != nil {
			return nil, "", err
		}
		entry := vodToUnified(msg)
		entry.StreamTitle = title
		entry.StreamStartedAt = &startedAt
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var next string
	if len(entries) > limit {
		last := entries[limit-1]
		entries = entries[:limit]
		if last.StreamStartedAt != nil {
			next = encodeChannelCursor(*last.StreamStartedAt, last.StreamID, last.OffsetSeconds, last.ID)
		}
	}
	return entries, next, nil
}

func scanChannelVODRow(rows pgxRowScanner) (VODChatMessage, time.Time, string, error) {
	var msg VODChatMessage
	var frags []byte
	var startedAt time.Time
	var title string
	if err := rows.Scan(
		&msg.ID, &msg.StreamID, &msg.MinuteTS, &msg.MessageID, &msg.DisplayName,
		&msg.CommenterLogin, &msg.SenderHash, &msg.Text, &frags, &msg.OffsetSeconds, &msg.SyncedAt,
		&startedAt, &title,
	); err != nil {
		return VODChatMessage{}, time.Time{}, "", err
	}
	if len(frags) > 0 {
		if err := json.Unmarshal(frags, &msg.EmoteFrags); err != nil {
			return VODChatMessage{}, time.Time{}, "", err
		}
	}
	return msg, startedAt, title, nil
}

type pgxRowScanner interface {
	Scan(dest ...any) error
}

func encodeChannelCursor(startedAt time.Time, streamID string, offsetSeconds int, id int64) string {
	raw := fmt.Sprintf("%d:%s:%d:%d", startedAt.UTC().Unix(), streamID, offsetSeconds, id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + ".channel"
}

func decodeChannelCursor(cursor string) (time.Time, string, int, int64, error) {
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 || parts[1] != "channel" {
		return time.Time{}, "", 0, 0, fmt.Errorf("malformed channel cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return time.Time{}, "", 0, 0, err
	}
	segments := strings.SplitN(string(raw), ":", 4)
	if len(segments) != 4 {
		return time.Time{}, "", 0, 0, fmt.Errorf("malformed channel cursor payload")
	}
	startedUnix, err := strconv.ParseInt(segments[0], 10, 64)
	if err != nil {
		return time.Time{}, "", 0, 0, err
	}
	offsetSeconds, err := strconv.Atoi(segments[2])
	if err != nil {
		return time.Time{}, "", 0, 0, err
	}
	id, err := strconv.ParseInt(segments[3], 10, 64)
	if err != nil {
		return time.Time{}, "", 0, 0, err
	}
	return time.Unix(startedUnix, 0).UTC(), segments[1], offsetSeconds, id, nil
}

func (s *Store) queryLiveMessages(ctx context.Context, params UnifiedQueryParams, limit int) ([]UnifiedLogEntry, string, error) {
	args := []any{strings.ToLower(params.Channel)}
	where := []string{"channel = $1"}
	if params.SenderHash != "" {
		return nil, "", fmt.Errorf("chatreplay: senderHash filter applies to VOD replay only")
	}
	if params.User != "" {
		args = append(args, params.User)
		where = append(where, fmt.Sprintf("(display_name ILIKE $%d OR login ILIKE $%d)", len(args), len(args)))
	}
	if params.Q != "" {
		args = append(args, "%"+params.Q+"%")
		where = append(where, fmt.Sprintf("text ILIKE $%d", len(args)))
	}
	if params.Cursor != "" {
		curTS, curID, curSource, err := decodeUnifiedCursor(params.Cursor)
		if err != nil || curSource != "live" {
			return nil, "", fmt.Errorf("chatreplay: invalid cursor: %w", err)
		}
		args = append(args, curTS, curID)
		where = append(where, fmt.Sprintf("(ts, id) > ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT id, channel, COALESCE(login,''), display_name, message_id, text, fragments, ts, synced_at
		FROM live_chat_messages
		WHERE %s
		ORDER BY ts ASC, id ASC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	entries := make([]UnifiedLogEntry, 0, limit)
	for rows.Next() {
		var msg LiveChatMessage
		var frags []byte
		if err := rows.Scan(&msg.ID, &msg.Channel, &msg.Login, &msg.DisplayName, &msg.MessageID, &msg.Text, &frags, &msg.TS, &msg.SyncedAt); err != nil {
			return nil, "", err
		}
		if len(frags) > 0 {
			_ = json.Unmarshal(frags, &msg.Fragments)
		}
		entries = append(entries, UnifiedLogEntry{
			Kind:        "message",
			ID:          msg.ID,
			TS:          msg.TS,
			DisplayName: msg.DisplayName,
			Login:       msg.Login,
			MessageID:   msg.MessageID,
			Text:        msg.Text,
			EmoteFrags:  msg.Fragments,
			Source:      "live",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var next string
	if len(entries) == limit {
		last := entries[len(entries)-1]
		next = encodeUnifiedCursor(last.TS, last.ID, "live")
	}
	return entries, next, nil
}

func (s *Store) queryModEvents(ctx context.Context, channel, cursor string, limit int) ([]UnifiedLogEntry, error) {
	args := []any{strings.ToLower(channel)}
	where := []string{"channel = $1"}
	if cursor != "" {
		curTS, curID, curSource, err := decodeUnifiedCursor(cursor)
		if err == nil && curSource == "mod" {
			args = append(args, curTS, curID)
			where = append(where, fmt.Sprintf("(ts, id) > ($%d, $%d)", len(args)-1, len(args)))
		}
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT id, channel, kind, COALESCE(actor_login,''), COALESCE(target_login,''), COALESCE(duration_sec,0),
		       COALESCE(reason,''), COALESCE(message_id,''), COALESCE(text_preview,''), ts, synced_at
		FROM chat_mod_events
		WHERE %s
		ORDER BY ts ASC, id ASC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UnifiedLogEntry, 0)
	for rows.Next() {
		var ev ChatModEvent
		if err := rows.Scan(&ev.ID, &ev.Channel, &ev.Kind, &ev.ActorLogin, &ev.TargetLogin, &ev.DurationSec, &ev.Reason, &ev.MessageID, &ev.TextPreview, &ev.TS, &ev.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, UnifiedLogEntry{
			Kind:    "mod_event",
			ID:      ev.ID,
			TS:      ev.TS,
			ModKind: ev.Kind,
			ModText: summarizeModEvent(ev),
			Source:  "mod",
		})
	}
	return out, rows.Err()
}

func summarizeModEvent(ev ChatModEvent) string {
	switch ev.Kind {
	case "clear_chat":
		return "Chat was cleared by a moderator"
	case "timeout":
		if ev.DurationSec > 0 {
			return fmt.Sprintf("%s was timed out for %ds", ev.TargetLogin, ev.DurationSec)
		}
		return fmt.Sprintf("%s was timed out", ev.TargetLogin)
	case "ban":
		return fmt.Sprintf("%s was banned", ev.TargetLogin)
	case "delete_message":
		if ev.TargetLogin != "" {
			return fmt.Sprintf("A message from %s was deleted", ev.TargetLogin)
		}
		return "A message was deleted"
	case "notice":
		if ev.TextPreview != "" {
			return ev.TextPreview
		}
		return ev.Kind
	default:
		return ev.Kind
	}
}

func mergeUnifiedTimeline(primary, secondary []UnifiedLogEntry, limit int) []UnifiedLogEntry {
	merged := append([]UnifiedLogEntry{}, primary...)
	merged = append(merged, secondary...)
	if len(merged) <= 1 {
		return merged
	}
	// Stable sort by timestamp then id.
	for i := 1; i < len(merged); i++ {
		j := i
		for j > 0 && (merged[j].TS.Before(merged[j-1].TS) || (merged[j].TS.Equal(merged[j-1].TS) && merged[j].ID < merged[j-1].ID)) {
			merged[j], merged[j-1] = merged[j-1], merged[j]
			j--
		}
	}
	if limit > 0 && len(merged) > limit {
		return merged[:limit]
	}
	return merged
}

func encodeUnifiedCursor(ts time.Time, id int64, source string) string {
	return encodeCursor(int(ts.Unix()), id) + "." + source
}

func decodeUnifiedCursor(cursor string) (time.Time, int64, string, error) {
	parts := strings.SplitN(cursor, ".", 2)
	source := "vod"
	if len(parts) == 2 {
		source = parts[1]
		cursor = parts[0]
	}
	offset, id, err := decodeCursor(cursor)
	if err != nil {
		return time.Time{}, 0, "", err
	}
	return time.Unix(int64(offset), 0).UTC(), id, source, nil
}

// PurgeLiveOlderThan deletes live chat rows older than cutoff.
func (s *Store) PurgeLiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if s.archiveProtectRetention {
		var missing int64
		err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM live_chat_messages m
			WHERE m.synced_at < $1
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $2
				  AND ae.natural_key = m.channel || ':' || m.message_id
				  AND ae.export_status = 'confirmed'
			  )`,
			cutoff.UTC(), archive.ArtifactLiveChatMessage,
		).Scan(&missing)
		if err != nil {
			return 0, err
		}
		if err := archive.BlockIfMissing(archive.ArtifactLiveChatMessage, missing); err != nil {
			return 0, err
		}
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM live_chat_messages WHERE synced_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PurgeModEventsOlderThan deletes mod events older than cutoff.
func (s *Store) PurgeModEventsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if s.archiveProtectRetention {
		var missing int64
		err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM chat_mod_events ev
			WHERE ev.synced_at < $1
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $2
				  AND ae.natural_key = ev.channel || ':' || ev.kind || ':' || COALESCE(ev.message_id, '') || ':' || EXTRACT(EPOCH FROM ev.ts)::bigint::text || ':' || COALESCE(ev.target_login, '')
				  AND ae.export_status = 'confirmed'
			  )`,
			cutoff.UTC(), archive.ArtifactChatModEvent,
		).Scan(&missing)
		if err != nil {
			return 0, err
		}
		if err := archive.BlockIfMissing(archive.ArtifactChatModEvent, missing); err != nil {
			return 0, err
		}
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM chat_mod_events WHERE synced_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CountLiveByChannel returns persisted live message count for stats.
func (s *Store) CountLiveByChannel(ctx context.Context, channel string) (int64, error) {
	if s == nil || s.db == nil || channel == "" {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM live_chat_messages WHERE channel = $1`, strings.ToLower(channel)).Scan(&count)
	return count, err
}
