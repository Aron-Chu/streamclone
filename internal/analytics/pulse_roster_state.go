package analytics

import (
	"context"
	"strings"
	"time"
)

type PulseRosterState struct {
	Login            string
	BroadcasterID    string
	Source           string
	Priority         int
	LastLiveStreamID string
	LastLiveSeenAt   *time.Time
	LastPolledAt     *time.Time
	NextPollAfter    *time.Time
	LastErrorCode    string
}

func (s *Store) RefreshProtectedGoLiveRoster(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		WITH protected AS (
			SELECT lower(login) AS login, 80 AS priority, 'global_protected' AS source
			FROM analytics_always_tracked
			UNION
			SELECT lower(login) AS login, 60 AS priority, 'principal_always_track' AS source
			FROM pulse_watchlist
			WHERE always_track = true
		)
		INSERT INTO pulse_roster_state (login, source, priority)
		SELECT login, source, priority FROM protected
		ON CONFLICT (login) DO UPDATE SET
			priority = GREATEST(pulse_roster_state.priority, EXCLUDED.priority),
			source = CASE
				WHEN EXCLUDED.priority > pulse_roster_state.priority THEN EXCLUDED.source
				ELSE pulse_roster_state.source
			END,
			updated_at = now()`)
	return err
}

func (s *Store) ListPulseRosterDue(ctx context.Context, limit int) ([]PulseRosterState, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT login, broadcaster_id, source, priority, last_live_stream_id,
			last_live_seen_at, last_polled_at, next_poll_after, last_error_code
		FROM pulse_roster_state
		WHERE next_poll_after IS NULL OR next_poll_after <= now()
		ORDER BY next_poll_after NULLS FIRST, login ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PulseRosterState
	for rows.Next() {
		var row PulseRosterState
		if err := rows.Scan(
			&row.Login,
			&row.BroadcasterID,
			&row.Source,
			&row.Priority,
			&row.LastLiveStreamID,
			&row.LastLiveSeenAt,
			&row.LastPolledAt,
			&row.NextPollAfter,
			&row.LastErrorCode,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePulseRosterPoll(ctx context.Context, login, broadcasterID, lastLiveStreamID, lastErrorCode string, lastLiveSeenAt, nextPollAfter time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	now := time.Now().UTC()
	var seenAt any
	if !lastLiveSeenAt.IsZero() {
		seenAt = lastLiveSeenAt.UTC()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE pulse_roster_state
		SET broadcaster_id = CASE WHEN $2 <> '' THEN $2 ELSE broadcaster_id END,
			last_live_stream_id = $3,
			last_live_seen_at = COALESCE($4::timestamptz, last_live_seen_at),
			last_polled_at = $5,
			next_poll_after = $6,
			last_error_code = $7,
			updated_at = now()
		WHERE login = $1`,
		normalizeLogin(login),
		strings.TrimSpace(broadcasterID),
		strings.TrimSpace(lastLiveStreamID),
		seenAt,
		now,
		nextPollAfter.UTC(),
		strings.TrimSpace(lastErrorCode),
	)
	return err
}
