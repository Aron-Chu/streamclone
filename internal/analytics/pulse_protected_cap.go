package analytics

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ErrProtectedCapReached is returned when PULSE_PROTECTED_CHANNEL_LIMIT_GLOBAL is exhausted.
var ErrProtectedCapReached = errors.New("protected_cap_reached")

// protectedCapAdvisoryKey serializes global protect writes across watchlist and always-tracked tables.
const protectedCapAdvisoryKey int64 = 0x50754c5f50524f54 // "PUL_PROT"

type protectedCapError struct {
	scope string
}

func (e *protectedCapError) Error() string { return ErrProtectedCapReached.Error() }

// CountDistinctProtectedLogins returns distinct normalized logins protected globally.
// Same login in pulse_watchlist(always_track) and analytics_always_tracked counts once.
func (s *Store) CountDistinctProtectedLogins(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT lower(login) AS login FROM pulse_watchlist WHERE always_track = true
			UNION
			SELECT lower(login) AS login FROM analytics_always_tracked
		) protected`).Scan(&count)
	return count, err
}

// IsLoginGloballyProtected reports whether login is protected in any global source.
func (s *Store) IsLoginGloballyProtected(ctx context.Context, login string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	login = normalizeLogin(login)
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pulse_watchlist WHERE always_track = true AND lower(login) = lower($1)
			UNION ALL
			SELECT 1 FROM analytics_always_tracked WHERE lower(login) = lower($1)
		)`, login).Scan(&exists)
	return exists, err
}

func countDistinctProtectedLoginsTx(ctx context.Context, tx pgx.Tx) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT lower(login) AS login FROM pulse_watchlist WHERE always_track = true
			UNION
			SELECT lower(login) AS login FROM analytics_always_tracked
		) protected`).Scan(&count)
	return count, err
}

func isLoginGloballyProtectedTx(ctx context.Context, tx pgx.Tx, login string) (bool, error) {
	login = normalizeLogin(login)
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pulse_watchlist WHERE always_track = true AND lower(login) = lower($1)
			UNION ALL
			SELECT 1 FROM analytics_always_tracked WHERE lower(login) = lower($1)
		)`, login).Scan(&exists)
	return exists, err
}

func acquireProtectedCapLock(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, protectedCapAdvisoryKey)
	return err
}

func checkProtectedCapTx(ctx context.Context, tx pgx.Tx, login string, globalLimit int) error {
	if globalLimit <= 0 {
		return &protectedCapError{scope: "protected_pool"}
	}
	protected, err := isLoginGloballyProtectedTx(ctx, tx, login)
	if err != nil {
		return err
	}
	if protected {
		return nil
	}
	count, err := countDistinctProtectedLoginsTx(ctx, tx)
	if err != nil {
		return err
	}
	return evaluateProtectedCap(globalLimit, count, false)
}

// UpsertPulseWatchlistWithCap upserts watchlist under the global protected-cap advisory lock.
func (s *Store) UpsertPulseWatchlistWithCap(
	ctx context.Context,
	principalID, principalKind, login string,
	alwaysTrack bool,
	globalLimit int,
) (PulseWatchlistEntry, error) {
	if s == nil || s.db == nil {
		return PulseWatchlistEntry{}, fmt.Errorf("store unavailable")
	}
	login = normalizeLogin(login)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PulseWatchlistEntry{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := acquireProtectedCapLock(ctx, tx); err != nil {
		return PulseWatchlistEntry{}, err
	}
	if alwaysTrack {
		if err := checkProtectedCapTx(ctx, tx, login, globalLimit); err != nil {
			return PulseWatchlistEntry{}, err
		}
	}

	id := newPulseWatchlistID()
	var item PulseWatchlistEntry
	err = tx.QueryRow(ctx, `
		INSERT INTO pulse_watchlist (id, principal_id, principal_kind, login, always_track)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (principal_id, login) DO UPDATE SET
			always_track = EXCLUDED.always_track,
			updated_at = now()
		RETURNING id, login, always_track, created_at, updated_at`,
		id, principalID, principalKind, login, alwaysTrack).Scan(
		&item.ID, &item.Login, &item.AlwaysTrack, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return PulseWatchlistEntry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PulseWatchlistEntry{}, err
	}
	return item, nil
}

// AddAlwaysTrackedWithCap inserts into analytics_always_tracked under the global protected-cap lock.
func (s *Store) AddAlwaysTrackedWithCap(ctx context.Context, login string, globalLimit int) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store unavailable")
	}
	login = normalizeLogin(login)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := acquireProtectedCapLock(ctx, tx); err != nil {
		return err
	}
	if err := checkProtectedCapTx(ctx, tx, login, globalLimit); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO analytics_always_tracked (login)
		VALUES ($1)
		ON CONFLICT (login) DO NOTHING`, login); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isProtectedCapError(err error) bool {
	var capErr *protectedCapError
	return errors.As(err, &capErr) || errors.Is(err, ErrProtectedCapReached)
}

// evaluateProtectedCap is the pure cap decision used inside the transactional guard.
func evaluateProtectedCap(globalLimit, currentCount int, alreadyProtected bool) error {
	if globalLimit <= 0 {
		return &protectedCapError{scope: "protected_pool"}
	}
	if alreadyProtected {
		return nil
	}
	if currentCount >= globalLimit {
		return &protectedCapError{scope: "protected_pool"}
	}
	return nil
}

func writeProtectedCapReached(w http.ResponseWriter) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error": "protected_cap_reached",
		"scope": "protected_pool",
	})
}
