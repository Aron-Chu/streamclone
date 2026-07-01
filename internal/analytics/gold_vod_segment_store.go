package analytics

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type GoldVODSegmentClaim struct {
	ID                 int64
	SegmentKey         string
	VODID              string
	StreamID           string
	Login              string
	BackfillJobID      *int64
	StrategyVersion    string
	StartOffsetSeconds int
	EndOffsetSeconds   int
	Attempt            int
	LeaseOwner         string
	LeaseExpiresAt     time.Time
	Cursor             string
	CommentsFetched    int
	Status             string
	MaxAttempts        int
	Error              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	HeartbeatAt        *time.Time
}

func (s *Store) UpsertGoldVODSegmentPlans(ctx context.Context, plans []GoldVODSegmentPlan, backfillJobID *int64, maxAttempts int) (int, error) {
	if s == nil || s.db == nil || len(plans) == 0 {
		return 0, nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	insertedOrTouched := 0
	for _, plan := range plans {
		if strings.TrimSpace(plan.SegmentKey) == "" || strings.TrimSpace(plan.VODID) == "" || plan.EndOffsetSeconds <= plan.StartOffsetSeconds {
			continue
		}
		tag, err := s.db.Exec(ctx, `
			INSERT INTO gold_vod_segments (
				segment_key, vod_id, stream_id, login, backfill_job_id, strategy_version,
				start_offset_seconds, end_offset_seconds, max_attempts
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (segment_key) DO UPDATE SET
				stream_id = COALESCE(NULLIF(EXCLUDED.stream_id, ''), gold_vod_segments.stream_id),
				login = COALESCE(NULLIF(EXCLUDED.login, ''), gold_vod_segments.login),
				backfill_job_id = COALESCE(EXCLUDED.backfill_job_id, gold_vod_segments.backfill_job_id),
				max_attempts = GREATEST(gold_vod_segments.max_attempts, EXCLUDED.max_attempts),
				updated_at = now()
			WHERE gold_vod_segments.status IN ('queued','failed','running')`,
			plan.SegmentKey,
			strings.TrimSpace(plan.VODID),
			strings.TrimSpace(plan.StreamID),
			normalizeLogin(plan.Login),
			backfillJobID,
			normalizeGoldSegmentStrategy(plan.StrategyVersion),
			plan.StartOffsetSeconds,
			plan.EndOffsetSeconds,
			maxAttempts,
		)
		if err != nil {
			return insertedOrTouched, err
		}
		if tag.RowsAffected() > 0 {
			insertedOrTouched++
		}
	}
	return insertedOrTouched, nil
}

func (s *Store) ClaimGoldVODSegment(ctx context.Context, owner string, leaseTTL time.Duration, maxSegmentsPerVOD int) (*GoldVODSegmentClaim, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "gold-worker"
	}
	leaseSeconds := durationSeconds(leaseTTL, 120)
	if maxSegmentsPerVOD <= 0 {
		maxSegmentsPerVOD = 1
	}
	var claim GoldVODSegmentClaim
	var backfillJobID int64
	var hasBackfillJobID bool
	var heartbeatAt time.Time
	var hasHeartbeatAt bool
	err := s.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT s.id
			FROM gold_vod_segments s
			WHERE (
					s.status IN ('queued','failed')
					OR (s.status = 'running' AND COALESCE(s.lease_expires_at, '-infinity'::timestamptz) <= now())
				)
			  AND s.next_run_at <= now()
			  AND s.attempt < s.max_attempts
			  AND (
				SELECT COUNT(*)::int
				FROM gold_vod_segments active
				WHERE active.vod_id = s.vod_id
				  AND active.status = 'running'
				  AND COALESCE(active.lease_expires_at, '-infinity'::timestamptz) > now()
			  ) < $3
			ORDER BY s.next_run_at ASC, s.vod_id ASC, s.start_offset_seconds ASC, s.id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE gold_vod_segments s
		SET status = 'running',
		    attempt = s.attempt + 1,
		    lease_owner = $1,
		    lease_expires_at = now() + make_interval(secs => $2),
		    heartbeat_at = now(),
		    error = '',
		    updated_at = now()
		FROM candidate
		WHERE s.id = candidate.id
		RETURNING s.id, s.segment_key, s.vod_id, s.stream_id, s.login,
		          COALESCE(s.backfill_job_id, 0), s.backfill_job_id IS NOT NULL,
		          s.strategy_version, s.start_offset_seconds, s.end_offset_seconds,
		          s.attempt, s.lease_owner, s.lease_expires_at, s.cursor,
		          s.comments_fetched, s.status, s.max_attempts, s.error,
		          s.created_at, s.updated_at, COALESCE(s.heartbeat_at, 'epoch'::timestamptz), s.heartbeat_at IS NOT NULL`,
		owner, leaseSeconds, maxSegmentsPerVOD,
	).Scan(
		&claim.ID,
		&claim.SegmentKey,
		&claim.VODID,
		&claim.StreamID,
		&claim.Login,
		&backfillJobID,
		&hasBackfillJobID,
		&claim.StrategyVersion,
		&claim.StartOffsetSeconds,
		&claim.EndOffsetSeconds,
		&claim.Attempt,
		&claim.LeaseOwner,
		&claim.LeaseExpiresAt,
		&claim.Cursor,
		&claim.CommentsFetched,
		&claim.Status,
		&claim.MaxAttempts,
		&claim.Error,
		&claim.CreatedAt,
		&claim.UpdatedAt,
		&heartbeatAt,
		&hasHeartbeatAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hasBackfillJobID {
		claim.BackfillJobID = &backfillJobID
	}
	if hasHeartbeatAt {
		claim.HeartbeatAt = &heartbeatAt
	}
	return &claim, nil
}

func (s *Store) GoldVODSegmentStatusByKey(ctx context.Context, segmentKey string) (status string, found bool, err error) {
	if s == nil || s.db == nil {
		return "", false, nil
	}
	segmentKey = strings.TrimSpace(segmentKey)
	if segmentKey == "" {
		return "", false, nil
	}
	err = s.db.QueryRow(ctx, `
		SELECT status
		FROM gold_vod_segments
		WHERE segment_key = $1`, segmentKey).Scan(&status)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(status), true, nil
}

func (s *Store) ClaimGoldVODSegmentByKey(ctx context.Context, segmentKey, owner string, leaseTTL time.Duration, maxSegmentsPerVOD int) (*GoldVODSegmentClaim, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	segmentKey = strings.TrimSpace(segmentKey)
	owner = strings.TrimSpace(owner)
	if segmentKey == "" || owner == "" {
		return nil, nil
	}
	leaseSeconds := durationSeconds(leaseTTL, 120)
	if maxSegmentsPerVOD <= 0 {
		maxSegmentsPerVOD = 1
	}
	var claim GoldVODSegmentClaim
	var backfillJobID int64
	var hasBackfillJobID bool
	var heartbeatAt time.Time
	var hasHeartbeatAt bool
	err := s.db.QueryRow(ctx, `
		WITH target AS (
			SELECT id, vod_id
			FROM gold_vod_segments
			WHERE segment_key = $1
		),
		running_count AS (
			SELECT COUNT(*)::int AS n
			FROM gold_vod_segments active
			JOIN target ON active.vod_id = target.vod_id
			WHERE active.status = 'running'
			  AND COALESCE(active.lease_expires_at, '-infinity'::timestamptz) > now()
			  AND active.segment_key <> $1
		)
		UPDATE gold_vod_segments s
		SET status = 'running',
		    attempt = s.attempt + 1,
		    lease_owner = $2,
		    lease_expires_at = now() + make_interval(secs => $3),
		    heartbeat_at = now(),
		    error = '',
		    updated_at = now()
		FROM target, running_count rc
		WHERE s.id = target.id
		  AND (
		    s.status IN ('queued','failed')
		    OR (s.status = 'running' AND COALESCE(s.lease_expires_at, '-infinity'::timestamptz) <= now())
		  )
		  AND s.attempt < s.max_attempts
		  AND rc.n < $4
		RETURNING s.id, s.segment_key, s.vod_id, s.stream_id, s.login,
		          COALESCE(s.backfill_job_id, 0), s.backfill_job_id IS NOT NULL,
		          s.strategy_version, s.start_offset_seconds, s.end_offset_seconds,
		          s.attempt, s.lease_owner, s.lease_expires_at, s.cursor,
		          s.comments_fetched, s.status, s.max_attempts, s.error,
		          s.created_at, s.updated_at, COALESCE(s.heartbeat_at, 'epoch'::timestamptz), s.heartbeat_at IS NOT NULL`,
		segmentKey, owner, leaseSeconds, maxSegmentsPerVOD,
	).Scan(
		&claim.ID,
		&claim.SegmentKey,
		&claim.VODID,
		&claim.StreamID,
		&claim.Login,
		&backfillJobID,
		&hasBackfillJobID,
		&claim.StrategyVersion,
		&claim.StartOffsetSeconds,
		&claim.EndOffsetSeconds,
		&claim.Attempt,
		&claim.LeaseOwner,
		&claim.LeaseExpiresAt,
		&claim.Cursor,
		&claim.CommentsFetched,
		&claim.Status,
		&claim.MaxAttempts,
		&claim.Error,
		&claim.CreatedAt,
		&claim.UpdatedAt,
		&heartbeatAt,
		&hasHeartbeatAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hasBackfillJobID {
		claim.BackfillJobID = &backfillJobID
	}
	if hasHeartbeatAt {
		claim.HeartbeatAt = &heartbeatAt
	}
	return &claim, nil
}

func (s *Store) HeartbeatGoldVODSegment(ctx context.Context, id int64, owner string, leaseTTL time.Duration) (bool, error) {
	if s == nil || s.db == nil || id <= 0 {
		return false, nil
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET heartbeat_at = now(),
		    lease_expires_at = now() + make_interval(secs => $3),
		    updated_at = now()
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2`, id, strings.TrimSpace(owner), durationSeconds(leaseTTL, 120))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) CompleteGoldVODSegment(ctx context.Context, id int64, owner string, commentsFetched int, cursor string) (bool, error) {
	if s == nil || s.db == nil || id <= 0 {
		return false, nil
	}
	if commentsFetched < 0 {
		commentsFetched = 0
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET status = 'done',
		    comments_fetched = GREATEST(comments_fetched, $3),
		    cursor = COALESCE(NULLIF($4, ''), cursor),
		    lease_owner = '',
		    lease_expires_at = NULL,
		    heartbeat_at = NULL,
		    error = '',
		    updated_at = now()
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2`, id, strings.TrimSpace(owner), commentsFetched, strings.TrimSpace(cursor))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) FailGoldVODSegment(ctx context.Context, id int64, owner, errMsg string, retryAfter time.Duration) (bool, error) {
	if s == nil || s.db == nil || id <= 0 {
		return false, nil
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE gold_vod_segments
		SET status = CASE WHEN attempt >= max_attempts THEN 'dead_letter' ELSE 'failed' END,
		    next_run_at = CASE WHEN attempt >= max_attempts THEN next_run_at ELSE now() + make_interval(secs => $4) END,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    heartbeat_at = NULL,
		    error = left($3, 500),
		    updated_at = now()
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2`, id, strings.TrimSpace(owner), strings.TrimSpace(errMsg), durationSeconds(retryAfter, 60))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func durationSeconds(value time.Duration, fallback int) int {
	if value <= 0 {
		return fallback
	}
	seconds := int(math.Ceil(value.Seconds()))
	if seconds <= 0 {
		return fallback
	}
	return seconds
}

func (s *Store) CorpusGoldSegmentSummary(ctx context.Context) (CorpusGoldSegmentSummary, error) {
	var out CorpusGoldSegmentSummary
	if s == nil || s.db == nil {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT status, COUNT(*)::int
		FROM gold_vod_segments
		GROUP BY status`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return out, err
		}
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "queued":
			out.Queued += count
		case "running":
			out.Running += count
		case "done":
			out.Done += count
		case "failed":
			out.Failed += count
		case "dead_letter":
			out.DeadLetter += count
		case "skipped":
			out.Skipped += count
		}
		out.Total += count
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	// Rate-limit readiness uses recent segment failures (PR 0A: gold_vod_rate_limits table was never migrated).
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM gold_vod_segments
		WHERE status IN ('failed', 'running')
		  AND (
		    lower(error) LIKE '%rate limit%'
		    OR lower(error) LIKE '%429%'
		    OR lower(error) LIKE '%throttl%'
		  )
		  AND updated_at > now() - interval '15 minutes'`).Scan(&out.RateLimitedBuckets); err != nil {
		out.RateLimitedBuckets = 0
	}
	return out, nil
}
