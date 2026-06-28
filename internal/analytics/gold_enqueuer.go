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
	db       *pgxpool.Pool
	rules    *GoldRulesEngine
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
		SELECT silver.stream_id, silver.login
		FROM backfill_jobs silver
		WHERE silver.tier = 'silver'
		  AND silver.status = 'done'
		  AND silver.export_status = 'confirmed'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	store := NewStore(e.db)
	seen := map[string]bool{}
	var out []goldCandidate
	for rows.Next() {
		var silverStreamID, fallbackLogin string
		if err := rows.Scan(&silverStreamID, &fallbackLogin); err != nil {
			return nil, err
		}
		stream, err := store.StreamByID(ctx, silverStreamID)
		if err != nil {
			continue
		}
		canonicalID := strings.TrimSpace(stream.StreamID)
		if canonicalID == "" {
			continue
		}
		if resolved, err := store.ResolveCanonicalStreamID(ctx, canonicalID); err == nil && resolved != "" {
			canonicalID = resolved
		}
		if err := store.EnsureSessionForStream(ctx, canonicalID); err != nil {
			continue
		}
		if seen[canonicalID] {
			continue
		}
		exists, err := goldBackfillJobExists(ctx, e.db, canonicalID, silverStreamID)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		seen[canonicalID] = true
		c := goldCandidate{
			StreamID:    canonicalID,
			Login:       normalizeLogin(stream.Login),
			PeakViewers: stream.PeakViewers,
			StartedAt:   stream.StartedAt,
			EndedAt:     stream.EndedAt,
		}
		if c.Login == "" {
			c.Login = normalizeLogin(fallbackLogin)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func goldBackfillJobExists(ctx context.Context, db *pgxpool.Pool, streamIDs ...string) (bool, error) {
	ids := uniqueStrings(streamIDs)
	if len(ids) == 0 {
		return false, nil
	}
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM backfill_jobs
			WHERE stream_id = ANY($1)
			  AND tier = 'gold'
			  AND status IN ('queued', 'running', 'done')
		)`, ids).Scan(&exists)
	return exists, err
}

func insertGoldBackfillJob(ctx context.Context, db *pgxpool.Pool, streamID, login string) error {
	_, inserted, err := insertGoldBackfillJobReturningID(ctx, db, streamID, login)
	if err != nil {
		return err
	}
	if !inserted {
		return fmt.Errorf("gold job already exists or stream %s is not eligible", strings.TrimSpace(streamID))
	}
	return nil
}

func insertGoldBackfillJobReturningID(ctx context.Context, db *pgxpool.Pool, streamID, login string) (int64, bool, error) {
	if db == nil {
		return 0, false, fmt.Errorf("db unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" || login == "" {
		return 0, false, fmt.Errorf("stream id and login are required")
	}
	store := NewStore(db)
	duplicateIDs := []string{streamID}
	if canonicalID, err := store.ResolveCanonicalStreamID(ctx, streamID); err == nil && canonicalID != "" {
		streamID = canonicalID
		duplicateIDs = append(duplicateIDs, canonicalID)
	}
	duplicateIDs = uniqueStrings(duplicateIDs)
	var jobID int64
	err := db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status, next_run_at)
			SELECT 'gold', $1, $2, 'queued', 'pending', now()
			WHERE NOT EXISTS (
				SELECT 1 FROM backfill_jobs
				WHERE stream_id = ANY($3)
				  AND (
					status IN ('queued', 'running')
					OR (tier = 'gold' AND status = 'done')
				  )
			)
			RETURNING id
		)
		SELECT COALESCE((SELECT id FROM inserted), 0)`, streamID, login, duplicateIDs).Scan(&jobID)
	if err != nil {
		return 0, false, err
	}
	if jobID == 0 {
		return 0, false, nil
	}
	return jobID, true, nil
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
