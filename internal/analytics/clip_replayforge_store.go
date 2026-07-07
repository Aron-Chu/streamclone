package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) GetClipCandidate(ctx context.Context, candidateID, principalID string) (ClipCandidate, error) {
	if s == nil || s.db == nil {
		return ClipCandidate{}, errors.New("store unavailable")
	}
	row := s.db.QueryRow(ctx, `
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
		LEFT JOIN clip_candidate_states s ON s.candidate_id = c.id AND s.principal_id = $2
		LEFT JOIN clip_candidate_jobs j ON j.candidate_id = c.id AND j.principal_id = $2
		WHERE c.id = $1
	`, candidateID, principalID)
	return scanClipCandidate(row)
}

func (s *Store) GetClipCandidateJob(ctx context.Context, candidateID, principalID string) (ClipCandidateJob, error) {
	if s == nil || s.db == nil {
		return ClipCandidateJob{}, errors.New("store unavailable")
	}
	row := s.db.QueryRow(ctx, `
		SELECT id, candidate_id, principal_id, principal_kind, status, replayforge_job_id,
			replayforge_state, request_json, response_json, COALESCE(error_code, ''),
			COALESCE(error_message, ''), submitted_at, last_checked_at, created_at, updated_at
		FROM clip_candidate_jobs
		WHERE candidate_id=$1 AND principal_id=$2
	`, candidateID, principalID)
	return scanClipCandidateJob(row)
}

func (s *Store) UpsertClipCandidateJob(ctx context.Context, job ClipCandidateJob) (ClipCandidateJob, error) {
	if s == nil || s.db == nil {
		return ClipCandidateJob{}, errors.New("store unavailable")
	}
	job.ErrorCode = sanitizeReplayForgeStatusText(job.ErrorCode)
	job.ErrorMessage = sanitizeReplayForgeStatusText(job.ErrorMessage)
	requestJSON, err := json.Marshal(job.Request)
	if err != nil {
		return ClipCandidateJob{}, err
	}
	response := job.Response
	if response == nil {
		response = map[string]interface{}{}
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return ClipCandidateJob{}, err
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO clip_candidate_jobs (
			id, candidate_id, principal_id, principal_kind, status, replayforge_job_id,
			replayforge_state, request_json, response_json, error_code, error_message,
			submitted_at, last_checked_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (candidate_id, principal_id) DO UPDATE SET
			principal_kind=EXCLUDED.principal_kind,
			status=EXCLUDED.status,
			replayforge_job_id=EXCLUDED.replayforge_job_id,
			replayforge_state=EXCLUDED.replayforge_state,
			request_json=EXCLUDED.request_json,
			response_json=EXCLUDED.response_json,
			error_code=EXCLUDED.error_code,
			error_message=EXCLUDED.error_message,
			submitted_at=EXCLUDED.submitted_at,
			last_checked_at=EXCLUDED.last_checked_at,
			updated_at=now()
		RETURNING id, candidate_id, principal_id, principal_kind, status, replayforge_job_id,
			replayforge_state, request_json, response_json, COALESCE(error_code, ''),
			COALESCE(error_message, ''), submitted_at, last_checked_at, created_at, updated_at
	`, job.ID, job.CandidateID, job.PrincipalID, job.PrincipalKind, job.Status, clipJobNullableString(job.ReplayForgeJobID),
		clipJobNullableString(job.ReplayForgeState), requestJSON, responseJSON, clipJobNullableString(job.ErrorCode), clipJobNullableString(job.ErrorMessage),
		job.SubmittedAt, job.LastCheckedAt)
	return scanClipCandidateJob(row)
}

func (s *Store) UpdateClipCandidateJobsByReplayForgeID(ctx context.Context, replayForgeJobID string, status ReplayForgeJobStatusResponse) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("store unavailable")
	}
	replayForgeJobID = strings.TrimSpace(replayForgeJobID)
	if replayForgeJobID == "" {
		return 0, errors.New("replayforge job id required")
	}
	responseJSON, err := json.Marshal(status.ResponseMap())
	if err != nil {
		return 0, err
	}
	incomingStatus := clipCandidateJobStatusFromReplayForgeStatus(status)
	errorCode, errorMessage := replayForgeStatusFailure(status)
	var updated int
	if err := s.db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE clip_candidate_jobs
			SET status = CASE
					WHEN status IN ('ready', 'failed') AND $2 = 'queued' THEN status
					ELSE $2
				END,
				replayforge_state = CASE
					WHEN status IN ('ready', 'failed') AND $2 = 'queued' THEN replayforge_state
					ELSE $3
				END,
				response_json = CASE
					WHEN status IN ('ready', 'failed') AND $2 = 'queued' THEN response_json
					ELSE $4
				END,
				error_code = CASE
					WHEN status IN ('ready', 'failed') AND $2 = 'queued' THEN error_code
					WHEN $2 = 'failed' THEN $5
					ELSE NULL
				END,
				error_message = CASE
					WHEN status IN ('ready', 'failed') AND $2 = 'queued' THEN error_message
					WHEN $2 = 'failed' THEN $6
					ELSE NULL
				END,
				last_checked_at=now(),
				updated_at=now()
			WHERE replayforge_job_id=$1
			RETURNING id
		)
		SELECT count(*) FROM updated
	`, replayForgeJobID, incomingStatus, clipJobNullableString(status.State()), responseJSON, clipJobNullableString(errorCode), clipJobNullableString(errorMessage)).Scan(&updated); err != nil {
		return 0, err
	}
	if updated == 0 {
		return 0, pgx.ErrNoRows
	}
	return updated, nil
}

func scanClipCandidateJob(row clipCandidateScanner) (ClipCandidateJob, error) {
	var item ClipCandidateJob
	var replayForgeJobID, replayForgeState pgtype.Text
	var requestJSON, responseJSON []byte
	var submittedAt, lastCheckedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.ID, &item.CandidateID, &item.PrincipalID, &item.PrincipalKind, &item.Status,
		&replayForgeJobID, &replayForgeState, &requestJSON, &responseJSON, &item.ErrorCode,
		&item.ErrorMessage, &submittedAt, &lastCheckedAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return ClipCandidateJob{}, err
	}
	item.ReplayForgeJobID = replayForgeJobID.String
	item.ReplayForgeState = replayForgeState.String
	_ = json.Unmarshal(requestJSON, &item.Request)
	_ = json.Unmarshal(responseJSON, &item.Response)
	item.SubmittedAt = timePtr(submittedAt)
	item.LastCheckedAt = timePtr(lastCheckedAt)
	return item, nil
}

func clipJobNullableString(value string) *string {
	if value == "" {
		return nil
	}
	v := value
	return &v
}

func timeNowPtr() *time.Time {
	now := time.Now().UTC()
	return &now
}
