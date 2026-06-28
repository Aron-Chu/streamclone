package analytics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTop500MetadataMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "000044_top500_metadata.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)
	mustContain := []string{
		"CREATE TABLE IF NOT EXISTS top500_channels",
		"CREATE TABLE IF NOT EXISTS top500_live_snapshots",
		"PARTITION BY RANGE (sample_tick_at)",
		"UNIQUE (channel_id, sample_tick_at)",
		"CREATE TABLE IF NOT EXISTS top500_current",
		"CHECK (source IN ('operator_seed', 'configured'))",
	}
	for _, want := range mustContain {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	mustNotContain := []string{
		"top500_viewer_rollups",
		"backfill_jobs",
		"CREATE TRIGGER",
		"CREATE EXTENSION",
	}
	for _, forbidden := range mustNotContain {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains forbidden %q", forbidden)
		}
	}
}

func TestTop500MetadataValidation(t *testing.T) {
	if err := validateTop500Channel(Top500Channel{ChannelID: "1", Login: "xqc", Rank: 1, Source: "helix"}); err == nil {
		t.Fatal("expected unsupported channel source error")
	}
	if err := validateTop500LiveSnapshot(Top500LiveSnapshot{ChannelID: "1", Login: "xqc", SampleTickAt: time.Now(), SampledAt: time.Now(), Source: "scraper"}); err == nil {
		t.Fatal("expected unsupported snapshot source error")
	}
	if err := validateTop500Current(Top500Current{ChannelID: "1", Login: "xqc", Rank: 1, CoverageSource: "backfill", SampledAt: time.Now(), StaleAfter: time.Now().Add(15 * time.Minute)}); err == nil {
		t.Fatal("expected unsupported coverage source error")
	}
}

func TestTop500CurrentFreshnessSeconds(t *testing.T) {
	now := time.Date(2026, 6, 24, 17, 30, 0, 0, time.UTC)
	current := Top500Current{SampledAt: now.Add(-90 * time.Second)}
	got := current.FreshnessSeconds(now)
	if got == nil || *got != 90 {
		t.Fatalf("freshness = %v, want 90", got)
	}
}

func TestTop500StoreUpsertsCurrentAndSnapshot(t *testing.T) {
	ctx, store := setupTop500Store(t)
	now := time.Now().UTC().Truncate(time.Second)
	streamID := "stream-1"
	viewerCount := 1200
	startedAt := now.Add(-2 * time.Hour)
	lastSuccessAt := now

	if err := store.UpsertTop500Channel(ctx, Top500Channel{
		ChannelID:      "100",
		Login:          "XQC",
		DisplayName:    "xQc",
		Rank:           1,
		Source:         Top500ChannelSourceOperatorSeed,
		SourceVersion:  "test-seed",
		SeededBy:       "test",
		EffectiveAt:    now,
		SourceMetadata: map[string]any{"batch": "i1"},
		Enabled:        true,
		LastSeenAt:     &now,
	}); err != nil {
		t.Fatalf("upsert channel: %v", err)
	}
	if err := store.UpsertTop500Current(ctx, Top500Current{
		ChannelID:      "100",
		Login:          "XQC",
		DisplayName:    "xQc",
		Rank:           1,
		CoverageSource: Top500CoverageSourceMetadata,
		IsLive:         true,
		StreamID:       &streamID,
		Title:          "test stream",
		CategoryID:     "509658",
		CategoryName:   "Just Chatting",
		StartedAt:      &startedAt,
		ViewerCount:    &viewerCount,
		Language:       "en",
		Tags:           []string{"English"},
		SampledAt:      now,
		StaleAfter:     now.Add(15 * time.Minute),
		LastSuccessAt:  &lastSuccessAt,
	}); err != nil {
		t.Fatalf("upsert current: %v", err)
	}

	byLogin, err := store.GetTop500CurrentByLogin(ctx, "xqc")
	if err != nil {
		t.Fatalf("get by login: %v", err)
	}
	if byLogin == nil || byLogin.ChannelID != "100" || byLogin.StreamID == nil || *byLogin.StreamID != streamID {
		t.Fatalf("current by login = %#v", byLogin)
	}
	byID, err := store.GetTop500CurrentByChannelID(ctx, "100")
	if err != nil {
		t.Fatalf("get by channel id: %v", err)
	}
	if byID == nil || byID.Login != "xqc" || len(byID.Tags) != 1 || byID.Tags[0] != "English" {
		t.Fatalf("current by id = %#v", byID)
	}

	if err := store.UpsertTop500LiveSnapshot(ctx, Top500LiveSnapshot{
		ChannelID:    "100",
		Login:        "XQC",
		StreamID:     &streamID,
		IsLive:       true,
		Title:        "test stream",
		CategoryID:   "509658",
		CategoryName: "Just Chatting",
		StartedAt:    &startedAt,
		ViewerCount:  &viewerCount,
		Language:     "en",
		Tags:         []string{"English"},
		SampleTickAt: now,
		SampledAt:    now,
		Source:       Top500SnapshotSourceHelixStreams,
	}); err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}
	updatedViewerCount := 1300
	if err := store.UpsertTop500LiveSnapshot(ctx, Top500LiveSnapshot{
		ChannelID:    "100",
		Login:        "xqc",
		StreamID:     &streamID,
		IsLive:       true,
		Title:        "test stream updated",
		CategoryID:   "509658",
		CategoryName: "Just Chatting",
		StartedAt:    &startedAt,
		ViewerCount:  &updatedViewerCount,
		Language:     "en",
		Tags:         []string{"English"},
		SampleTickAt: now,
		SampledAt:    now.Add(5 * time.Second),
		Source:       Top500SnapshotSourceHelixStreams,
	}); err != nil {
		t.Fatalf("upsert snapshot retry: %v", err)
	}

	var rows int
	var storedViewerCount int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*), max(viewer_count) FROM top500_live_snapshots WHERE channel_id='100'`).Scan(&rows, &storedViewerCount); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if rows != 1 || storedViewerCount != updatedViewerCount {
		t.Fatalf("snapshot rows=%d viewer=%d, want 1/%d", rows, storedViewerCount, updatedViewerCount)
	}
}

func TestTop500StoreWriteSamplesRollsBackCurrentWhenSnapshotFails(t *testing.T) {
	ctx, store := setupTop500Store(t)
	now := time.Now().UTC().Truncate(time.Second)
	err := store.WriteTop500MetadataSamples(ctx, []Top500MetadataSample{{
		Snapshot: Top500LiveSnapshot{
			ChannelID:    "rollback-1",
			Login:        "rollbackchan",
			IsLive:       true,
			SampleTickAt: now,
			SampledAt:    now,
			Source:       "invalid_source",
		},
		Current: Top500Current{
			ChannelID:      "rollback-1",
			Login:          "rollbackchan",
			DisplayName:    "RollbackChan",
			Rank:           1,
			CoverageSource: Top500CoverageSourceMetadata,
			IsLive:         true,
			SampledAt:      now,
			StaleAfter:     now.Add(15 * time.Minute),
			LastSuccessAt:  &now,
		},
	}})
	if err == nil {
		t.Fatal("expected invalid snapshot source error")
	}
	var currentRows int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_current WHERE channel_id='rollback-1'`).Scan(&currentRows); err != nil {
		t.Fatalf("count current rows: %v", err)
	}
	if currentRows != 0 {
		t.Fatalf("current rows after failed snapshot write = %d, want 0", currentRows)
	}
}

func setupTop500Store(t *testing.T) (context.Context, *Store) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run top500 metadata integration tests")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:test@localhost:15432/emotes?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pgxpool.New: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("top500_metadata_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("test pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)

	store := NewStore(pool)
	analyticsMigrationPath := filepath.Join("..", "..", "migrations", "000005_analytics.up.sql")
	analyticsMigrationSQL, err := os.ReadFile(analyticsMigrationPath)
	if err != nil {
		t.Fatalf("read analytics migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(analyticsMigrationSQL)); err != nil {
		t.Fatalf("apply analytics migration: %v", err)
	}
	migrationPath := filepath.Join("..", "..", "migrations", "000044_top500_metadata.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply top500 migration: %v", err)
	}
	return ctx, store
}
