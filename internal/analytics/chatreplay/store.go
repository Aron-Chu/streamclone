package chatreplay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pagination defaults for the chat-replay query (Requirement 27.5).
const (
	DefaultPageLimit = 200
	MaxPageLimit     = 500
)

// Store provides CRUD and paginated access to persisted VOD chat messages in
// the analytics_vod_chat_messages table. It follows the pgxpool.Pool pattern
// used by the analytics package Store.
type Store struct {
	db *pgxpool.Pool
}

// NewStore constructs a chat-replay Store backed by the given connection pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Ping verifies database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

// Insert upserts a single message. Conflicts on (stream_id, message_id) are
// ignored (ON CONFLICT DO NOTHING) so repeated/resumed syncs are idempotent.
func (s *Store) Insert(ctx context.Context, msg VODChatMessage) error {
	if s == nil || s.db == nil {
		return nil
	}
	if msg.StreamID == "" || msg.MessageID == "" {
		return nil
	}
	frags, err := marshalFrags(msg.EmoteFrags)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, insertSQL,
		msg.StreamID, msg.MinuteTS, msg.MessageID, msg.DisplayName,
		msg.CommenterLogin, msg.SenderHash, msg.Text, frags, msg.OffsetSeconds,
	)
	return err
}

// BulkInsert upserts a batch of messages in a single round-trip. Each row uses
// ON CONFLICT (stream_id, message_id) DO NOTHING for idempotency. Rows missing
// a stream id or message id are skipped.
func (s *Store) BulkInsert(ctx context.Context, msgs []VODChatMessage) error {
	if s == nil || s.db == nil || len(msgs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	queued := 0
	for _, msg := range msgs {
		if msg.StreamID == "" || msg.MessageID == "" {
			continue
		}
		frags, err := marshalFrags(msg.EmoteFrags)
		if err != nil {
			return err
		}
		batch.Queue(insertSQL,
			msg.StreamID, msg.MinuteTS, msg.MessageID, msg.DisplayName,
			msg.CommenterLogin, msg.SenderHash, msg.Text, frags, msg.OffsetSeconds,
		)
		queued++
	}
	if queued == 0 {
		return nil
	}
	br := s.db.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

const insertSQL = `
	INSERT INTO analytics_vod_chat_messages (
		stream_id, minute_ts, message_id, display_name, commenter_login, sender_hash, text, emote_frags, offset_seconds
	)
	VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8::jsonb,$9)
	ON CONFLICT (stream_id, message_id) DO NOTHING`

// QueryParams describes a paginated chat-replay query. Messages are filtered to
// [OffsetStart, OffsetEnd] inclusive (when set) and returned ordered by
// offset_seconds ascending, then id ascending for a stable cursor.
type QueryParams struct {
	StreamID    string
	OffsetStart int
	OffsetEnd   int
	Limit       int
	Cursor      string
	Q           string
	User        string
	SenderHash  string
}

// QueryResult is a single page of chat-replay messages plus an opaque cursor
// for fetching the next page. NextCursor is empty when no further pages exist.
type QueryResult struct {
	Messages   []VODChatMessage
	NextCursor string
}

// Query returns a page of messages for a stream ordered by offset ascending.
// It honors a default page limit of 200, capped at 500 (Requirement 27.5), and
// uses a keyset cursor over (offset_seconds, id) for stable pagination.
func (s *Store) Query(ctx context.Context, params QueryParams) (QueryResult, error) {
	if s == nil || s.db == nil || params.StreamID == "" {
		return QueryResult{}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = DefaultPageLimit
	}
	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}

	args := []any{params.StreamID}
	where := []string{"stream_id = $1"}

	if params.OffsetStart > 0 {
		args = append(args, params.OffsetStart)
		where = append(where, fmt.Sprintf("offset_seconds >= $%d", len(args)))
	}
	if params.OffsetEnd > 0 {
		args = append(args, params.OffsetEnd)
		where = append(where, fmt.Sprintf("offset_seconds <= $%d", len(args)))
	}

	if params.SenderHash != "" {
		args = append(args, params.SenderHash)
		where = append(where, fmt.Sprintf("sender_hash = $%d", len(args)))
	}
	if params.User != "" {
		args = append(args, params.User)
		where = append(where, fmt.Sprintf("(display_name ILIKE $%d OR commenter_login ILIKE $%d)", len(args), len(args)))
	}
	if params.Q != "" {
		args = append(args, "%"+params.Q+"%")
		where = append(where, fmt.Sprintf("text ILIKE $%d", len(args)))
	}

	if params.Cursor != "" {
		curOffset, curID, err := decodeCursor(params.Cursor)
		if err != nil {
			return QueryResult{}, fmt.Errorf("chatreplay: invalid cursor: %w", err)
		}
		args = append(args, curOffset, curID)
		where = append(where, fmt.Sprintf("(offset_seconds, id) > ($%d, $%d)", len(args)-1, len(args)))
	}

	// Fetch one extra row to determine whether another page exists.
	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT id, stream_id, minute_ts, message_id, display_name, COALESCE(commenter_login,''), sender_hash, text, emote_frags, offset_seconds, synced_at
		FROM analytics_vod_chat_messages
		WHERE %s
		ORDER BY offset_seconds ASC, id ASC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return QueryResult{}, err
	}
	defer rows.Close()

	messages := make([]VODChatMessage, 0, limit)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return QueryResult{}, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, err
	}

	result := QueryResult{}
	if len(messages) > limit {
		last := messages[limit-1]
		result.Messages = messages[:limit]
		result.NextCursor = encodeCursor(last.OffsetSeconds, last.ID)
	} else {
		result.Messages = messages
	}
	return result, nil
}

// CountByStream returns the number of stored messages for a stream. Used by the
// replay endpoint to set the "unavailable" flag (Requirement 27.6).
func (s *Store) CountByStream(ctx context.Context, streamID string) (int64, error) {
	if s == nil || s.db == nil || streamID == "" {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_vod_chat_messages WHERE stream_id = $1`,
		streamID,
	).Scan(&count)
	return count, err
}

// DeleteByStream removes all stored messages for a stream and returns the count
// of deleted rows.
func (s *Store) DeleteByStream(ctx context.Context, streamID string) (int64, error) {
	if s == nil || s.db == nil || streamID == "" {
		return 0, nil
	}
	tag, err := s.db.Exec(ctx,
		`DELETE FROM analytics_vod_chat_messages WHERE stream_id = $1`,
		streamID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PurgeOlderThan deletes all stored messages whose synced_at is strictly older
// than cutoff and returns the number of rows removed. It uses synced_at (the
// ingestion timestamp) to match the analytics retention semantics rather than
// the in-VOD minute timestamp. Safe on a nil receiver/pool.
func (s *Store) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	tag, err := s.db.Exec(ctx,
		`DELETE FROM analytics_vod_chat_messages WHERE synced_at < $1`,
		cutoff.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanMessage(rows pgx.Row) (VODChatMessage, error) {
	var msg VODChatMessage
	var frags []byte
	if err := rows.Scan(
		&msg.ID, &msg.StreamID, &msg.MinuteTS, &msg.MessageID, &msg.DisplayName,
		&msg.CommenterLogin, &msg.SenderHash, &msg.Text, &frags, &msg.OffsetSeconds, &msg.SyncedAt,
	); err != nil {
		return VODChatMessage{}, err
	}
	if len(frags) > 0 {
		if err := json.Unmarshal(frags, &msg.EmoteFrags); err != nil {
			return VODChatMessage{}, err
		}
	}
	return msg, nil
}

func marshalFrags(frags []EmoteFrag) (string, error) {
	if len(frags) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(frags)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// encodeCursor produces an opaque keyset cursor over (offsetSeconds, id).
func encodeCursor(offsetSeconds int, id int64) string {
	raw := fmt.Sprintf("%d:%d", offsetSeconds, id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (int, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0, err
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("malformed cursor")
	}
	offsetSeconds, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return offsetSeconds, id, nil
}
