package ingest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/storygraph/store"
)

func setupOriginIntegrationStore(t *testing.T) (context.Context, *pgxpool.Pool, *store.Store) {
	t.Helper()
	dsn := os.Getenv("STORYGRAPH_STORE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set STORYGRAPH_STORE_TEST_DATABASE_URL to run seeded origin integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	dir := filepath.Join("..", "..", "..", "migrations")
	for _, name := range []string{
		"000005_analytics.up.sql",
		"000007_analytics_vod_id.up.sql",
		"000015_story_graph_core.up.sql",
		"000016_story_graph_social.up.sql",
		"000018_evidence_previews.up.sql",
		"000019_pulse_directory_stats.up.sql",
		"000020_story_window_scores.up.sql",
		"000023_story_origin_contract.up.sql",
		"000024_story_operator_actions.up.sql",
		"000025_story_class.up.sql",
		"000026_story_watch_entries.up.sql",
		"000027_story_origin_search_status.up.sql",
		"000028_story_source_reliability_extensions.up.sql",
		"000029_social_item_metric_snapshots.up.sql",
	} {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return ctx, pool, store.New(pool)
}

func TestAttachOriginsForSeededAnalyticsMoment(t *testing.T) {
	ctx, pool, st := setupOriginIntegrationStore(t)
	started := time.Now().UTC().Add(-90 * time.Minute)
	lastSeen := started.Add(2 * time.Hour)
	entityID, err := st.UpsertEntity(ctx, "caseoh", "caseoh-id", "CaseOh", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO analytics_streams (stream_id, broadcaster_id, login, display_name, started_at, last_seen_at, vod_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"stream-origin-1", "caseoh-id", "caseoh", "CaseOh", started, lastSeen, "2371095470"); err != nil {
		t.Fatalf("seed analytics stream: %v", err)
	}
	for _, row := range []struct {
		offsetS int
		chat    int
		emotes  int
		sevenTV int
		viewers int
	}{
		{240, 8, 4, 2, 1200},
		{300, 12, 5, 3, 1250},
		{360, 26, 18, 12, 1500},
		{420, 81, 66, 44, 2100},
		{480, 34, 20, 11, 1700},
		{540, 18, 9, 4, 1380},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO analytics_minute_rollups
				(stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				 chat_count, total_emote_count, seventv_emote_count, emotes_json)
			VALUES ($1, $2, $3, $3, $3, 1, $4, $5, $6, '{}'::jsonb)`,
			"stream-origin-1", started.Add(time.Duration(row.offsetS)*time.Second), row.viewers, row.chat, row.emotes, row.sevenTV); err != nil {
			t.Fatalf("seed rollup %d: %v", row.offsetS, err)
		}
	}

	quotes, _ := json.Marshal([]string{"streamer explains the contract leak on stream"})
	topEmotes, _ := json.Marshal([]map[string]any{{"name": "OMEGALUL", "count": 42}})
	originConfidence := 0.91
	fpID, err := st.InsertFingerprint(ctx, store.MomentFingerprint{
		EntityID:         &entityID,
		StreamID:         "stream-origin-1",
		VODID:            "2371095470",
		VODOffsetS:       420,
		TranscriptKW:     quotes,
		TopEmotes:        topEmotes,
		ChatSpikeSummary: "Chat jumped near the contract leak quote.",
		OriginConfidence: &originConfidence,
	})
	if err != nil {
		t.Fatalf("InsertFingerprint: %v", err)
	}
	clusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "Streamer explains the contract leak on stream",
		Category: "drama",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	occurredAt := started.Add(45 * time.Minute)
	if _, err := st.InsertEvidence(ctx, store.Evidence{
		ClusterID:  clusterID,
		SourceType: "reddit_thread",
		SourceURL:  "https://reddit.com/r/LivestreamFail/seeded-origin",
		MatchConf:  0.8,
		Weight:     0.7,
		OccurredAt: &occurredAt,
	}); err != nil {
		t.Fatalf("InsertEvidence: %v", err)
	}

	streams, err := st.ListOriginCandidateStreams(ctx, []string{"caseoh"}, started.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListOriginCandidateStreams: %v", err)
	}
	if len(streams) != 1 || streams[0].VODID != "2371095470" {
		t.Fatalf("candidate streams = %+v", streams)
	}
	workers := &Workers{opts: Options{
		Store:  st,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	if err := workers.attachOriginsForStream(ctx, streams[0]); err != nil {
		t.Fatalf("attachOriginsForStream: %v", err)
	}

	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil || card.Origin == nil {
		t.Fatalf("story origin missing after attach: %+v", card)
	}
	if card.Origin.ID != fpID || card.Origin.VODID != "2371095470" || card.Origin.VODOffsetS != 420 {
		t.Fatalf("attached origin = %+v, fpID=%d", card.Origin, fpID)
	}
	if !strings.Contains(string(card.Origin.TopEmotes), "OMEGALUL") {
		t.Fatalf("top emotes not preserved: %s", string(card.Origin.TopEmotes))
	}
	if len(card.Origin.OriginSpikePoints) == 0 {
		t.Fatalf("origin spike points missing: %+v", card.Origin)
	}
	foundOriginPoint := false
	for _, point := range card.Origin.OriginSpikePoints {
		if point.RelativeS == 0 && point.ChatCount == 81 && point.TotalEmoteCount == 66 && point.ViewerMax == 2100 {
			foundOriginPoint = true
			break
		}
	}
	if !foundOriginPoint {
		t.Fatalf("origin point near timestamp missing: %+v", card.Origin.OriginSpikePoints)
	}
	foundPulseReceipt := false
	for _, receipt := range card.Receipts {
		if receipt.SourceType == "pulse_origin" {
			foundPulseReceipt = true
			break
		}
	}
	if !foundPulseReceipt {
		t.Fatalf("pulse origin receipt missing: %+v", card.Receipts)
	}
}
