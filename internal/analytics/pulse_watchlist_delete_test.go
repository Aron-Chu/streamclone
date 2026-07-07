package analytics

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupWatchlistDeleteStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL required for watchlist delete integration test")
	}

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	schema := "watchlist_delete_" + strings.ReplaceAll(t.Name(), "/", "_")
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": schema}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	store := NewStore(pool)
	mustExecWatchlistDelete(t, ctx, store, `CREATE SCHEMA IF NOT EXISTS `+schema)
	mustExecWatchlistDelete(t, ctx, store, `SET search_path TO `+schema)
	mustExecWatchlistDelete(t, ctx, store, `
		CREATE TABLE pulse_watchlist (
			id TEXT PRIMARY KEY,
			principal_id TEXT NOT NULL,
			principal_kind TEXT NOT NULL DEFAULT 'beta',
			login TEXT NOT NULL,
			always_track BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (principal_id, login)
		);
		CREATE TABLE analytics_always_tracked (
			login TEXT PRIMARY KEY
		)`)
	return ctx, store
}

func mustExecWatchlistDelete(t *testing.T, ctx context.Context, store *Store, sql string, args ...any) {
	t.Helper()
	if _, err := store.db.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec failed: %v\nSQL: %s", err, sql)
	}
}

func TestIsLoginGloballyProtectedAfterOnePrincipalDeletes(t *testing.T) {
	ctx, store := setupWatchlistDeleteStore(t)

	if _, err := store.UpsertPulseWatchlist(ctx, "principal-a", "beta", "xqc", true); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if _, err := store.UpsertPulseWatchlist(ctx, "principal-b", "beta", "xqc", true); err != nil {
		t.Fatalf("upsert b: %v", err)
	}

	protected, err := store.IsLoginGloballyProtected(ctx, "xqc")
	if err != nil || !protected {
		t.Fatalf("expected protected before delete, got protected=%v err=%v", protected, err)
	}

	if err := store.DeletePulseWatchlist(ctx, "principal-a", "xqc"); err != nil {
		t.Fatalf("delete a: %v", err)
	}

	protected, err = store.IsLoginGloballyProtected(ctx, "xqc")
	if err != nil || !protected {
		t.Fatalf("expected still protected after one principal delete, got protected=%v err=%v", protected, err)
	}

	if err := store.DeletePulseWatchlist(ctx, "principal-b", "xqc"); err != nil {
		t.Fatalf("delete b: %v", err)
	}

	protected, err = store.IsLoginGloballyProtected(ctx, "xqc")
	if err != nil || protected {
		t.Fatalf("expected unprotected after both delete, got protected=%v err=%v", protected, err)
	}
}
