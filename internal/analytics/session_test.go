package analytics

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPickCanonicalSessionTieBreaksDeterministically(t *testing.T) {
	got := pickCanonicalSession(
		sessionCandidate{StreamID: "stream-b", Login: "chan"},
		sessionCandidate{StreamID: "stream-a", Login: "chan"},
	)
	if got.StreamID != "stream-a" {
		t.Fatalf("winner = %q, want lexical stream-a", got.StreamID)
	}
}

func TestSessionsMatchRejectsDistinctTwitchStreamIDs(t *testing.T) {
	now := time.Now().UTC()
	existing := sessionCandidate{
		StreamID:       "old-live-stream",
		CanonicalID:    "old-live-stream",
		Login:          "lacy",
		TwitchStreamID: "old-live-stream",
		StartedAt:      now.Add(-30 * time.Minute),
	}
	incoming := sessionCandidate{
		StreamID:       "new-live-stream",
		CanonicalID:    "new-live-stream",
		Login:          "lacy",
		TwitchStreamID: "new-live-stream",
		StartedAt:      now,
	}

	if sessionsMatch(incoming, existing) {
		t.Fatal("distinct Twitch live stream ids should not merge through open-ended overlap")
	}
}

func TestSessionsMatchAllowsOverlapWhenOnlyOneTwitchStreamIDIsKnown(t *testing.T) {
	now := time.Now().UTC()
	existing := sessionCandidate{
		StreamID:       "live-stream",
		CanonicalID:    "live-stream",
		Login:          "chan",
		TwitchStreamID: "live-stream",
		StartedAt:      now.Add(-30 * time.Minute),
	}
	incoming := sessionCandidate{
		StreamID:   "tt-stream",
		Login:      "chan",
		TTStreamID: "tt-stream",
		StartedAt:  now.Add(-20 * time.Minute),
	}

	if !sessionsMatch(incoming, existing) {
		t.Fatal("overlap should still match when only one side has a Twitch stream id")
	}
}

func TestResolveCanonicalStreamIDFollowsAliasChain(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 1)
	insertTestStream(t, ctx, store, "c", "chan", 2)
	insertTestAlias(t, ctx, store, "a", "b")
	insertTestAlias(t, ctx, store, "b", "c")

	got, err := store.ResolveCanonicalStreamID(ctx, "a")
	if err != nil {
		t.Fatalf("ResolveCanonicalStreamID: %v", err)
	}
	if got != "c" {
		t.Fatalf("canonical = %q, want c", got)
	}
}

func TestCleanupSessionStubsRepairsCircularAliases(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")
	insertTestAlias(t, ctx, store, "b", "a")

	dryRun, err := store.CleanupSessionStubsWithOptions(ctx, []string{"chan"}, SessionCleanupOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run cleanup: %v", err)
	}
	if len(dryRun.AliasCyclesRepaired) == 0 {
		t.Fatal("dry-run should report circular alias repair")
	}

	report, err := store.CleanupSessionStubs(ctx, []string{"chan"})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(report.AliasCyclesRepaired) == 0 {
		t.Fatal("cleanup should report circular alias repair")
	}
	canonA, err := store.ResolveCanonicalStreamID(ctx, "a")
	if err != nil {
		t.Fatalf("resolve a: %v", err)
	}
	canonB, err := store.ResolveCanonicalStreamID(ctx, "b")
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}
	if canonA != canonB {
		t.Fatalf("canon a=%q b=%q, want same", canonA, canonB)
	}
	if canonA != "b" {
		t.Fatalf("canonical = %q, want richer stream b", canonA)
	}
}

func TestLinkSessionAliasMergesCheckpointConflicts(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_sync_checkpoints (
			stream_id, video_id, cursor, offset_seconds, comments_fetched, segments_json, fetch_mode
		) VALUES
			('a', 'vod-1', 'cursor-a', 120, 50, '{"a":true}', 'parallel'),
			('b', 'vod-1', 'cursor-b', 30, 10, '{"b":true}', 'serial')`)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (
			stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples
		) VALUES ('a', now(), 10, 20, 20, 1)`)

	if err := store.linkSessionAlias(ctx, "a", "b"); err != nil {
		t.Fatalf("linkSessionAlias: %v", err)
	}

	var cursor, segments, mode string
	var offset, fetched int
	err := store.db.QueryRow(ctx, `
		SELECT cursor, offset_seconds, comments_fetched, segments_json, fetch_mode
		FROM analytics_sync_checkpoints
		WHERE stream_id='b' AND video_id='vod-1'`).Scan(&cursor, &offset, &fetched, &segments, &mode)
	if err != nil {
		t.Fatalf("load merged checkpoint: %v", err)
	}
	if cursor != "cursor-a" || offset != 120 || fetched != 50 || !strings.Contains(segments, "a") || mode != "parallel" {
		t.Fatalf("merged checkpoint = cursor=%q offset=%d fetched=%d segments=%q mode=%q", cursor, offset, fetched, segments, mode)
	}
	var losingRows int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_sync_checkpoints WHERE stream_id='a'`).Scan(&losingRows); err != nil {
		t.Fatalf("count losing checkpoints: %v", err)
	}
	if losingRows != 0 {
		t.Fatalf("losing checkpoint rows = %d, want 0", losingRows)
	}
}

func TestBulkPatchViewerRollupsWritesThroughAlias(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")

	err := store.BulkPatchViewerRollups(ctx, "a", []MinuteRollup{{
		MinuteTS:      time.Now().UTC().Truncate(time.Minute),
		ViewerAvg:     10,
		ViewerMax:     15,
		ViewerLatest:  15,
		ViewerSamples: 1,
	}})
	if err != nil {
		t.Fatalf("BulkPatchViewerRollups: %v", err)
	}
	var streamID string
	if err := store.db.QueryRow(ctx, `SELECT stream_id FROM analytics_minute_rollups`).Scan(&streamID); err != nil {
		t.Fatalf("load rollup stream id: %v", err)
	}
	if streamID != "b" {
		t.Fatalf("rollup stream_id = %q, want b", streamID)
	}
}

func TestBulkPatchViewerRollupsPreservesChatOnConflict(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "viewer-chat", "chan", 0)
	minute := time.Date(2026, 7, 4, 1, 52, 0, 0, time.UTC)
	if err := store.BulkUpsertMinuteRollups(ctx, "viewer-chat", []MinuteRollup{{
		MinuteTS:          minute,
		ChatCount:         445,
		TotalEmoteCount:   352,
		SevenTVEmoteCount: 352,
		Emotes:            map[string]int{"seventv:1:LO": 352},
	}}); err != nil {
		t.Fatalf("BulkUpsertMinuteRollups: %v", err)
	}
	if err := store.BulkPatchViewerRollups(ctx, "viewer-chat", []MinuteRollup{{
		MinuteTS:      minute,
		ViewerAvg:     18825,
		ViewerMax:     18825,
		ViewerLatest:  18825,
		ViewerSamples: 1,
	}}); err != nil {
		t.Fatalf("BulkPatchViewerRollups: %v", err)
	}
	var chatCount, emoteCount int
	if err := store.db.QueryRow(ctx, `
		SELECT chat_count, total_emote_count
		FROM analytics_minute_rollups
		WHERE stream_id='viewer-chat' AND minute_ts=$1`, minute).Scan(&chatCount, &emoteCount); err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if chatCount != 445 || emoteCount != 352 {
		t.Fatalf("rollup after viewer patch = chat:%d emote:%d, want 445/352", chatCount, emoteCount)
	}
}

func TestBulkPatchViewerRollupsInsertsViewerOnlyRow(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "viewer-only", "chan", 0)
	minute := time.Date(2026, 7, 4, 1, 52, 0, 0, time.UTC)
	if err := store.BulkPatchViewerRollups(ctx, "viewer-only", []MinuteRollup{{
		MinuteTS:      minute,
		ViewerAvg:     18825,
		ViewerMax:     18825,
		ViewerLatest:  18825,
		ViewerSamples: 1,
	}}); err != nil {
		t.Fatalf("BulkPatchViewerRollups: %v", err)
	}
	var chatCount int
	if err := store.db.QueryRow(ctx, `
		SELECT chat_count
		FROM analytics_minute_rollups
		WHERE stream_id='viewer-only' AND minute_ts=$1`, minute).Scan(&chatCount); err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if chatCount != 0 {
		t.Fatalf("viewer-only insert chat_count = %d, want 0 until chat upsert", chatCount)
	}
}

func TestBulkUpsertLiveMinuteRollupsInsertsViewerOnlyHeartbeat(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "live-heartbeat", "chan", 0)
	minute := time.Date(2026, 7, 4, 2, 10, 0, 0, time.UTC)
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "live-heartbeat", []MinuteRollup{{
		MinuteTS:      minute,
		ViewerAvg:     4200,
		ViewerMax:     4200,
		ViewerLatest:  4200,
		ViewerSamples: 1,
		ChatCount:     0,
	}}); err != nil {
		t.Fatalf("BulkUpsertLiveMinuteRollups: %v", err)
	}
	var chatCount, viewerSamples int
	if err := store.db.QueryRow(ctx, `
		SELECT chat_count, viewer_samples
		FROM analytics_minute_rollups
		WHERE stream_id='live-heartbeat' AND minute_ts=$1`, minute).Scan(&chatCount, &viewerSamples); err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if chatCount != 0 {
		t.Fatalf("viewer-only heartbeat chat_count = %d, want 0", chatCount)
	}
	if viewerSamples != 1 {
		t.Fatalf("viewer-only heartbeat viewer_samples = %d, want 1", viewerSamples)
	}
}

func TestBulkUpsertMinuteRollupsStampsGQLCanonical(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "bulk-gql", "chan", 0)
	minute := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	err := store.BulkUpsertMinuteRollups(ctx, "bulk-gql", []MinuteRollup{{
		MinuteTS:          minute,
		ChatCount:         42,
		TotalEmoteCount:   10,
		SevenTVEmoteCount: 3,
		Emotes:            map[string]int{"twitch:1:Kappa": 3},
	}})
	if err != nil {
		t.Fatalf("BulkUpsertMinuteRollups: %v", err)
	}
	var chatSource, confidence string
	if err := store.db.QueryRow(ctx, `
		SELECT chat_source, source_confidence
		FROM analytics_minute_rollups
		WHERE stream_id='bulk-gql' AND minute_ts=$1`, minute).Scan(&chatSource, &confidence); err != nil {
		t.Fatalf("load rollup source: %v", err)
	}
	if chatSource != RollupChatSourceGQL || confidence != SourceConfidenceCanonical {
		t.Fatalf("source = %q/%q, want gql/canonical", chatSource, confidence)
	}
}

func TestTopHistoricalChatMinutesUsesMinutePeaks(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "peak-stream", "chan", 0)
	minute := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	if err := store.BulkUpsertMinuteRollups(ctx, "peak-stream", []MinuteRollup{{
		MinuteTS:          minute,
		ChatCount:         99,
		TotalEmoteCount:   12,
		SevenTVEmoteCount: 5,
		Emotes:            map[string]int{"7tv:wave:Wave": 7},
	}}); err != nil {
		t.Fatalf("BulkUpsertMinuteRollups: %v", err)
	}
	mustExec(t, ctx, store, `DELETE FROM analytics_minute_rollups WHERE stream_id=$1`, "peak-stream")

	candidates, err := store.TopHistoricalChatMinutesInWindow(ctx, minute.Add(-time.Minute), minute.Add(time.Minute), 5)
	if err != nil {
		t.Fatalf("TopHistoricalChatMinutesInWindow: %v", err)
	}
	if len(candidates) != 1 || candidates[0].StreamID != "peak-stream" || candidates[0].ChatCount != 99 {
		t.Fatalf("candidates = %+v", candidates)
	}
	if candidates[0].Emotes["7tv:wave:Wave"] != 7 {
		t.Fatalf("emotes = %+v", candidates[0].Emotes)
	}
}

func TestAliasReadPathsResolveCanonicalStream(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")
	fixedUpdatedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `UPDATE analytics_streams SET updated_at=$2 WHERE stream_id=$1`, "b", fixedUpdatedAt)

	if err := store.UpsertMinuteRollup(ctx, "a", MinuteRollup{
		MinuteTS:      fixedUpdatedAt.Truncate(time.Minute),
		ViewerAvg:     42,
		ViewerMax:     50,
		ViewerLatest:  48,
		ViewerSamples: 2,
	}); err != nil {
		t.Fatalf("UpsertMinuteRollup through alias: %v", err)
	}
	rollups, err := store.RollupsByStream(ctx, "a")
	if err != nil {
		t.Fatalf("RollupsByStream alias: %v", err)
	}
	if len(rollups) != 1 || rollups[0].ViewerAvg != 42 {
		t.Fatalf("rollups = %+v, want one canonical rollup", rollups)
	}

	if err := store.SaveGameSegments(ctx, "a", []GameSegment{{
		GameName:        "Counter-Strike 2",
		BoxArtURL:       "https://example.test/box.jpg",
		OffsetSeconds:   30,
		DurationSeconds: 90,
	}}); err != nil {
		t.Fatalf("SaveGameSegments alias: %v", err)
	}
	segments, err := store.GetGameSegments(ctx, "a")
	if err != nil {
		t.Fatalf("GetGameSegments alias: %v", err)
	}
	if len(segments) != 1 || segments[0].StreamID != "b" || segments[0].DurationSeconds != 90 {
		t.Fatalf("segments = %+v, want canonical stream b", segments)
	}

	updatedAt, err := store.GetStreamUpdatedAt(ctx, "a")
	if err != nil {
		t.Fatalf("GetStreamUpdatedAt alias: %v", err)
	}
	if !updatedAt.Equal(fixedUpdatedAt) {
		t.Fatalf("updated_at = %s, want %s", updatedAt, fixedUpdatedAt)
	}

	if err := store.UpsertSyncCheckpoint(ctx, SyncCheckpoint{
		StreamID:        "a",
		VideoID:         "vod-1",
		Cursor:          "cursor-b",
		OffsetSeconds:   120,
		CommentsFetched: 12,
	}); err != nil {
		t.Fatalf("UpsertSyncCheckpoint alias: %v", err)
	}
	cp, err := store.GetSyncCheckpoint(ctx, "a", "vod-1")
	if err != nil {
		t.Fatalf("GetSyncCheckpoint alias: %v", err)
	}
	if cp == nil || cp.StreamID != "b" || cp.Cursor != "cursor-b" {
		t.Fatalf("checkpoint = %+v, want canonical stream b", cp)
	}
	if err := store.DeleteSyncCheckpoint(ctx, "a", "vod-1"); err != nil {
		t.Fatalf("DeleteSyncCheckpoint alias: %v", err)
	}
	cp, err = store.GetSyncCheckpoint(ctx, "b", "vod-1")
	if err != nil {
		t.Fatalf("GetSyncCheckpoint after delete: %v", err)
	}
	if cp != nil {
		t.Fatalf("checkpoint after delete = %+v, want nil", cp)
	}
}

func TestCleanupSessionStubsReportsAndAppliesBackfillJobRekeys(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status)
		VALUES
			('silver', 'a', 'chan', 'failed', 'failed'),
			('gold', 'b', 'chan', 'queued', 'pending'),
			('gold', 'a', 'chan', 'running', 'pending')`)

	dryRun, err := store.CleanupSessionStubsWithOptions(ctx, []string{"chan"}, SessionCleanupOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run cleanup: %v", err)
	}
	if len(dryRun.BackfillJobsRekeyed) != 2 {
		t.Fatalf("dry-run job rekeys = %v, want two alias jobs", dryRun.BackfillJobsRekeyed)
	}

	report, err := store.CleanupSessionStubs(ctx, []string{"chan"})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(report.BackfillJobsRekeyed) != 2 {
		t.Fatalf("job rekeys = %v, want two alias jobs", report.BackfillJobsRekeyed)
	}

	var failedStreamID string
	if err := store.db.QueryRow(ctx, `
		SELECT stream_id FROM backfill_jobs
		WHERE tier='silver' AND status='failed'`).Scan(&failedStreamID); err != nil {
		t.Fatalf("load failed job: %v", err)
	}
	if failedStreamID != "b" {
		t.Fatalf("failed job stream_id = %q, want b", failedStreamID)
	}

	var aliasGoldStatus, aliasGoldExportStatus string
	if err := store.db.QueryRow(ctx, `
		SELECT status, export_status FROM backfill_jobs
		WHERE tier='gold' AND stream_id='a'`).Scan(&aliasGoldStatus, &aliasGoldExportStatus); err != nil {
		t.Fatalf("load duplicate alias gold job: %v", err)
	}
	if aliasGoldStatus != "skipped" || aliasGoldExportStatus != "skipped" {
		t.Fatalf("duplicate alias gold status/export = %s/%s, want skipped/skipped", aliasGoldStatus, aliasGoldExportStatus)
	}
}

func TestResolveSessionCleanupLoginsIncludesBackfillJobs(t *testing.T) {
	ctx, store := setupSessionStore(t)
	mustExec(t, ctx, store, `INSERT INTO analytics_always_tracked (login) VALUES ('always')`)
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status)
		VALUES ('silver', 's1', 'corpuschan', 'done', 'confirmed')`)

	logins, err := store.ResolveSessionCleanupLogins(ctx, nil, []string{"envchan"})
	if err != nil {
		t.Fatalf("ResolveSessionCleanupLogins: %v", err)
	}
	want := []string{"always", "corpuschan", "envchan"}
	if strings.Join(logins, ",") != strings.Join(want, ",") {
		t.Fatalf("logins = %v, want %v", logins, want)
	}
}

func TestCanonicalizeClaimedBackfillJobSkipsDuplicateActiveCanonical(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (id, tier, stream_id, login, status, export_status)
		VALUES
			(1, 'gold', 'b', 'chan', 'queued', 'pending'),
			(2, 'gold', 'a', 'chan', 'running', 'pending')`)

	worker := &BackfillWorker{db: store.db}
	job := &BackfillJob{ID: 2, Tier: "gold", StreamID: "a", Login: "chan", Status: "running", ExportStatus: "pending"}
	_, skipped, err := worker.canonicalizeClaimedBackfillJob(ctx, job)
	if err != nil {
		t.Fatalf("canonicalizeClaimedBackfillJob: %v", err)
	}
	if !skipped {
		t.Fatal("expected duplicate active canonical job to be skipped")
	}

	var status, exportStatus string
	if err := store.db.QueryRow(ctx, `SELECT status, export_status FROM backfill_jobs WHERE id=2`).Scan(&status, &exportStatus); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "skipped" || exportStatus != "skipped" {
		t.Fatalf("status/export = %s/%s, want skipped/skipped", status, exportStatus)
	}
}

func TestGoldEnqueuerCanonicalizesAliasSilverJobs(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "a", "chan", 0)
	insertTestStream(t, ctx, store, "b", "chan", 10)
	insertTestAlias(t, ctx, store, "a", "b")
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, export_status)
		VALUES ('silver', 'a', 'chan', 'done', 'confirmed')`)

	enqueuer := NewGoldEnqueuer(store.db, NewGoldRulesEngine([]string{"chan"}, 0, 0), time.Minute)
	enqueued, err := enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1", enqueued)
	}

	var goldStreamID string
	if err := store.db.QueryRow(ctx, `
		SELECT stream_id FROM backfill_jobs WHERE tier='gold'`).Scan(&goldStreamID); err != nil {
		t.Fatalf("load gold job: %v", err)
	}
	if goldStreamID != "b" {
		t.Fatalf("gold stream_id = %q, want canonical b", goldStreamID)
	}

	enqueued, err = enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if enqueued != 0 {
		t.Fatalf("second enqueued = %d, want 0", enqueued)
	}
}

func TestPersistEarlyViewerChartBootstrapsBeforeRollupWrite(t *testing.T) {
	ctx, store := setupSessionStore(t)
	service := &SyncService{
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	service.setSyncChannel("new-stream", "chan")

	service.persistEarlyViewerChart(
		ctx,
		"new-stream",
		time.Now().UTC().Truncate(time.Minute),
		60,
		20,
		15,
		[]parsedViewerPoint{{OffsetSeconds: 0, Viewers: 20}},
		nil,
	)

	var streamRows, rollupRows int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_streams WHERE stream_id='new-stream'`).Scan(&streamRows); err != nil {
		t.Fatalf("count stream rows: %v", err)
	}
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_minute_rollups WHERE stream_id='new-stream'`).Scan(&rollupRows); err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if streamRows != 1 || rollupRows != 1 {
		t.Fatalf("stream rows=%d rollups=%d, want 1/1", streamRows, rollupRows)
	}
}

func setupSessionStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run analytics session integration tests")
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
	schema := fmt.Sprintf("analytics_session_test_%d", time.Now().UnixNano())
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
	createSessionTestSchema(t, ctx, store)
	return ctx, store
}

func createSessionTestSchema(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	mustExec(t, ctx, store, `
		CREATE TABLE analytics_streams (
			stream_id TEXT PRIMARY KEY,
			canonical_stream_id TEXT,
			broadcaster_id TEXT NOT NULL DEFAULT 'pending',
			login TEXT NOT NULL,
			display_name TEXT,
			profile_image_url TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			vod_id TEXT NOT NULL DEFAULT '',
			vod_source TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMPTZ NOT NULL,
			ended_at TIMESTAMPTZ,
			avg_viewers INT NOT NULL DEFAULT 0,
			current_viewers INT NOT NULL DEFAULT 0,
			viewer_source TEXT NOT NULL DEFAULT 'unknown',
			viewer_samples INT NOT NULL DEFAULT 0,
			chat_messages BIGINT NOT NULL DEFAULT 0,
			total_emote_uses BIGINT NOT NULL DEFAULT 0,
			seventv_emote_uses BIGINT NOT NULL DEFAULT 0,
			peak_viewers INT NOT NULL DEFAULT 0,
			title TEXT,
			category TEXT,
			language TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE analytics_stream_sessions (
			canonical_stream_id TEXT PRIMARY KEY,
			login TEXT NOT NULL,
			twitch_stream_id TEXT NOT NULL DEFAULT '',
			tt_stream_id TEXT NOT NULL DEFAULT '',
			vod_id TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMPTZ NOT NULL,
			ended_at TIMESTAMPTZ,
			title TEXT,
			category TEXT,
			viewer_source TEXT NOT NULL DEFAULT 'unknown',
			source_confidence TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE analytics_stream_aliases (
			alias_stream_id TEXT PRIMARY KEY,
			canonical_stream_id TEXT NOT NULL REFERENCES analytics_stream_sessions(canonical_stream_id) ON DELETE CASCADE,
			alias_kind TEXT NOT NULL DEFAULT 'unknown',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE analytics_minute_rollups (
			stream_id TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
			minute_ts TIMESTAMPTZ NOT NULL,
			viewer_avg INT NOT NULL DEFAULT 0,
			viewer_max INT NOT NULL DEFAULT 0,
			viewer_latest INT NOT NULL DEFAULT 0,
			viewer_samples INT NOT NULL DEFAULT 0,
			chat_count INT NOT NULL DEFAULT 0,
			total_emote_count INT NOT NULL DEFAULT 0,
			seventv_emote_count INT NOT NULL DEFAULT 0,
			emotes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			chat_source TEXT NOT NULL DEFAULT '',
			source_confidence TEXT NOT NULL DEFAULT '',
			chat_source_detail TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (stream_id, minute_ts)
		);
		CREATE TABLE analytics_minute_peaks (
			stream_id TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
			minute_ts TIMESTAMPTZ NOT NULL,
			chat_count INT NOT NULL DEFAULT 0,
			total_emote_count INT NOT NULL DEFAULT 0,
			seventv_emote_count INT NOT NULL DEFAULT 0,
			emotes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			chat_source TEXT NOT NULL DEFAULT '',
			source_confidence TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (stream_id, minute_ts)
		);
		CREATE TABLE stream_game_segments (
			id BIGSERIAL PRIMARY KEY,
			stream_id TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
			game_name TEXT NOT NULL DEFAULT '',
			box_art_url TEXT NOT NULL DEFAULT '',
			offset_seconds INT NOT NULL,
			duration_seconds INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE analytics_sync_checkpoints (
			stream_id TEXT NOT NULL,
			video_id TEXT NOT NULL,
			cursor TEXT NOT NULL DEFAULT '',
			offset_seconds INT NOT NULL DEFAULT 0,
			comments_fetched INT NOT NULL DEFAULT 0,
			segments_json TEXT NOT NULL DEFAULT '',
			fetch_mode TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (stream_id, video_id)
		);
		CREATE TABLE backfill_jobs (
			id BIGSERIAL PRIMARY KEY,
			tier TEXT NOT NULL DEFAULT 'silver',
			stream_id TEXT NOT NULL,
			login TEXT NOT NULL,
			egress_slot INT NOT NULL DEFAULT 0,
			attempt INT NOT NULL DEFAULT 0,
			export_status TEXT NOT NULL DEFAULT 'pending',
			status TEXT NOT NULL DEFAULT 'queued',
			next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX idx_backfill_jobs_active_stream
			ON backfill_jobs (stream_id)
			WHERE status IN ('queued', 'running');
		CREATE TABLE analytics_always_tracked (
			login TEXT PRIMARY KEY
		)`)
}

func insertTestStream(t *testing.T, ctx context.Context, store *Store, streamID, login string, score int) {
	t.Helper()
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, started_at,
			viewer_samples, chat_messages, peak_viewers, title
		)
		VALUES ($1, $1, $2, $3, now(), $4, $5, $6, $7)`,
		streamID, fmt.Sprintf("bc-%s", streamID), login, score, score, score, fmt.Sprintf("title-%s", streamID))
	mustExec(t, ctx, store, `
		INSERT INTO analytics_stream_sessions (
			canonical_stream_id, login, twitch_stream_id, started_at, title
		)
		VALUES ($1, $2, $1, now(), $3)`,
		streamID, login, fmt.Sprintf("title-%s", streamID))
}

func insertTestAlias(t *testing.T, ctx context.Context, store *Store, aliasID, canonicalID string) {
	t.Helper()
	mustExec(t, ctx, store, `
		INSERT INTO analytics_stream_aliases (alias_stream_id, canonical_stream_id, alias_kind)
		VALUES ($1, $2, 'test')
		ON CONFLICT (alias_stream_id) DO UPDATE SET canonical_stream_id = EXCLUDED.canonical_stream_id`,
		aliasID, canonicalID)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET canonical_stream_id = $2
		WHERE stream_id = $1`, aliasID, canonicalID)
}

func mustExec(t *testing.T, ctx context.Context, store *Store, sql string, args ...any) {
	t.Helper()
	if _, err := store.db.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec failed: %v\nSQL: %s", err, sql)
	}
}

func TestUpsertLiveStreamStubBeforeSessionAliasLink(t *testing.T) {
	ctx, store := setupSessionStore(t)
	startedAt := time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC)
	insertTestStream(t, ctx, store, "canonical-old", "xqc", 5)
	mustExec(t, ctx, store, `UPDATE analytics_streams SET started_at=$2 WHERE stream_id=$1`, "canonical-old", startedAt)

	err := store.UpsertLiveStream(ctx, LiveStream{
		ID:            "319944688345",
		Login:         "xqc",
		BroadcasterID: "12345",
		Title:         "Live now",
		GameName:      "Just Chatting",
		StartedAt:     startedAt,
		ViewerCount:   50000,
	}, UserProfile{ID: "12345", DisplayName: "xQc"}, startedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("UpsertLiveStream incoming helix id before canonical row existed: %v", err)
	}

	err = store.UpsertMinuteRollup(ctx, "319944688345", MinuteRollup{
		MinuteTS:  startedAt.Truncate(time.Minute),
		ChatCount: 10,
	})
	if err != nil {
		t.Fatalf("UpsertMinuteRollup on helix stream id after live upsert: %v", err)
	}

	var rollupStreamID string
	if err := store.db.QueryRow(ctx, `SELECT stream_id FROM analytics_minute_rollups LIMIT 1`).Scan(&rollupStreamID); err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if rollupStreamID == "" {
		t.Fatal("expected rollup row")
	}
}
