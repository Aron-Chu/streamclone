package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GoldEnqueuer scans silver-complete streams and inserts gold-tier backfill jobs.
type GoldEnqueuer struct {
	db     *pgxpool.Pool
	rules  *GoldRulesEngine
	interval time.Duration
}

func NewGoldEnqueuer(db *pgxpool.Pool, rules *GoldRulesEngine, interval time.Duration) *GoldEnqueuer {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &GoldEnqueuer{db: db, rules: rules, interval: interval}
}

type goldCandidate struct {
	StreamID    string
	Login       string
	PeakViewers int
	StartedAt   time.Time
	EndedAt     *time.Time
}

func (e *GoldEnqueuer) RunOnce(ctx context.Context) (int, error) {
	if e == nil || e.db == nil || e.rules == nil {
		return 0, nil
	}
	candidates, err := e.listCandidates(ctx)
	if err != nil {
		return 0, err
	}
	enqueued := 0
	for _, c := range candidates {
		duration := streamDurationMinutes(c.StartedAt, c.EndedAt)
		if !e.rules.Match(c.Login, c.PeakViewers, duration) {
			continue
		}
		if err := insertGoldBackfillJob(ctx, e.db, c.StreamID, c.Login); err != nil {
			return enqueued, err
		}
		enqueued++
	}
	return enqueued, nil
}

func (e *GoldEnqueuer) listCandidates(ctx context.Context) ([]goldCandidate, error) {
	rows, err := e.db.Query(ctx, `
		SELECT s.stream_id, s.login, s.peak_viewers, s.started_at, s.ended_at
		FROM backfill_jobs silver
		JOIN analytics_streams s ON s.stream_id = silver.stream_id
		WHERE silver.tier = 'silver'
		  AND silver.status = 'done'
		  AND silver.export_status = 'confirmed'
		  AND NOT EXISTS (
			SELECT 1 FROM backfill_jobs gold
			WHERE gold.stream_id = silver.stream_id
			  AND gold.tier = 'gold'
			  AND gold.status IN ('queued', 'running', 'done')
		  )`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []goldCandidate
	for rows.Next() {
		var c goldCandidate
		if err := rows.Scan(&c.StreamID, &c.Login, &c.PeakViewers, &c.StartedAt, &c.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func insertGoldBackfillJob(ctx context.Context, db *pgxpool.Pool, streamID, login string) error {
	if db == nil {
		return fmt.Errorf("db unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" || login == "" {
		return fmt.Errorf("stream id and login are required")
	}
	tag, err := db.Exec(ctx, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status, next_run_at)
		SELECT 'gold', $1, $2, 'queued', 'pending', now()
		WHERE NOT EXISTS (
			SELECT 1 FROM backfill_jobs
			WHERE stream_id = $1
			  AND tier = 'gold'
			  AND status IN ('queued', 'running', 'done')
		)`, streamID, login)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gold job already exists or stream %s is not eligible", streamID)
	}
	return nil
}

// EnqueueGoldJob inserts a gold job, optionally bypassing rules when force is true.
func EnqueueGoldJob(ctx context.Context, db *pgxpool.Pool, rules *GoldRulesEngine, streamID, login string, force bool) error {
	if db == nil {
		return fmt.Errorf("db unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("stream id is required")
	}
	store := NewStore(db)
	stream, err := store.StreamByID(ctx, streamID)
	if err != nil {
		return fmt.Errorf("load stream %s: %w", streamID, err)
	}
	if login = normalizeLogin(login); login == "" {
		login = normalizeLogin(stream.Login)
	}
	if login == "" {
		return fmt.Errorf("login is required for stream %s", streamID)
	}
	if !force {
		if rules == nil {
			return fmt.Errorf("gold rules engine is not configured")
		}
		duration := streamDurationMinutes(stream.StartedAt, stream.EndedAt)
		if !rules.Match(login, stream.PeakViewers, duration) {
			return fmt.Errorf("stream %s does not match gold rules", streamID)
		}
	}
	return insertGoldBackfillJob(ctx, db, streamID, login)
}

// EvalGoldRules loads stream metadata and returns a dry-run rules evaluation.
func EvalGoldRules(ctx context.Context, db *pgxpool.Pool, rules *GoldRulesEngine, streamID string) (GoldEval, error) {
	if db == nil {
		return GoldEval{}, fmt.Errorf("db unavailable")
	}
	if rules == nil {
		return GoldEval{}, fmt.Errorf("gold rules engine is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return GoldEval{}, fmt.Errorf("stream id is required")
	}
	stream, err := NewStore(db).StreamByID(ctx, streamID)
	if err != nil {
		return GoldEval{}, err
	}
	duration := streamDurationMinutes(stream.StartedAt, stream.EndedAt)
	return rules.Eval(streamID, stream.Login, stream.PeakViewers, duration), nil
}

// EvalRules returns a dry-run rules evaluation for operator CLI.
func (e *GoldEnqueuer) EvalRules(ctx context.Context, streamID string) (GoldEval, error) {
	if e == nil {
		return GoldEval{}, fmt.Errorf("gold enqueuer is not configured")
	}
	return EvalGoldRules(ctx, e.db, e.rules, streamID)
}

// EnqueueForce inserts a gold job bypassing rules. Returns false when a job already exists.
func (e *GoldEnqueuer) EnqueueForce(ctx context.Context, streamID, login string) (bool, error) {
	if e == nil || e.db == nil {
		return false, fmt.Errorf("gold enqueuer is not configured")
	}
	err := EnqueueGoldJob(ctx, e.db, e.rules, streamID, login, true)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func StartGoldEnqueuer(ctx context.Context, enqueuer *GoldEnqueuer, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if enqueuer == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(enqueuer.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := enqueuer.RunOnce(ctx)
				if err != nil && log != nil {
					log.Warn("gold enqueuer tick failed", "err", err)
				} else if n > 0 && log != nil {
					log.Info("gold enqueuer inserted jobs", "count", n)
				}
			}
		}
	}()
}
