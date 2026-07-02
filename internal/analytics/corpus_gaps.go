package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	CorpusGapKindFailed       = "failed"
	CorpusGapKindDeadLetter   = "dead_letter"
	CorpusGapKindStaleRunning = "stale_running"
	CorpusGapKindQueued       = "queued"
	CorpusGapKindKnownEmpty   = "known_empty"
)

// CorpusGoldGap is an operator-facing Gold segment gap (internal API only).
type CorpusGoldGap struct {
	SegmentKey         string `json:"segmentKey"`
	VODID              string `json:"vodId"`
	StreamID           string `json:"streamId,omitempty"`
	Login              string `json:"login,omitempty"`
	Status             string `json:"status"`
	GapKind            string `json:"gapKind"`
	CommentsFetched    int    `json:"commentsFetched,omitempty"`
	Attempt            int    `json:"attempt,omitempty"`
	MaxAttempts        int    `json:"maxAttempts,omitempty"`
	LeaseOwner         string `json:"leaseOwner,omitempty"`
	LeaseExpiresAt     string `json:"leaseExpiresAt,omitempty"`
	NextRunAt          string `json:"nextRunAt,omitempty"`
	Error              string `json:"error,omitempty"`
	BackfillJobID      *int64 `json:"backfillJobId,omitempty"`
	StartOffsetSeconds int    `json:"startOffsetSeconds"`
	EndOffsetSeconds   int    `json:"endOffsetSeconds"`
}

// CorpusGoldWorkerLease summarizes active durable segment leases by owner.
type CorpusGoldWorkerLease struct {
	LeaseOwner string `json:"leaseOwner"`
	Running    int    `json:"running"`
}

type corpusGoldGapListOpts struct {
	Limit  int
	VODID  string
	Status string
}

func (s *Store) ListCorpusGoldGaps(ctx context.Context, limit int, vodID, status string) ([]CorpusGoldGap, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store unavailable")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{limit}
	where := []string{
		`(
			status IN ('failed', 'dead_letter')
			OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < now())
			OR status = 'queued'
		)`,
	}
	if v := strings.TrimSpace(vodID); v != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("vod_id = $%d", len(args)))
	}
	if st := strings.TrimSpace(status); st != "" {
		args = append(args, st)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT segment_key, vod_id, COALESCE(stream_id, ''), COALESCE(login, ''),
		       status, COALESCE(comments_fetched, 0), COALESCE(attempt, 0), COALESCE(max_attempts, 0),
		       COALESCE(lease_owner, ''), lease_expires_at, next_run_at, COALESCE(error, ''),
		       backfill_job_id, start_offset_seconds, end_offset_seconds
		FROM gold_vod_segments
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT $1`, strings.Join(where, " AND "))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CorpusGoldGap, 0, limit)
	for rows.Next() {
		var g CorpusGoldGap
		var leaseExp, nextRun *time.Time
		if err := rows.Scan(
			&g.SegmentKey, &g.VODID, &g.StreamID, &g.Login, &g.Status,
			&g.CommentsFetched, &g.Attempt, &g.MaxAttempts, &g.LeaseOwner,
			&leaseExp, &nextRun, &g.Error, &g.BackfillJobID,
			&g.StartOffsetSeconds, &g.EndOffsetSeconds,
		); err != nil {
			return nil, err
		}
		g.GapKind = classifyCorpusGoldGap(g.Status, g.CommentsFetched, g.Error, leaseExp)
		if leaseExp != nil {
			g.LeaseExpiresAt = leaseExp.UTC().Format(time.RFC3339)
		}
		if nextRun != nil {
			g.NextRunAt = nextRun.UTC().Format(time.RFC3339)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func classifyCorpusGoldGap(status string, commentsFetched int, errMsg string, leaseExp *time.Time) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed":
		return CorpusGapKindFailed
	case "dead_letter":
		return CorpusGapKindDeadLetter
	case "running":
		if leaseExp != nil && leaseExp.Before(time.Now().UTC()) {
			return CorpusGapKindStaleRunning
		}
		return CorpusGapKindQueued
	case "done":
		if commentsFetched == 0 && strings.TrimSpace(errMsg) == "" {
			return CorpusGapKindKnownEmpty
		}
		return ""
	case "queued":
		return CorpusGapKindQueued
	default:
		return strings.TrimSpace(status)
	}
}

// RequeueCorpusGoldSegments resets failed/dead_letter segments to queued for operator retry.
func (s *Store) RequeueCorpusGoldSegments(ctx context.Context, segmentKeys []string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store unavailable")
	}
	keys := make([]string, 0, len(segmentKeys))
	for _, k := range segmentKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET status = 'queued',
		    lease_owner = '',
		    lease_expires_at = NULL,
		    heartbeat_at = NULL,
		    next_run_at = now(),
		    error = '',
		    attempt = 0,
		    updated_at = now()
		WHERE segment_key = ANY($1)
		  AND status IN ('failed', 'dead_letter')`, keys)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) ListCorpusGoldWorkerLeases(ctx context.Context) ([]CorpusGoldWorkerLease, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store unavailable")
	}
	rows, err := s.db.Query(ctx, `
		SELECT COALESCE(NULLIF(lease_owner, ''), '(unowned)'), COUNT(*)::int
		FROM gold_vod_segments
		WHERE status = 'running'
		  AND (lease_expires_at IS NULL OR lease_expires_at > now())
		GROUP BY 1
		ORDER BY 2 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CorpusGoldWorkerLease, 0, 8)
	for rows.Next() {
		var row CorpusGoldWorkerLease
		if err := rows.Scan(&row.LeaseOwner, &row.Running); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SyncTop500GoldStatusFromSegments derives inventory gold_status from durable segment rows.
func (s *Store) SyncTop500GoldStatusFromSegments(ctx context.Context, vodID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("store unavailable")
	}
	vodID = strings.TrimSpace(vodID)
	if vodID == "" {
		return false, fmt.Errorf("vod_id required")
	}
	var open, knownEmpty, done, skipped int
	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('queued','running','failed','dead_letter')
				OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < now())),
			COUNT(*) FILTER (WHERE status = 'done' AND COALESCE(comments_fetched, 0) = 0 AND COALESCE(error, '') = ''),
			COUNT(*) FILTER (WHERE status = 'done'),
			COUNT(*) FILTER (WHERE status = 'skipped')
		FROM gold_vod_segments
		WHERE vod_id = $1`, vodID).Scan(&open, &knownEmpty, &done, &skipped)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	total := open + done + skipped
	if total == 0 {
		return false, nil
	}
	status := GoldVODStatusQueued
	availability := GoldVODAvailabilityEligible
	switch {
	case open > 0:
		status = GoldVODStatusFailed
		availability = GoldVODAvailabilityEligible
	case done > 0 && knownEmpty == done:
		status = GoldVODStatusDone
		availability = GoldVODAvailabilityLoaded
	case done+skipped == total:
		status = GoldVODStatusDone
		availability = GoldVODAvailabilityLoaded
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE top500_vod_inventory
		SET gold_status = $2,
		    availability_state = $3,
		    updated_at = now()
		WHERE vod_id = $1`, vodID, status, availability)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
