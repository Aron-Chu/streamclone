package analytics

import (
	"testing"
	"time"
)

func TestCloseStaleOpenStreams(t *testing.T) {
	ctx, store := setupSessionStore(t)
	streamID := "stale-open-1"
	insertTestStream(t, ctx, store, streamID, "jynxzi", 0)

	started := time.Now().UTC().Add(-72 * time.Hour)
	lastSeen := started.Add(2 * time.Hour)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET started_at=$2, last_seen_at=$3, ended_at=NULL, viewer_samples=0, chat_messages=0
		WHERE stream_id=$1`, streamID, started, lastSeen)

	report, err := store.CloseStaleOpenStreams(ctx, 48*time.Hour)
	if err != nil {
		t.Fatalf("CloseStaleOpenStreams: %v", err)
	}
	if report.Closed != 1 {
		t.Fatalf("closed=%d want 1 (%+v)", report.Closed, report)
	}

	var endedAt *time.Time
	if err := store.db.QueryRow(ctx, `SELECT ended_at FROM analytics_streams WHERE stream_id=$1`, streamID).Scan(&endedAt); err != nil {
		t.Fatalf("load ended_at: %v", err)
	}
	if endedAt == nil {
		t.Fatal("expected ended_at to be set")
	}
}

func TestRefreshSummariesForRollupStreams(t *testing.T) {
	ctx, store := setupSessionStore(t)
	streamID := "summary-repair-1"
	insertTestStream(t, ctx, store, streamID, "jynxzi", 0)

	started := time.Now().UTC().Add(-6 * time.Hour)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET started_at=$2, viewer_samples=0, chat_messages=0
		WHERE stream_id=$1`, streamID, started)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (
			stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples, chat_count
		) VALUES ($1, $2, 1200, 1300, 1250, 1, 42)
		ON CONFLICT (stream_id, minute_ts) DO UPDATE SET chat_count=EXCLUDED.chat_count`,
		streamID, started.Add(30*time.Minute))

	report, err := store.RefreshSummariesForRollupStreams(ctx, 10)
	if err != nil {
		t.Fatalf("RefreshSummariesForRollupStreams: %v", err)
	}
	if report.Refreshed != 1 {
		t.Fatalf("refreshed=%d want 1", report.Refreshed)
	}

	var chatMessages int64
	if err := store.db.QueryRow(ctx, `SELECT chat_messages FROM analytics_streams WHERE stream_id=$1`, streamID).Scan(&chatMessages); err != nil {
		t.Fatalf("load chat_messages: %v", err)
	}
	if chatMessages != 42 {
		t.Fatalf("chat_messages=%d want 42", chatMessages)
	}
}
