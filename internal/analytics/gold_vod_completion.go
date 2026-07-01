package analytics

import (
	"context"
	"fmt"
	"strings"
)

// GoldVODSegmentUnresolvedSummary counts durable gold_vod_segments rows that block job completion.
type GoldVODSegmentUnresolvedSummary struct {
	Queued     int
	Running    int
	Failed     int
	DeadLetter int
}

func (s GoldVODSegmentUnresolvedSummary) BlocksCompletion() bool {
	return s.Queued+s.Running+s.Failed+s.DeadLetter > 0
}

func (s GoldVODSegmentUnresolvedSummary) TotalBlocking() int {
	return s.Queued + s.Running + s.Failed + s.DeadLetter
}

func goldVODSegmentsIncompleteError(summary GoldVODSegmentUnresolvedSummary) error {
	if !summary.BlocksCompletion() {
		return nil
	}
	return fmt.Errorf(
		"gold vod segments incomplete: queued=%d running=%d failed=%d dead_letter=%d",
		summary.Queued,
		summary.Running,
		summary.Failed,
		summary.DeadLetter,
	)
}

// GoldVODSegmentUnresolvedSummary returns blocking segment counts for a gold backfill job.
// Rows scoped to backfill_job_id are preferred; when none exist, falls back to vod_id on stream_id.
func (s *Store) GoldVODSegmentUnresolvedSummary(ctx context.Context, jobID int64, streamID string) (GoldVODSegmentUnresolvedSummary, error) {
	var out GoldVODSegmentUnresolvedSummary
	if s == nil || s.db == nil || jobID <= 0 {
		return out, nil
	}
	var total int
	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued')::int,
			COUNT(*) FILTER (WHERE status = 'running')::int,
			COUNT(*) FILTER (WHERE status = 'failed')::int,
			COUNT(*) FILTER (WHERE status = 'dead_letter')::int,
			COUNT(*)::int
		FROM gold_vod_segments
		WHERE backfill_job_id = $1`,
		jobID,
	).Scan(&out.Queued, &out.Running, &out.Failed, &out.DeadLetter, &total)
	if err != nil {
		return out, err
	}
	if total > 0 {
		return out, nil
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return out, nil
	}
	var vodID string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(vod_id, '')
		FROM analytics_streams
		WHERE stream_id = $1`, streamID).Scan(&vodID); err != nil {
		return out, err
	}
	vodID = strings.TrimSpace(vodID)
	if vodID == "" {
		return out, nil
	}
	err = s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued')::int,
			COUNT(*) FILTER (WHERE status = 'running')::int,
			COUNT(*) FILTER (WHERE status = 'failed')::int,
			COUNT(*) FILTER (WHERE status = 'dead_letter')::int
		FROM gold_vod_segments
		WHERE vod_id = $1`,
		vodID,
	).Scan(&out.Queued, &out.Running, &out.Failed, &out.DeadLetter)
	return out, err
}

func goldVODSegmentsLedgerEmptyError(jobID int64) error {
	return fmt.Errorf("gold vod segments ledger empty for job %d", jobID)
}

func (s *SyncService) goldVODSegmentsBlockCompletion(ctx context.Context, jobID int64, streamID string) error {
	if s == nil || !s.goldVODSegmentsEnabled || s.store == nil || jobID <= 0 {
		return nil
	}
	jobRows, err := s.store.GoldVODSegmentJobRowCount(ctx, jobID)
	if err != nil {
		return fmt.Errorf("gold vod segments completion check: %w", err)
	}
	if jobRows == 0 {
		return goldVODSegmentsLedgerEmptyError(jobID)
	}
	summary, err := s.store.GoldVODSegmentUnresolvedSummary(ctx, jobID, streamID)
	if err != nil {
		return fmt.Errorf("gold vod segments completion check: %w", err)
	}
	return goldVODSegmentsIncompleteError(summary)
}

func (w *BackfillWorker) applyGoldSegmentCompletionGate(ctx context.Context, job *BackfillJob, streamID string, syncErr error) error {
	if syncErr != nil || w == nil || w.sync == nil || job == nil || !isGoldFullTier(job.Tier) {
		return syncErr
	}
	return w.sync.goldVODSegmentsBlockCompletion(ctx, job.ID, streamID)
}
