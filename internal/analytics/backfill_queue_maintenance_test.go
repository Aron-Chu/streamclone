package analytics

import (
	"testing"
	"time"
)

func TestIsRetriableGoldFailure(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{"failed to save minute rollups to DB: violates foreign key constraint", true},
		{"ensure session before gold sync: canonical stream row missing from analytics_streams", true},
		{"failed to save chat rollups to DB: context deadline exceeded", true},
		{"twitch gql service unavailable: 503", true},
		{"archive upload failed: bad gateway", true},
		{"vod chat export: too many requests", true},
		{"archive export: no vod chat rows for stream 123", false},
		{"gold chat sync: no VOD chat comments fetched (vod_id=abc)", false},
		{"pulse backfill failed: no_chat_data_in_range", false},
		{"Viewer timeline synced; VOD was not found", false},
		{"some random scrape error", false},
	}
	for _, tc := range cases {
		if got := isRetriableGoldFailure(tc.err); got != tc.want {
			t.Fatalf("isRetriableGoldFailure(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsRetriableSilverFailure(t *testing.T) {
	if !isRetriableSilverFailure("scrape backoff active for 15m") {
		t.Fatal("expected scrape backoff to be retriable")
	}
	if isRetriableSilverFailure("foreign key violation") {
		t.Fatal("expected FK to be non-retriable for silver maintenance")
	}
}

func TestRequeueRetriableFailedJobsRespectsActiveStreamGuard(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "stream-active", "xqc", 10)
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, attempt, error, updated_at)
		VALUES ('gold', 'stream-active', 'xqc', 'failed', 1, 'context deadline exceeded', now() - interval '1 hour')`)
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status)
		VALUES ('silver', 'stream-active', 'xqc', 'queued')`)

	requeued, err := requeueRetriableFailedJobs(ctx, store.db, []string{"gold", "gold_full"}, 10, isRetriableGoldFailure)
	if err != nil {
		t.Fatalf("requeue failed jobs: %v", err)
	}
	if requeued != 0 {
		t.Fatalf("requeued = %d, want 0 while another active job exists", requeued)
	}

	mustExec(t, ctx, store, `DELETE FROM backfill_jobs WHERE tier='silver' AND stream_id='stream-active'`)
	requeued, err = requeueRetriableFailedJobs(ctx, store.db, []string{"gold", "gold_full"}, 10, isRetriableGoldFailure)
	if err != nil {
		t.Fatalf("requeue failed jobs without active guard: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("requeued = %d, want 1 after active job clears", requeued)
	}
}

func TestRequeueRetriableFailedJobsAttemptBoundary(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "stream-final", "xqc", 10)
	insertTestStream(t, ctx, store, "stream-retry", "pokimane", 10)
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, attempt, error, updated_at)
		VALUES
			('gold', 'stream-final', 'xqc', 'failed', $1, 'bad gateway', now() - interval '2 hours'),
			('gold', 'stream-retry', 'pokimane', 'failed', $2, 'bad gateway', now() - interval '2 hours')`,
		maxBackfillSyncAttempts, maxBackfillSyncAttempts-1)

	requeued, err := requeueRetriableFailedJobs(ctx, store.db, []string{"gold", "gold_full"}, 10, isRetriableGoldFailure)
	if err != nil {
		t.Fatalf("requeue failed jobs: %v", err)
	}
	if requeued != 1 {
		t.Fatalf("requeued = %d, want 1 below attempt boundary", requeued)
	}
	var finalStatus, retryStatus string
	if err := store.db.QueryRow(ctx, `SELECT status FROM backfill_jobs WHERE stream_id='stream-final'`).Scan(&finalStatus); err != nil {
		t.Fatalf("load final job: %v", err)
	}
	if err := store.db.QueryRow(ctx, `SELECT status FROM backfill_jobs WHERE stream_id='stream-retry'`).Scan(&retryStatus); err != nil {
		t.Fatalf("load retry job: %v", err)
	}
	if finalStatus != "failed" || retryStatus != "queued" {
		t.Fatalf("statuses final=%q retry=%q, want failed/queued", finalStatus, retryStatus)
	}
}

func TestRunBackfillQueueMaintenanceRepairsSessionThenRequeuesGold(t *testing.T) {
	ctx, store := setupSessionStore(t)
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, started_at, title
		)
		VALUES ('stream-repair', 'stream-repair', 'bc-repair', 'xqc', $1, 'needs session')`, startedAt)
	mustExec(t, ctx, store, `
		INSERT INTO backfill_jobs (tier, stream_id, login, status, attempt, error, updated_at)
		VALUES ('gold', 'stream-repair', 'xqc', 'failed', 1, 'ensure session before gold sync: missing from analytics_streams', now() - interval '1 hour')`)

	report, err := RunBackfillQueueMaintenance(ctx, store.db, BackfillQueueMaintenanceOptions{
		RequeueFailedMax:  10,
		RepairSessionsMax: 10,
	})
	if err != nil {
		t.Fatalf("maintenance: %v", err)
	}
	if report.SessionsRepaired != 1 || report.GoldRequeued != 1 {
		t.Fatalf("report = %+v, want one session repair and gold requeue", report)
	}
	var sessionCount int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_stream_sessions WHERE canonical_stream_id='stream-repair'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("session count = %d, want 1", sessionCount)
	}
}
