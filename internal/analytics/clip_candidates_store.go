package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) UpsertClipCandidates(ctx context.Context, candidates []ClipCandidate) error {
	if s == nil || s.db == nil || len(candidates) == 0 {
		return nil
	}
	for _, candidate := range candidates {
		topEmotes, err := json.Marshal(candidate.TopEmotes)
		if err != nil {
			return err
		}
		signals, err := json.Marshal(candidate.Signals)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(ctx, `
			INSERT INTO clip_candidates (
				id, login, stream_id, vod_id, minute_ts, offset_seconds, start_seconds, end_seconds,
				score, confidence, reason, source_kind, coverage_state, signals_json, top_emotes_json,
				source_status, source_checked_at
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			ON CONFLICT (stream_id, offset_seconds, reason) DO UPDATE SET
				vod_id=EXCLUDED.vod_id,
				minute_ts=EXCLUDED.minute_ts,
				start_seconds=EXCLUDED.start_seconds,
				end_seconds=EXCLUDED.end_seconds,
				score=EXCLUDED.score,
				confidence=EXCLUDED.confidence,
				source_kind=EXCLUDED.source_kind,
				coverage_state=EXCLUDED.coverage_state,
				signals_json=EXCLUDED.signals_json,
				top_emotes_json=EXCLUDED.top_emotes_json,
				source_status=EXCLUDED.source_status,
				source_checked_at=EXCLUDED.source_checked_at,
				updated_at=now()
		`, candidate.ID, candidate.Login, candidate.StreamID, candidate.VodID, candidate.MinuteTS, candidate.OffsetSeconds,
			candidate.StartSeconds, candidate.EndSeconds, candidate.Score, candidate.Confidence, candidate.Reason, candidate.SourceKind,
			candidate.CoverageState, signals, topEmotes, candidate.SourceStatus, candidate.SourceCheckedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecentStreamsForClipCandidateSeeding(ctx context.Context, limit int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store unavailable")
	}
	if limit <= 0 || limit > defaultClipSeedStreamLimit {
		limit = defaultClipSeedStreamLimit
	}
	rows, err := s.db.Query(ctx, `
		SELECT st.stream_id
		FROM analytics_streams st
		WHERE COALESCE(NULLIF(st.canonical_stream_id, ''), st.stream_id) = st.stream_id
		  AND EXISTS (
			SELECT 1
			FROM analytics_minute_rollups r
			WHERE r.stream_id = st.stream_id
			  AND (r.chat_count > 0 OR r.total_emote_count > 0 OR r.seventv_emote_count > 0 OR r.viewer_samples > 0)
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM clip_candidates c
			WHERE c.stream_id = st.stream_id
		  )
		ORDER BY st.ended_at DESC NULLS LAST, st.last_seen_at DESC, st.started_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var streamID string
		if err := rows.Scan(&streamID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(streamID) != "" {
			out = append(out, streamID)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListClipCandidates(ctx context.Context, filter ListClipCandidatesFilter) ([]ClipCandidate, string, error) {
	if s == nil || s.db == nil {
		return nil, "", errors.New("store unavailable")
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	args := []any{}
	clauses := []string{"1=1"}
	if filter.Login != "" {
		args = append(args, filter.Login)
		clauses = append(clauses, "c.login=$"+strconv.Itoa(len(args)))
	}
	if filter.StreamID != "" {
		args = append(args, filter.StreamID)
		clauses = append(clauses, "c.stream_id=$"+strconv.Itoa(len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, "COALESCE(s.status, 'new')=$"+strconv.Itoa(len(args)))
	}
	if filter.MinChatCount > 0 {
		args = append(args, filter.MinChatCount)
		clauses = append(clauses, "COALESCE((c.signals_json->>'chatCount')::int, 0)>=$"+strconv.Itoa(len(args)))
	}
	if filter.MaxChatCount > 0 {
		args = append(args, filter.MaxChatCount)
		clauses = append(clauses, "COALESCE((c.signals_json->>'chatCount')::int, 0)<=$"+strconv.Itoa(len(args)))
	}
	if filter.Cursor != nil {
		args = append(args, filter.Cursor.Score)
		scoreIdx := strconv.Itoa(len(args))
		args = append(args, filter.Cursor.CreatedAt)
		createdAtIdx := strconv.Itoa(len(args))
		args = append(args, filter.Cursor.ID)
		idIdx := strconv.Itoa(len(args))
		clauses = append(clauses, "(c.score<$"+scoreIdx+" OR (c.score=$"+scoreIdx+" AND c.created_at<$"+createdAtIdx+") OR (c.score=$"+scoreIdx+" AND c.created_at=$"+createdAtIdx+" AND c.id<$"+idIdx+"))")
	}
	if !filter.Before.IsZero() {
		args = append(args, filter.Before)
		clauses = append(clauses, "c.created_at<$"+strconv.Itoa(len(args)))
	}
	principalID := filter.PrincipalID
	args = append(args, principalID)
	principalIdx := strconv.Itoa(len(args))
	args = append(args, filter.Limit+1)
	limitIdx := strconv.Itoa(len(args))
	query := `
		SELECT
			c.id, c.login, c.stream_id, c.vod_id, st.title, st.category, st.started_at,
			c.minute_ts, c.offset_seconds, c.start_seconds, c.end_seconds, c.score, c.confidence,
			c.reason, c.source_kind, c.coverage_state, c.signals_json, c.top_emotes_json,
			c.source_status, c.source_checked_at, c.created_at, c.updated_at,
			s.id, s.candidate_id, s.principal_id, s.principal_kind, COALESCE(s.status, 'new'),
			s.title_override, s.start_seconds_override, s.end_seconds_override, COALESCE(s.notes, ''),
			s.created_at, s.updated_at,
			j.id, j.candidate_id, j.principal_id, j.principal_kind, j.status, j.replayforge_job_id,
			j.replayforge_state, j.request_json, j.response_json, COALESCE(j.error_code, ''),
			COALESCE(j.error_message, ''), j.submitted_at, j.last_checked_at, j.created_at, j.updated_at
		FROM clip_candidates c
		LEFT JOIN analytics_streams st ON st.stream_id = c.stream_id
		LEFT JOIN clip_candidate_states s ON s.candidate_id = c.id AND s.principal_id = $` + principalIdx + `
		LEFT JOIN clip_candidate_jobs j ON j.candidate_id = c.id AND j.principal_id = $` + principalIdx + `
		WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY c.score DESC, c.created_at DESC, c.id DESC
		LIMIT $` + limitIdx
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]ClipCandidate, 0, filter.Limit)
	for rows.Next() {
		item, err := scanClipCandidate(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(items) > filter.Limit {
		nextCursor = encodeClipCandidateCursor(items[filter.Limit-1])
		items = items[:filter.Limit]
	}
	return items, nextCursor, nil
}

func (s *Store) UpdateClipCandidateState(ctx context.Context, candidateID string, principal PulsePrincipal, patch clipCandidateStatePatch) (ClipCandidateState, error) {
	if s == nil || s.db == nil {
		return ClipCandidateState{}, errors.New("store unavailable")
	}
	if strings.TrimSpace(principal.ID) == "" {
		return ClipCandidateState{}, errors.New("principal required")
	}
	var exists string
	if err := s.db.QueryRow(ctx, `SELECT id FROM clip_candidates WHERE id=$1`, candidateID).Scan(&exists); err != nil {
		return ClipCandidateState{}, err
	}
	stateID := newClipCandidateStateID(candidateID, principal.ID)
	row := s.db.QueryRow(ctx, `
		INSERT INTO clip_candidate_states (
			id, candidate_id, principal_id, principal_kind, status, title_override,
			start_seconds_override, end_seconds_override, notes
		)
		VALUES ($1,$2,$3,$4,COALESCE($5, 'new'),$6,$7,$8,COALESCE($9, ''))
		ON CONFLICT (candidate_id, principal_id) DO UPDATE SET
			principal_kind=EXCLUDED.principal_kind,
			status=COALESCE($5, clip_candidate_states.status),
			title_override=COALESCE($6, clip_candidate_states.title_override),
			start_seconds_override=COALESCE($7, clip_candidate_states.start_seconds_override),
			end_seconds_override=COALESCE($8, clip_candidate_states.end_seconds_override),
			notes=COALESCE($9, clip_candidate_states.notes),
			updated_at=now()
		RETURNING id, candidate_id, principal_id, principal_kind, status, title_override,
			start_seconds_override, end_seconds_override, notes, created_at, updated_at
	`, stateID, candidateID, principal.ID, principal.Kind, patch.Status, patch.TitleOverride, patch.StartSecondsOverride, patch.EndSecondsOverride, patch.Notes)
	return scanClipCandidateState(row)
}

type clipCandidateScanner interface {
	Scan(dest ...any) error
}

func scanClipCandidate(row clipCandidateScanner) (ClipCandidate, error) {
	var item ClipCandidate
	var vodID, title, category, coverageState pgtype.Text
	var startedAt, minuteTS, sourceCheckedAt pgtype.Timestamptz
	var signalsBytes, topEmotesBytes []byte
	var stateID, stateCandidateID, statePrincipalID, statePrincipalKind pgtype.Text
	var stateTitle pgtype.Text
	var stateStart, stateEnd pgtype.Int4
	var stateCreated, stateUpdated pgtype.Timestamptz
	var state ClipCandidateState
	var jobID, jobCandidateID, jobPrincipalID, jobPrincipalKind, jobStatus, jobReplayForgeID, jobReplayForgeState pgtype.Text
	var jobErrorCode, jobErrorMessage pgtype.Text
	var jobRequestBytes, jobResponseBytes []byte
	var jobSubmittedAt, jobLastCheckedAt, jobCreatedAt, jobUpdatedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.ID, &item.Login, &item.StreamID, &vodID, &title, &category, &startedAt,
		&minuteTS, &item.OffsetSeconds, &item.StartSeconds, &item.EndSeconds, &item.Score, &item.Confidence,
		&item.Reason, &item.SourceKind, &coverageState, &signalsBytes, &topEmotesBytes,
		&item.SourceStatus, &sourceCheckedAt, &item.CreatedAt, &item.UpdatedAt,
		&stateID, &stateCandidateID, &statePrincipalID, &statePrincipalKind, &state.Status,
		&stateTitle, &stateStart, &stateEnd, &state.Notes, &stateCreated, &stateUpdated,
		&jobID, &jobCandidateID, &jobPrincipalID, &jobPrincipalKind, &jobStatus, &jobReplayForgeID,
		&jobReplayForgeState, &jobRequestBytes, &jobResponseBytes, &jobErrorCode, &jobErrorMessage,
		&jobSubmittedAt, &jobLastCheckedAt, &jobCreatedAt, &jobUpdatedAt,
	); err != nil {
		return ClipCandidate{}, err
	}
	item.VodID = textPtr(vodID)
	item.StreamTitle = title.String
	item.StreamCategory = category.String
	item.StartedAt = timePtr(startedAt)
	item.MinuteTS = timePtr(minuteTS)
	item.CoverageState = coverageState.String
	item.SourceCheckedAt = timePtr(sourceCheckedAt)
	_ = json.Unmarshal(signalsBytes, &item.Signals)
	_ = json.Unmarshal(topEmotesBytes, &item.TopEmotes)
	hydrateClipCandidateSignals(&item)
	if stateID.Valid {
		state.ID = stateID.String
		state.CandidateID = stateCandidateID.String
		state.PrincipalID = statePrincipalID.String
		state.PrincipalKind = statePrincipalKind.String
		state.TitleOverride = textPtr(stateTitle)
		if stateStart.Valid {
			v := int(stateStart.Int32)
			state.StartSecondsOverride = &v
		}
		if stateEnd.Valid {
			v := int(stateEnd.Int32)
			state.EndSecondsOverride = &v
		}
		if stateCreated.Valid {
			state.CreatedAt = stateCreated.Time
		}
		if stateUpdated.Valid {
			state.UpdatedAt = stateUpdated.Time
		}
		item.State = &state
	}
	if jobID.Valid {
		job := ClipCandidateJob{
			ID:               jobID.String,
			CandidateID:      jobCandidateID.String,
			PrincipalID:      jobPrincipalID.String,
			PrincipalKind:    jobPrincipalKind.String,
			Status:           jobStatus.String,
			ReplayForgeJobID: jobReplayForgeID.String,
			ReplayForgeState: jobReplayForgeState.String,
			ErrorCode:        jobErrorCode.String,
			ErrorMessage:     jobErrorMessage.String,
			SubmittedAt:      timePtr(jobSubmittedAt),
			LastCheckedAt:    timePtr(jobLastCheckedAt),
		}
		if jobCreatedAt.Valid {
			job.CreatedAt = jobCreatedAt.Time
		}
		if jobUpdatedAt.Valid {
			job.UpdatedAt = jobUpdatedAt.Time
		}
		_ = json.Unmarshal(jobRequestBytes, &job.Request)
		_ = json.Unmarshal(jobResponseBytes, &job.Response)
		item.Job = &job
	}
	enrichClipCandidateInbox(&item)
	return item, nil
}

func hydrateClipCandidateSignals(item *ClipCandidate) {
	if item == nil || len(item.Signals) == 0 {
		return
	}
	if item.ChatCount == 0 {
		item.ChatCount = clipMaxInt(0, clipSignalInt(item.Signals["chatCount"]))
	}
	if item.EmoteCount == 0 {
		item.EmoteCount = clipMaxInt(0, clipSignalInt(item.Signals["emoteCount"]))
	}
	if item.ViewerCount == 0 {
		item.ViewerCount = clipMaxInt(0, clipSignalInt(item.Signals["viewerCount"]))
	}
}

func clipSignalInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		parsed, _ := v.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func scanClipCandidateState(row clipCandidateScanner) (ClipCandidateState, error) {
	var item ClipCandidateState
	var title pgtype.Text
	var start, end pgtype.Int4
	if err := row.Scan(&item.ID, &item.CandidateID, &item.PrincipalID, &item.PrincipalKind, &item.Status, &title, &start, &end, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ClipCandidateState{}, err
	}
	item.TitleOverride = textPtr(title)
	if start.Valid {
		v := int(start.Int32)
		item.StartSecondsOverride = &v
	}
	if end.Valid {
		v := int(end.Int32)
		item.EndSecondsOverride = &v
	}
	return item, nil
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}
