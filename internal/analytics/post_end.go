package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostEndDetector enqueues TT gap-fill jobs after streams end.
type PostEndDetector struct {
	db       *pgxpool.Pool
	waitMin  time.Duration
	waitMax  time.Duration
	coverage float64
}

func NewPostEndDetector(db *pgxpool.Pool, waitMin, waitMax time.Duration, coverageThreshold float64) *PostEndDetector {
	if waitMin <= 0 {
		waitMin = 10 * time.Minute
	}
	if waitMax <= 0 {
		waitMax = 30 * time.Minute
	}
	if coverageThreshold <= 0 {
		coverageThreshold = 70
	}
	return &PostEndDetector{db: db, waitMin: waitMin, waitMax: waitMax, coverage: coverageThreshold}
}

func (p *PostEndDetector) EnqueueIfNeeded(ctx context.Context, streamID, login string) error {
	if p == nil || p.db == nil || streamID == "" {
		return nil
	}
	store := NewStore(p.db)
	stream, err := store.StreamByID(ctx, streamID)
	if err != nil {
		return err
	}
	rollups, err := store.RollupsByStream(ctx, streamID)
	if err != nil {
		return err
	}
	if hasGoodViewerCoverageFromRollups(rollups, stream) || Tier0CoveragePct(stream, rollups) >= p.coverage {
		_, err = p.db.Exec(ctx, `
			INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status, next_run_at)
			SELECT 'silver', $1, $2, 'skipped', 'skipped', now()
			WHERE NOT EXISTS (
				SELECT 1 FROM backfill_jobs
				WHERE stream_id = $1 AND status IN ('queued','running','skipped','done')
			)`, streamID, normalizeLogin(login))
		return err
	}
	runAt := time.Now().UTC().Add(p.waitMin + time.Duration(time.Now().UnixNano()%int64(p.waitMax-p.waitMin)))
	_, err = p.db.Exec(ctx, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status, next_run_at)
		SELECT 'silver', $1, $2, 'queued', 'pending', $3
		WHERE NOT EXISTS (
			SELECT 1 FROM backfill_jobs
			WHERE stream_id = $1 AND status IN ('queued','running')
		)`, streamID, normalizeLogin(login), runAt)
	return err
}
