package analytics

import (
	"testing"
	"time"
)

func TestBulkUpsertLiveMinuteRollupsOpenMinuteSkipsSummaryRefresh(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "open-skip", "chan", 0)
	minute := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	rollup := MinuteRollup{
		MinuteTS:      minute,
		ChatCount:     12,
		ViewerAvg:     100,
		ViewerMax:     100,
		ViewerLatest:  100,
		ViewerSamples: 1,
		Emotes:        map[string]int{},
	}
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "open-skip", []MinuteRollup{rollup}, LiveRollupWriteOptions{Mode: LiveRollupWriteOpenMinute}); err != nil {
		t.Fatalf("open minute write: %v", err)
	}
	var chatMessages int64
	if err := store.db.QueryRow(ctx, `SELECT chat_messages FROM analytics_streams WHERE stream_id=$1`, "open-skip").Scan(&chatMessages); err != nil {
		t.Fatalf("load chat_messages: %v", err)
	}
	if chatMessages != 0 {
		t.Fatalf("chat_messages = %d, want 0 without summary refresh", chatMessages)
	}
	var rollupChat int
	if err := store.db.QueryRow(ctx, `SELECT chat_count FROM analytics_minute_rollups WHERE stream_id=$1`, "open-skip").Scan(&rollupChat); err != nil {
		t.Fatalf("load rollup chat_count: %v", err)
	}
	if rollupChat != 12 {
		t.Fatalf("rollup chat_count = %d, want 12", rollupChat)
	}
}

func TestBulkUpsertLiveMinuteRollupsCompletedMinuteRefreshesSummary(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "completed-refresh", "chan", 0)
	minute := time.Date(2026, 7, 5, 12, 1, 0, 0, time.UTC)
	rollup := MinuteRollup{
		MinuteTS:          minute,
		ChatCount:         42,
		TotalEmoteCount:   10,
		SevenTVEmoteCount: 4,
		ViewerAvg:         200,
		ViewerMax:         200,
		ViewerLatest:      200,
		ViewerSamples:     1,
		Emotes:            map[string]int{},
	}
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "completed-refresh", []MinuteRollup{rollup}, LiveRollupWriteOptions{Mode: LiveRollupWriteCompletedMinute}); err != nil {
		t.Fatalf("completed minute write: %v", err)
	}
	var chatMessages int64
	if err := store.db.QueryRow(ctx, `SELECT chat_messages FROM analytics_streams WHERE stream_id=$1`, "completed-refresh").Scan(&chatMessages); err != nil {
		t.Fatalf("load chat_messages: %v", err)
	}
	if chatMessages != 42 {
		t.Fatalf("chat_messages = %d, want 42", chatMessages)
	}
}

func TestLatestRollupChatSignalsByStreamIDs(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "sig-a", "chan", 0)
	insertTestStream(t, ctx, store, "sig-b", "chan", 1)
	minute := time.Date(2026, 7, 5, 12, 2, 0, 0, time.UTC)
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "sig-a", []MinuteRollup{{
		MinuteTS: minute, ChatCount: 5, Emotes: map[string]int{},
	}}, LiveRollupWriteOptions{Mode: LiveRollupWriteCompletedMinute}); err != nil {
		t.Fatalf("write sig-a: %v", err)
	}
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "sig-b", []MinuteRollup{{
		MinuteTS: minute.Add(time.Minute), ChatCount: 9, TotalEmoteCount: 3, Emotes: map[string]int{},
	}}, LiveRollupWriteOptions{Mode: LiveRollupWriteCompletedMinute}); err != nil {
		t.Fatalf("write sig-b: %v", err)
	}
	got, err := store.LatestRollupChatSignalsByStreamIDs(ctx, []string{"sig-a", "sig-b", "missing"})
	if err != nil {
		t.Fatalf("LatestRollupChatSignalsByStreamIDs: %v", err)
	}
	if got["sig-a"].ChatCount != 5 {
		t.Fatalf("sig-a chat = %d, want 5", got["sig-a"].ChatCount)
	}
	if got["sig-b"].ChatCount != 9 || got["sig-b"].TotalEmoteCount != 3 {
		t.Fatalf("sig-b signal = %+v, want chat=9 emotes=3", got["sig-b"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("missing stream should not appear in signal map")
	}
}

func TestLatestRollupChatSignalsIgnoresViewerOnlyLatestRow(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "sig-viewer", "chan", 0)
	chatMinute := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	viewerMinute := time.Now().UTC().Truncate(time.Minute)
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "sig-viewer", []MinuteRollup{{
		MinuteTS: chatMinute, ChatCount: 17, TotalEmoteCount: 4, Emotes: map[string]int{},
	}}, LiveRollupWriteOptions{Mode: LiveRollupWriteCompletedMinute}); err != nil {
		t.Fatalf("write chat minute: %v", err)
	}
	if err := store.BulkUpsertLiveMinuteRollups(ctx, "sig-viewer", []MinuteRollup{{
		MinuteTS: viewerMinute, ViewerAvg: 100, ViewerMax: 100, ViewerLatest: 100, ViewerSamples: 1, Emotes: map[string]int{},
	}}, LiveRollupWriteOptions{Mode: LiveRollupWriteOpenMinute}); err != nil {
		t.Fatalf("write viewer minute: %v", err)
	}
	got, err := store.LatestRollupChatSignalsByStreamIDs(ctx, []string{"sig-viewer"})
	if err != nil {
		t.Fatalf("LatestRollupChatSignalsByStreamIDs: %v", err)
	}
	signal := got["sig-viewer"]
	if signal.ChatCount != 17 || signal.TotalEmoteCount != 4 {
		t.Fatalf("signal = %+v, want chat=17 emotes=4 from earlier chat row", signal)
	}
}
