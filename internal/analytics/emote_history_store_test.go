package analytics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEmoteHistoryStoreIntegration(t *testing.T) {
	ctx, store := setupEmoteHistoryStore(t)
	base := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	first := SaveEmoteSnapshotInput{
		TwitchID:      "71092938",
		Login:         "xqc",
		Provider:      "7tv",
		ProviderSetID: "set-a",
		FetchedAt:     base,
		EffectiveAt:   base,
		Complete:      true,
		HTTPStatus:    200,
		Source:        "integration_test",
		Items: []EmoteSnapshotItem{
			{Provider: "seventv", ProviderEmoteID: "emote-a", ProviderSetID: "set-a", Alias: "OMEGALUL", CanonicalName: "OMEGALUL"},
			{Provider: "seventv", ProviderEmoteID: "emote-b", ProviderSetID: "set-a", Alias: "KEKW", CanonicalName: "KEKW"},
		},
	}
	result, err := store.SaveEmoteSnapshot(ctx, first)
	if err != nil {
		t.Fatalf("save first snapshot: %v", err)
	}
	if !result.Created || result.SnapshotID == "" {
		t.Fatalf("first snapshot result = %+v", result)
	}

	unchanged := first
	unchanged.FetchedAt = base.Add(10 * time.Minute)
	unchanged.EffectiveAt = unchanged.FetchedAt
	result, err = store.SaveEmoteSnapshot(ctx, unchanged)
	if err != nil {
		t.Fatalf("save unchanged snapshot: %v", err)
	}
	if !result.Unchanged || result.Created {
		t.Fatalf("unchanged snapshot result = %+v", result)
	}
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_set_snapshots WHERE twitch_id='71092938'`, 1)

	second := first
	second.FetchedAt = base.Add(20 * time.Minute)
	second.EffectiveAt = second.FetchedAt
	second.Items = []EmoteSnapshotItem{
		{Provider: "seventv", ProviderEmoteID: "emote-a", ProviderSetID: "set-a", Alias: "OMEGA", CanonicalName: "OMEGALUL"},
		{Provider: "seventv", ProviderEmoteID: "emote-c", ProviderSetID: "set-a", Alias: "catJAM", CanonicalName: "catJAM"},
	}
	result, err = store.SaveEmoteSnapshot(ctx, second)
	if err != nil {
		t.Fatalf("save changed snapshot: %v", err)
	}
	if !result.Created || len(result.Diff.Removed) != 1 || len(result.Diff.Added) != 1 || len(result.Diff.AliasChanges) != 1 {
		t.Fatalf("changed snapshot diff = %+v", result.Diff)
	}
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_membership_periods WHERE provider_emote_id='emote-b' AND valid_to IS NOT NULL`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_alias_history WHERE provider_emote_id='emote-a' AND alias='OMEGALUL' AND valid_to IS NOT NULL`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_alias_history WHERE provider_emote_id='emote-a' AND alias='OMEGA' AND valid_to IS NULL`, 1)

	beforeClosed := scalarInt(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_membership_periods WHERE valid_to IS NOT NULL`)
	if err := store.RecordEmoteSnapshotFailure(ctx, EmoteSnapshotFailureInput{TwitchID: "71092938", Login: "xqc", Provider: "seventv", State: "failed", Error: "provider timeout"}); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_providers WHERE twitch_id='71092938' AND provider='seventv' AND snapshot_state='failed' AND snapshot_error='provider timeout'`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_membership_periods WHERE valid_to IS NOT NULL`, beforeClosed)

	if err := store.RecordEmoteSnapshotFailure(ctx, EmoteSnapshotFailureInput{TwitchID: "999", Login: "smallchan", Provider: "seventv", State: "failed", Error: "not found"}); err != nil {
		t.Fatalf("record failure for new channel: %v", err)
	}
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channels WHERE twitch_id='999' AND login='smallchan'`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM channel_emote_providers WHERE twitch_id='999' AND provider='seventv' AND snapshot_state='failed'`, 1)

	insertEmoteHistoryRollups(t, ctx, store, base)
	normalized, err := store.NormalizeEmoteUsageForChannel(ctx, "xqc", base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("normalize usage: %v", err)
	}
	if normalized.Streams != 1 || normalized.Minutes != 2 || normalized.Rows != 7 {
		t.Fatalf("normalized result = %+v", normalized)
	}
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc'`, 7)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc' AND identity_resolution='provider_id'`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc' AND identity_resolution='alias_fallback'`, 2)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc' AND identity_resolution='ambiguous'`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc' AND identity_resolution='unresolved'`, 3)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_minute_rollups WHERE login='xqc' AND source_key='7tv:catJAM' AND provider='seventv' AND provider_emote_id='emote-c'`, 1)
	assertCount(t, ctx, store, `SELECT COUNT(*) FROM emote_usage_stream_rollups WHERE login='xqc'`, 7)

	portal, err := store.PortalChannelEmotes(ctx, "xqc", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("portal emotes: %v", err)
	}
	if portal.Coverage.MinutesWithData != 5 || portal.Coverage.NormalizedMinutes != 2 || len(portal.TopEmotes) == 0 || len(portal.History) == 0 {
		t.Fatalf("portal response = %+v", portal)
	}
	if portal.Coverage.ChatCoveragePct != 40 || !portal.Partial || !portal.LowConfidence {
		t.Fatalf("portal coverage flags = coverage=%+v partial=%v low=%v", portal.Coverage, portal.Partial, portal.LowConfidence)
	}
	if portal.IdentityResolutionPct <= 0 || portal.Freshness.ProviderState == "" {
		t.Fatalf("portal trust fields missing = %+v", portal)
	}
}

func TestEmoteHistoryNormalizerRejectsMalformedRollupJSON(t *testing.T) {
	var emotes map[string]int
	if err := jsonUnmarshalEmoteCounts([]byte(`{"KEKW":"not-a-count"}`), &emotes); err == nil {
		t.Fatal("expected malformed emote count JSON to fail")
	}
}

func setupEmoteHistoryStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run emote history integration tests")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5432/streamclone?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pgxpool.New: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("emote_history_test_%d", time.Now().UnixNano())
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
	for _, migration := range []string{
		"000002_emotes.up.sql",
		"000003_provider_emotes.up.sql",
		"000004_channel_emote_providers.up.sql",
		"000005_analytics.up.sql",
		"000022_emote_provider_integrity.up.sql",
		"000045_emote_name_text.up.sql",
		"000048_emote_history_phase1a.up.sql",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
	return ctx, store
}

func insertEmoteHistoryRollups(t *testing.T, ctx context.Context, store *Store, base time.Time) {
	t.Helper()
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, broadcaster_id, login, display_name, started_at)
		VALUES ('stream-1','71092938','xqc','xQc',$1)`, base.Add(-10*time.Minute))
	mustExec(t, ctx, store, `
		INSERT INTO emote_alias_history (twitch_id, login, provider, provider_emote_id, alias, valid_from, first_seen_by_us, last_seen_by_us)
		VALUES ('71092938','xqc','bttv','bttv-a','OMEGA',$1,$1,$1)`, base)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count, total_emote_count, seventv_emote_count, emotes_json)
		VALUES
		('stream-1',$1,12,8,7,'{"seventv:emote-a:OMEGALUL": 5, "KEKW": 2, "bad:key": 1}'::jsonb),
		('stream-1',$2,8,6,3,'{"7tv:catJAM": 3, "Nope": 1, "OMEGA": 2, "unknown/name": 1}'::jsonb),
		('stream-1',$3,0,0,0,'{}'::jsonb),
		('stream-1',$4,0,0,0,'{}'::jsonb),
		('stream-1',$5,0,0,0,'{}'::jsonb)`, base.Add(10*time.Minute), base.Add(22*time.Minute), base.Add(23*time.Minute), base.Add(24*time.Minute), base.Add(25*time.Minute))
}

func assertCount(t *testing.T, ctx context.Context, store *Store, query string, want int) {
	t.Helper()
	got := scalarInt(t, ctx, store, query)
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}

func scalarInt(t *testing.T, ctx context.Context, store *Store, query string) int {
	t.Helper()
	var got int
	if err := store.db.QueryRow(ctx, query).Scan(&got); err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	return got
}
