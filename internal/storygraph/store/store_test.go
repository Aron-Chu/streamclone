package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupPreviewStoreTest(t *testing.T) (context.Context, *Store) {
	t.Helper()
	dsn := os.Getenv("STORYGRAPH_STORE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set STORYGRAPH_STORE_TEST_DATABASE_URL to run storygraph store preview tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pg ping: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	applyStoryGraphMigrations(t, ctx, pool)
	return ctx, New(pool)
}

func applyStoryGraphMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "migrations")
	for _, name := range []string{
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
}

func TestMarkOriginSearchedRecordsMissingOriginState(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		Title:    "Origin missing story",
		Category: "drama",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	if err := st.MarkOriginSearched(ctx, clusterID, "searched_missing"); err != nil {
		t.Fatalf("MarkOriginSearched: %v", err)
	}
	card, err := st.GetStory(ctx, clusterID, "")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil || card.Cluster.OriginSearchStatus != "searched_missing" || card.Cluster.OriginCheckedAt == nil {
		t.Fatalf("origin search state not recorded: %+v", card)
	}
}

func TestMatchExplanationFlagsDuplicateAuthorsWithinSource(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		Title:    "Repeated author story",
		Category: "drama",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	expires := time.Now().Add(24 * time.Hour)
	seeds := []struct {
		sourceType string
		source     string
		kind       string
		externalID string
		author     string
		url        string
	}{
		{
			sourceType: "reddit_thread",
			source:     "reddit",
			kind:       "thread",
			externalID: "dupe-a",
			author:     "Clipper",
			url:        "https://www.reddit.com/r/LivestreamFail/comments/dupe-a/story/",
		},
		{
			sourceType: "reddit_thread",
			source:     "reddit",
			kind:       "thread",
			externalID: "dupe-b",
			author:     "@clipper",
			url:        "https://www.reddit.com/r/LivestreamFail/comments/dupe-b/story/",
		},
		{
			sourceType: "youtube_video",
			source:     "youtube",
			kind:       "video",
			externalID: "yt-same-name",
			author:     "Clipper",
			url:        "https://www.youtube.com/watch?v=yt-same-name",
		},
	}
	for _, seed := range seeds {
		itemID, err := st.UpsertSocialItem(ctx, SocialItem{
			Source:     seed.source,
			Kind:       seed.kind,
			ExternalID: seed.externalID,
			URL:        seed.url,
			Author:     seed.author,
			ExpiresAt:  expires,
		}, nil, nil)
		if err != nil {
			t.Fatalf("UpsertSocialItem %q: %v", seed.externalID, err)
		}
		if _, err := st.InsertEvidence(ctx, Evidence{
			ClusterID:  clusterID,
			ItemID:     &itemID,
			SourceType: seed.sourceType,
			SourceURL:  seed.url,
			MatchConf:  0.8,
			Weight:     1,
		}); err != nil {
			t.Fatalf("InsertEvidence %q: %v", seed.externalID, err)
		}
	}

	matches, err := st.MatchExplanationForCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("MatchExplanationForCluster: %v", err)
	}
	redditDupes := 0
	youtubeDupes := 0
	for _, match := range matches {
		if hasFactor(match.Factors, "duplicate_author:clipper") {
			switch match.SourceType {
			case "reddit_thread":
				redditDupes++
			case "youtube_video":
				youtubeDupes++
			}
		}
	}
	if redditDupes != 2 {
		t.Fatalf("expected two reddit duplicate-author factors, got %d in %+v", redditDupes, matches)
	}
	if youtubeDupes != 0 {
		t.Fatalf("expected source-scoped duplicate detection to ignore single youtube author, got %d in %+v", youtubeDupes, matches)
	}
}

func hasFactor(factors []string, want string) bool {
	for _, factor := range factors {
		if factor == want {
			return true
		}
	}
	return false
}

func TestSocialMetricSnapshotsRoundTripCommentHistory(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	expires := time.Now().Add(24 * time.Hour)
	itemID, err := st.UpsertSocialItem(ctx, SocialItem{
		Source:     "reddit",
		Kind:       "thread",
		ExternalID: "comment-history",
		URL:        "https://www.reddit.com/r/LivestreamFail/comments/comment-history/story/",
		Metrics:    json.RawMessage(`{"comments":10}`),
		ExpiresAt:  expires,
	}, nil, nil)
	if err != nil {
		t.Fatalf("UpsertSocialItem: %v", err)
	}
	base := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	commentCounts := []int{10, 12, 45}
	for i, comments := range commentCounts {
		raw, _ := json.Marshal(map[string]float64{"comments": float64(comments), "score": float64(100 + i)})
		if err := st.InsertSocialMetricSnapshot(ctx, itemID, base.Add(time.Duration(i)*time.Hour), "reddit", "comment-history", raw, &comments); err != nil {
			t.Fatalf("InsertSocialMetricSnapshot %d: %v", i, err)
		}
	}

	snaps, err := st.ListSocialMetricSnapshots(ctx, itemID, base.Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("ListSocialMetricSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("snapshot count = %d, want 3", len(snaps))
	}
	for i, snap := range snaps {
		if snap.Source != "reddit" || snap.ExternalID != "comment-history" {
			t.Fatalf("snapshot identity mismatch: %+v", snap)
		}
		if snap.Comments == nil || *snap.Comments != commentCounts[i] {
			t.Fatalf("snapshot comments[%d] = %v, want %d", i, snap.Comments, commentCounts[i])
		}
	}
}

func TestMarkStoryRecordsOperatorAudit(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		Title:    "Operator audit story",
		Category: "drama",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}

	action, err := st.MarkStory(ctx, clusterID, "mark_debunked", "aron", "source retracted")
	if err != nil {
		t.Fatalf("MarkStory: %v", err)
	}
	if action.Action != "mark_debunked" || action.Operator != "aron" || action.Note != "source retracted" {
		t.Fatalf("operator action mismatch: %+v", action)
	}
	var before map[string]string
	var after map[string]string
	if err := json.Unmarshal(action.BeforeData, &before); err != nil {
		t.Fatalf("before json: %v", err)
	}
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	if before["category"] != "drama" || before["state"] != "published" {
		t.Fatalf("before data = %+v", before)
	}
	if before["storyClass"] != "" {
		t.Fatalf("before story class = %+v", before)
	}
	if after["category"] != "debunked" || after["storyClass"] != "debunked" || after["state"] != "settled" {
		t.Fatalf("after data = %+v", after)
	}

	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil || card.Cluster.Category != "debunked" || card.Cluster.StoryClass != "debunked" || card.Cluster.State != "settled" {
		t.Fatalf("story after mark = %+v", card)
	}
	if len(card.OperatorActions) != 1 || card.OperatorActions[0].ID != action.ID {
		t.Fatalf("operator actions on story = %+v", card.OperatorActions)
	}
}

func TestConfirmStoryEntityAuditsEntityChange(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	firstID, err := st.UpsertEntity(ctx, "oldlogin", "", "Old Login", nil)
	if err != nil {
		t.Fatalf("UpsertEntity old: %v", err)
	}
	nextID, err := st.UpsertEntity(ctx, "newlogin", "", "New Login", nil)
	if err != nil {
		t.Fatalf("UpsertEntity next: %v", err)
	}
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		EntityID: &firstID,
		Title:    "Entity confirmation story",
		Category: "drama",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}

	action, err := st.ConfirmStoryEntity(ctx, clusterID, nextID, "aron", "confirmed streamer")
	if err != nil {
		t.Fatalf("ConfirmStoryEntity: %v", err)
	}
	if action.Action != "confirm_streamer_entity" || action.Operator != "aron" || action.Note != "confirmed streamer" {
		t.Fatalf("operator action mismatch: %+v", action)
	}
	var before map[string]any
	var after map[string]any
	if err := json.Unmarshal(action.BeforeData, &before); err != nil {
		t.Fatalf("before json: %v", err)
	}
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	if int64(before["entityId"].(float64)) != firstID || before["entityLogin"] != "oldlogin" {
		t.Fatalf("before entity audit = %+v", before)
	}
	if int64(after["entityId"].(float64)) != nextID || after["entityLogin"] != "newlogin" || after["entityDisplayName"] != "New Login" {
		t.Fatalf("after entity audit = %+v", after)
	}

	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil || card.Cluster.EntityID == nil || *card.Cluster.EntityID != nextID || card.Entity == nil || card.Entity.TwitchLogin != "newlogin" {
		t.Fatalf("story after entity confirm = %+v", card)
	}
	if len(card.OperatorActions) != 1 || card.OperatorActions[0].ID != action.ID {
		t.Fatalf("operator actions on story = %+v", card.OperatorActions)
	}
}

func TestConfirmStoryOriginAuditsMomentChange(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	entityID, err := st.UpsertEntity(ctx, "caseoh", "", "CaseOh", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	firstMomentID, err := st.InsertFingerprint(ctx, MomentFingerprint{
		EntityID:     &entityID,
		StreamID:     "stream-old",
		VODID:        "vod-old",
		VODOffsetS:   120,
		TranscriptKW: json.RawMessage(`["old quote"]`),
		TopEmotes:    json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("InsertFingerprint old: %v", err)
	}
	nextMomentID, err := st.InsertFingerprint(ctx, MomentFingerprint{
		EntityID:     &entityID,
		StreamID:     "stream-new",
		VODID:        "vod-new",
		VODOffsetS:   420,
		TranscriptKW: json.RawMessage(`["new quote"]`),
		TopEmotes:    json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("InsertFingerprint next: %v", err)
	}
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		EntityID:   &entityID,
		MomentFPID: &firstMomentID,
		Title:      "Origin confirmation story",
		Category:   "drama",
		State:      "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}

	action, err := st.ConfirmStoryOrigin(ctx, clusterID, nextMomentID, "aron", "confirmed origin")
	if err != nil {
		t.Fatalf("ConfirmStoryOrigin: %v", err)
	}
	if action.Action != "confirm_origin_moment" || action.Operator != "aron" || action.Note != "confirmed origin" {
		t.Fatalf("operator action mismatch: %+v", action)
	}
	var before map[string]any
	var after map[string]any
	if err := json.Unmarshal(action.BeforeData, &before); err != nil {
		t.Fatalf("before json: %v", err)
	}
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	if int64(before["momentFpId"].(float64)) != firstMomentID || before["streamId"] != "stream-old" || before["vodId"] != "vod-old" || int(before["vodOffsetS"].(float64)) != 120 {
		t.Fatalf("before origin audit = %+v", before)
	}
	if int64(after["momentFpId"].(float64)) != nextMomentID || after["streamId"] != "stream-new" || after["vodId"] != "vod-new" || int(after["vodOffsetS"].(float64)) != 420 {
		t.Fatalf("after origin audit = %+v", after)
	}

	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil || card.Cluster.MomentFPID == nil || *card.Cluster.MomentFPID != nextMomentID || card.Cluster.OriginSearchStatus != "matched" || card.Cluster.OriginCheckedAt == nil {
		t.Fatalf("story after origin confirm = %+v", card)
	}
	if card.Origin == nil || card.Origin.StreamID != "stream-new" || card.Origin.VODID != "vod-new" || card.Origin.VODOffsetS != 420 {
		t.Fatalf("origin after confirm = %+v", card.Origin)
	}
	if len(card.OperatorActions) != 1 || card.OperatorActions[0].ID != action.ID {
		t.Fatalf("operator actions on story = %+v", card.OperatorActions)
	}
}

func TestMergeDuplicateStoryMovesEvidenceAndAudits(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	targetID, err := st.InsertCluster(ctx, StoryCluster{Title: "Target story", Category: "drama", State: "published"})
	if err != nil {
		t.Fatalf("InsertCluster target: %v", err)
	}
	sourceID, err := st.InsertCluster(ctx, StoryCluster{Title: "Duplicate story", Category: "drama", State: "published"})
	if err != nil {
		t.Fatalf("InsertCluster source: %v", err)
	}
	evID, err := st.InsertEvidence(ctx, Evidence{
		ClusterID:  sourceID,
		SourceType: "reddit_thread",
		SourceURL:  "https://www.reddit.com/r/LivestreamFail/comments/merge/story/",
		MatchConf:  0.8,
		Weight:     0.7,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence source: %v", err)
	}
	previewID, err := st.UpsertEvidencePreview(ctx, EvidencePreview{
		CanonicalURL:  "https://www.reddit.com/r/LivestreamFail/comments/merge/story/",
		Platform:      "reddit",
		ProviderName:  "Reddit",
		Title:         "Duplicate evidence",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, sourceID, &evID, previewID, "url", "merge me"); err != nil {
		t.Fatalf("LinkEvidencePreview: %v", err)
	}

	action, err := st.MergeDuplicateStory(ctx, sourceID, targetID, "operator", "duplicate")
	if err != nil {
		t.Fatalf("MergeDuplicateStory: %v", err)
	}
	if action.Action != "merge_duplicate_story" || action.ClusterID != sourceID {
		t.Fatalf("operator action mismatch: %+v", action)
	}
	var after map[string]any
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	if int64(after["targetClusterId"].(float64)) != targetID || int(after["movedEvidence"].(float64)) != 1 || int(after["movedPreviews"].(float64)) != 1 {
		t.Fatalf("merge after audit = %+v", after)
	}

	var evidenceClusterID int64
	if err := st.pool.QueryRow(ctx, `SELECT cluster_id FROM story_evidence WHERE id = $1`, evID).Scan(&evidenceClusterID); err != nil {
		t.Fatalf("read moved evidence: %v", err)
	}
	if evidenceClusterID != targetID {
		t.Fatalf("evidence cluster = %d, want target %d", evidenceClusterID, targetID)
	}
	var previewClusterID int64
	if err := st.pool.QueryRow(ctx, `SELECT cluster_id FROM story_evidence_previews WHERE preview_id = $1`, previewID).Scan(&previewClusterID); err != nil {
		t.Fatalf("read moved preview: %v", err)
	}
	if previewClusterID != targetID {
		t.Fatalf("preview cluster = %d, want target %d", previewClusterID, targetID)
	}
	sourceCard, err := st.GetStory(ctx, sourceID, "local")
	if err != nil {
		t.Fatalf("GetStory source: %v", err)
	}
	if sourceCard == nil || sourceCard.Cluster.State != "suppressed" {
		t.Fatalf("source after merge = %+v", sourceCard)
	}
	targetCard, err := st.GetStory(ctx, targetID, "local")
	if err != nil {
		t.Fatalf("GetStory target: %v", err)
	}
	if targetCard == nil || len(targetCard.Receipts) != 1 || len(targetCard.EvidenceGallery) != 1 {
		t.Fatalf("target after merge = %+v", targetCard)
	}
}

func TestSplitUnrelatedEvidenceCreatesStoryAndAudits(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	sourceID, err := st.InsertCluster(ctx, StoryCluster{Title: "Mixed story", Category: "drama", State: "published"})
	if err != nil {
		t.Fatalf("InsertCluster source: %v", err)
	}
	keepID, err := st.InsertEvidence(ctx, Evidence{
		ClusterID:  sourceID,
		SourceType: "reddit_thread",
		SourceURL:  "https://example.com/keep",
		MatchConf:  0.8,
		Weight:     0.7,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence keep: %v", err)
	}
	moveID, err := st.InsertEvidence(ctx, Evidence{
		ClusterID:  sourceID,
		SourceType: "youtube_video",
		SourceURL:  "https://www.youtube.com/watch?v=split",
		MatchConf:  0.7,
		Weight:     0.6,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence move: %v", err)
	}
	previewID, err := st.UpsertEvidencePreview(ctx, EvidencePreview{
		CanonicalURL:  "https://www.youtube.com/watch?v=split",
		Platform:      "youtube",
		ProviderName:  "YouTube",
		Title:         "Split evidence",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, sourceID, &moveID, previewID, "url", "split me"); err != nil {
		t.Fatalf("LinkEvidencePreview: %v", err)
	}

	action, err := st.SplitUnrelatedEvidence(ctx, sourceID, []int64{moveID}, "Separated story", "operator", "unrelated source")
	if err != nil {
		t.Fatalf("SplitUnrelatedEvidence: %v", err)
	}
	if action.Action != "split_unrelated_evidence" || action.ClusterID != sourceID {
		t.Fatalf("operator action mismatch: %+v", action)
	}
	var after map[string]any
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	newClusterID := int64(after["newClusterId"].(float64))
	if newClusterID == sourceID || after["title"] != "Separated story" {
		t.Fatalf("split after audit = %+v", after)
	}

	var keepClusterID, moveClusterID, previewClusterID int64
	if err := st.pool.QueryRow(ctx, `SELECT cluster_id FROM story_evidence WHERE id = $1`, keepID).Scan(&keepClusterID); err != nil {
		t.Fatalf("read keep evidence: %v", err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT cluster_id FROM story_evidence WHERE id = $1`, moveID).Scan(&moveClusterID); err != nil {
		t.Fatalf("read moved evidence: %v", err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT cluster_id FROM story_evidence_previews WHERE preview_id = $1`, previewID).Scan(&previewClusterID); err != nil {
		t.Fatalf("read moved preview: %v", err)
	}
	if keepClusterID != sourceID || moveClusterID != newClusterID || previewClusterID != newClusterID {
		t.Fatalf("clusters after split keep=%d move=%d preview=%d new=%d source=%d", keepClusterID, moveClusterID, previewClusterID, newClusterID, sourceID)
	}
	newCard, err := st.GetStory(ctx, newClusterID, "local")
	if err != nil {
		t.Fatalf("GetStory new: %v", err)
	}
	if newCard == nil || newCard.Cluster.Title != "Separated story" || len(newCard.Receipts) != 1 || len(newCard.EvidenceGallery) != 1 {
		t.Fatalf("new story after split = %+v", newCard)
	}
}

func TestStoryClassDoesNotReplaceCategory(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		Title:      "Classified story",
		Category:   "drama",
		StoryClass: "drama_claim",
		State:      "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if card == nil {
		t.Fatal("story missing")
	}
	if card.Cluster.Category != "drama" {
		t.Fatalf("category = %q, want preserved drama", card.Cluster.Category)
	}
	if card.Cluster.StoryClass != "drama_claim" {
		t.Fatalf("storyClass = %q, want drama_claim", card.Cluster.StoryClass)
	}
}

func TestManualSuppressAuditsAndHidesFromDefaultFeed(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	clusterID := insertStoryWithEvidence(t, ctx, st, "story to suppress", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: now.Add(-10 * time.Minute)},
	})

	action, err := st.MarkStory(ctx, clusterID, "manual_suppress", "operator", "quality review")
	if err != nil {
		t.Fatalf("MarkStory manual_suppress: %v", err)
	}
	if action.Action != "manual_suppress" {
		t.Fatalf("action = %q, want manual_suppress", action.Action)
	}
	var before map[string]string
	if err := json.Unmarshal(action.BeforeData, &before); err != nil {
		t.Fatalf("before json: %v", err)
	}
	var after map[string]string
	if err := json.Unmarshal(action.AfterData, &after); err != nil {
		t.Fatalf("after json: %v", err)
	}
	if before["category"] != "news" || before["state"] != "published" {
		t.Fatalf("before data = %+v", before)
	}
	if after["category"] != "news" || after["storyClass"] != "" || after["state"] != "suppressed" {
		t.Fatalf("after data = %+v", after)
	}
	if err := st.UpdateClusterMeta(ctx, clusterID, nil, "revived title", "", "drama", "published"); err != nil {
		t.Fatalf("UpdateClusterMeta: %v", err)
	}

	defaultFeed, err := st.ListFeed(ctx, "", "", "", "rank", "24h", now.Add(-24*time.Hour), 10, 0)
	if err != nil {
		t.Fatalf("ListFeed default: %v", err)
	}
	if containsStoryID(defaultFeed, clusterID) {
		t.Fatalf("default feed included suppressed story %d: %+v", clusterID, defaultFeed)
	}

	suppressedFeed, err := st.ListFeed(ctx, "suppressed", "", "", "rank", "24h", now.Add(-24*time.Hour), 10, 0)
	if err != nil {
		t.Fatalf("ListFeed suppressed: %v", err)
	}
	if !containsStoryID(suppressedFeed, clusterID) {
		t.Fatalf("suppressed feed omitted story %d: %+v", clusterID, suppressedFeed)
	}
}

func TestWatchEntriesPersistDedupeAndDelete(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	category, err := st.UpsertWatchEntry(ctx, "local", "category", "BANS", "Bans")
	if err != nil {
		t.Fatalf("UpsertWatchEntry category: %v", err)
	}
	if category.Kind != "category" || category.Value != "bans" || category.Label != "Bans" {
		t.Fatalf("category watch = %+v", category)
	}
	keyword, err := st.UpsertWatchEntry(ctx, "local", "keyword", "  Contract Leak  ", "")
	if err != nil {
		t.Fatalf("UpsertWatchEntry keyword: %v", err)
	}
	if keyword.Kind != "keyword" || keyword.Value != "contract leak" || keyword.Label != "contract leak" {
		t.Fatalf("keyword watch = %+v", keyword)
	}
	again, err := st.UpsertWatchEntry(ctx, "local", "keyword", "contract   leak", "Contract leak")
	if err != nil {
		t.Fatalf("UpsertWatchEntry duplicate keyword: %v", err)
	}
	if again.ID != keyword.ID {
		t.Fatalf("duplicate keyword id = %d, want %d", again.ID, keyword.ID)
	}
	items, err := st.ListWatchEntries(ctx, "local")
	if err != nil {
		t.Fatalf("ListWatchEntries: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("watch entries count = %d, want 2: %+v", len(items), items)
	}
	if err := st.DeleteWatchEntry(ctx, "local", category.ID); err != nil {
		t.Fatalf("DeleteWatchEntry: %v", err)
	}
	items, err = st.ListWatchEntries(ctx, "local")
	if err != nil {
		t.Fatalf("ListWatchEntries after delete: %v", err)
	}
	if len(items) != 1 || items[0].ID != keyword.ID {
		t.Fatalf("watch entries after delete = %+v, want keyword only", items)
	}
}

func TestPreviewStoreUpsertLinkAndList(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()

	clusterID, err := st.InsertCluster(ctx, StoryCluster{Title: "Preview store test", State: "developing"})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	evidenceID, err := st.InsertEvidence(ctx, Evidence{
		ClusterID:  clusterID,
		SourceType: "reddit_thread",
		SourceURL:  "https://www.reddit.com/r/LivestreamFail/comments/abc/title/",
		MatchConf:  0.8,
		Weight:     0.7,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence: %v", err)
	}

	first := EvidencePreview{
		CanonicalURL:  "https://x.com/streamer/status/1",
		Platform:      "x",
		ProviderName:  "X",
		Title:         "first",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	}
	firstID, err := st.UpsertEvidencePreview(ctx, first)
	if err != nil {
		t.Fatalf("UpsertEvidencePreview first: %v", err)
	}
	first.Title = "updated"
	firstAgainID, err := st.UpsertEvidencePreview(ctx, first)
	if err != nil {
		t.Fatalf("UpsertEvidencePreview duplicate: %v", err)
	}
	if firstAgainID != firstID {
		t.Fatalf("duplicate canonical URL created id %d, want %d", firstAgainID, firstID)
	}

	secondID, err := st.UpsertEvidencePreview(ctx, EvidencePreview{
		CanonicalURL:  "https://www.youtube.com/watch?v=abc123",
		Platform:      "youtube",
		ProviderName:  "YouTube",
		ThumbnailURL:  "https://img.youtube.com/vi/abc123/hqdefault.jpg",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview second: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clusterID, &evidenceID, firstID, "url", ""); err != nil {
		t.Fatalf("LinkEvidencePreview first: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clusterID, &evidenceID, secondID, "url", "same evidence row"); err != nil {
		t.Fatalf("LinkEvidencePreview second: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clusterID, &evidenceID, firstID, "url", "duplicate link"); err != nil {
		t.Fatalf("LinkEvidencePreview duplicate: %v", err)
	}

	previews, err := st.ListEvidencePreviewsForCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("ListEvidencePreviewsForCluster: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("preview count = %d, want 2: %+v", len(previews), previews)
	}
	seen := map[string]EvidencePreview{}
	for _, preview := range previews {
		seen[preview.CanonicalURL] = preview
	}
	if seen[first.CanonicalURL].Title != "updated" {
		t.Fatalf("duplicate canonical URL did not reuse updated row: %+v", seen[first.CanonicalURL])
	}
	if seen["https://www.youtube.com/watch?v=abc123"].Note != "same evidence row" {
		t.Fatalf("second preview link note not listed: %+v", seen["https://www.youtube.com/watch?v=abc123"])
	}
}

func TestPreviewStoreRetryAndRefreshDue(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	failed := EvidencePreview{
		CanonicalURL:  "https://www.reddit.com/r/test/comments/retry/story/",
		Platform:      "reddit",
		ProviderName:  "Reddit",
		FetchedAt:     past,
		HTTPStatus:    500,
		Error:         "temporary failure",
		NextFetchAt:   &past,
		ExpiresAt:     past,
		PreviewStatus: "error",
	}
	id, err := st.UpsertEvidencePreview(ctx, failed)
	if err != nil {
		t.Fatalf("UpsertEvidencePreview failed insert: %v", err)
	}
	failed.FetchedAt = now
	failed.NextFetchAt = &past
	if _, err := st.UpsertEvidencePreview(ctx, failed); err != nil {
		t.Fatalf("UpsertEvidencePreview failed retry: %v", err)
	}
	var retryCount int
	var nextFetchAt time.Time
	if err := st.pool.QueryRow(ctx, `SELECT retry_count, next_fetch_at FROM evidence_previews WHERE id = $1`, id).Scan(&retryCount, &nextFetchAt); err != nil {
		t.Fatalf("read retry metadata: %v", err)
	}
	if retryCount != 2 {
		t.Fatalf("retry_count = %d, want 2", retryCount)
	}
	if !nextFetchAt.After(now) {
		t.Fatalf("next_fetch_at = %s, want after %s", nextFetchAt, now)
	}

	dueID, err := st.UpsertEvidencePreview(ctx, EvidencePreview{
		CanonicalURL: "https://clips.twitch.tv/FunnySlug",
		Platform:     "twitch_clip",
		ProviderName: "Twitch",
		EmbedURL:     "https://clips.twitch.tv/embed?clip=FunnySlug",
		FetchedAt:    past,
		NextFetchAt:  &past,
		ExpiresAt:    past,
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview due: %v", err)
	}
	if _, err := st.UpsertEvidencePreview(ctx, EvidencePreview{
		CanonicalURL: "https://www.youtube.com/watch?v=future",
		Platform:     "youtube",
		ProviderName: "YouTube",
		ThumbnailURL: "https://img.youtube.com/vi/future/hqdefault.jpg",
		FetchedAt:    now,
		NextFetchAt:  &future,
		ExpiresAt:    future,
	}); err != nil {
		t.Fatalf("UpsertEvidencePreview future: %v", err)
	}
	due, err := st.ListEvidencePreviewsDueForRefresh(ctx, 10)
	if err != nil {
		t.Fatalf("ListEvidencePreviewsDueForRefresh: %v", err)
	}
	if len(due) != 1 || due[0].ID != dueID {
		t.Fatalf("due previews = %+v, want only id %d", due, dueID)
	}
}

func TestListFeedWindowAndQualityRanking(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)

	multiID := insertStoryWithEvidence(t, ctx, st, "corroborated ban story", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: now.Add(-2 * time.Hour)},
		{sourceType: "streamerbans_post", occurredAt: now.Add(-90 * time.Minute)},
		{sourceType: "youtube_video", occurredAt: now.Add(-30 * time.Minute)},
	})
	twitchOnlyID := insertStoryWithEvidence(t, ctx, st, "single twitch clip", []evidenceSeed{
		{sourceType: "twitch_clip", occurredAt: now.Add(-30 * time.Minute)},
	})
	staleID := insertStoryWithEvidence(t, ctx, st, "stale reddit story", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: old},
	})

	feed24, err := st.ListFeed(ctx, "", "", "", "rank", "24h", now.Add(-24*time.Hour), 10, 0)
	if err != nil {
		t.Fatalf("ListFeed 24h: %v", err)
	}
	if len(feed24) != 2 {
		t.Fatalf("24h feed count = %d, want 2: %+v", len(feed24), feed24)
	}
	if feed24[0].Cluster.ID != multiID {
		t.Fatalf("ranked first id = %d, want corroborated story %d ahead of twitch-only %d", feed24[0].Cluster.ID, multiID, twitchOnlyID)
	}
	if feed24[0].WindowScores == nil || feed24[0].WindowScores.RecentSourceDelta != 1 {
		t.Fatalf("recent source delta = %+v, want 1 new source in last hour", feed24[0].WindowScores)
	}
	for _, card := range feed24 {
		if card.Cluster.ID == staleID {
			t.Fatalf("24h feed included stale story %d", staleID)
		}
	}

	feed7d, err := st.ListFeed(ctx, "", "", "", "rank", "7d", now.Add(-7*24*time.Hour), 10, 0)
	if err != nil {
		t.Fatalf("ListFeed 7d: %v", err)
	}
	if !containsStoryID(feed7d, staleID) {
		t.Fatalf("7d feed omitted stale-within-week story %d: %+v", staleID, feed7d)
	}
}

func TestSourceMixUsesEvidenceWindow(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	insertStoryWithEvidence(t, ctx, st, "recent source mix", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: now.Add(-2 * time.Hour)},
	})
	insertStoryWithEvidence(t, ctx, st, "old source mix", []evidenceSeed{
		{sourceType: "youtube_video", occurredAt: now.Add(-48 * time.Hour)},
	})

	mix, err := st.SourceMix(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SourceMix: %v", err)
	}
	if mix["reddit_thread"] != 1 {
		t.Fatalf("recent reddit count = %d, want 1", mix["reddit_thread"])
	}
	if mix["youtube_video"] != 0 {
		t.Fatalf("old youtube count = %d, want 0", mix["youtube_video"])
	}
}

func TestListTrendingStreamersUsesWindowedEvidence(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	xqcID, err := st.UpsertEntity(ctx, "xqc", "71092938", "xQc", nil)
	if err != nil {
		t.Fatalf("UpsertEntity xqc: %v", err)
	}
	forsenID, err := st.UpsertEntity(ctx, "forsen", "22484632", "forsen", nil)
	if err != nil {
		t.Fatalf("UpsertEntity forsen: %v", err)
	}
	insertEntityStoryWithEvidence(t, ctx, st, &xqcID, "xQc reddit story", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: now.Add(-2 * time.Hour)},
		{sourceType: "streamerbans_post", occurredAt: now.Add(-90 * time.Minute)},
	})
	insertEntityStoryWithEvidence(t, ctx, st, &forsenID, "forsen clip story", []evidenceSeed{
		{sourceType: "twitch_clip", occurredAt: now.Add(-30 * time.Minute)},
	})
	insertEntityStoryWithEvidence(t, ctx, st, &forsenID, "forsen old story", []evidenceSeed{
		{sourceType: "reddit_thread", occurredAt: now.Add(-48 * time.Hour)},
	})

	items, err := st.ListTrendingStreamers(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ListTrendingStreamers: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("trending items = %+v, want at least two streamers", items)
	}
	if items[0].Login != "xqc" || items[0].EvidenceCount != 2 || items[0].SourceDiversity != 2 {
		t.Fatalf("first trending streamer = %+v, want xqc with evidence/source diversity", items[0])
	}
	for _, item := range items {
		if item.Login == "forsen" && item.StoryCount != 1 {
			t.Fatalf("forsen 24h story count = %d, want old story excluded", item.StoryCount)
		}
	}
}

type evidenceSeed struct {
	sourceType string
	occurredAt time.Time
}

func insertStoryWithEvidence(t *testing.T, ctx context.Context, st *Store, title string, seeds []evidenceSeed) int64 {
	t.Helper()
	return insertEntityStoryWithEvidence(t, ctx, st, nil, title, seeds)
}

func insertEntityStoryWithEvidence(t *testing.T, ctx context.Context, st *Store, entityID *int64, title string, seeds []evidenceSeed) int64 {
	t.Helper()
	clusterID, err := st.InsertCluster(ctx, StoryCluster{
		EntityID: entityID,
		Title:    title,
		State:    "published",
		Category: "news",
	})
	if err != nil {
		t.Fatalf("InsertCluster %q: %v", title, err)
	}
	for _, seed := range seeds {
		occurredAt := seed.occurredAt
		if _, err := st.InsertEvidence(ctx, Evidence{
			ClusterID:  clusterID,
			SourceType: seed.sourceType,
			SourceURL:  "https://example.com/" + seed.sourceType,
			MatchConf:  0.8,
			Weight:     0.7,
			OccurredAt: &occurredAt,
		}); err != nil {
			t.Fatalf("InsertEvidence %q: %v", seed.sourceType, err)
		}
	}
	return clusterID
}

func containsStoryID(cards []StoryCard, id int64) bool {
	for _, card := range cards {
		if card.Cluster.ID == id {
			return true
		}
	}
	return false
}

func TestUpsertSocialItemMetricsMerge(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	expires := time.Now().Add(24 * time.Hour)
	helixThumb := "https://static-cdn.jtvnw.net/twitch-video-assets/thumb-480x272.jpg"
	firstMetrics, _ := json.Marshal(map[string]any{
		"views":            float64(100),
		"thumbnail_url":    helixThumb,
		"thumbnail_source": "helix",
		"thumbnail_status": "ready",
	})
	id, err := st.UpsertSocialItem(ctx, SocialItem{
		Source:     "twitch_clip",
		Kind:       "clip",
		ExternalID: "ClipMergeTest123",
		URL:        "https://clips.twitch.tv/ClipMergeTest123",
		Text:       "merge test clip",
		Metrics:    firstMetrics,
		ExpiresAt:  expires,
	}, json.RawMessage("{}"), []byte("snap1"))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	secondMetrics, _ := json.Marshal(map[string]any{"views": float64(150)})
	if _, err := st.UpsertSocialItem(ctx, SocialItem{
		Source:     "twitch_clip",
		Kind:       "clip",
		ExternalID: "ClipMergeTest123",
		URL:        "https://clips.twitch.tv/ClipMergeTest123",
		Text:       "merge test clip updated",
		Metrics:    secondMetrics,
		ExpiresAt:  expires,
	}, json.RawMessage("{}"), []byte("snap2")); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	var stored json.RawMessage
	if err := st.pool.QueryRow(ctx, `SELECT metrics FROM social_items WHERE id = $1`, id).Scan(&stored); err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	var metrics map[string]any
	if err := json.Unmarshal(stored, &metrics); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if metrics["views"] != float64(150) {
		t.Fatalf("views = %v, want 150", metrics["views"])
	}
	if metrics["thumbnail_url"] != helixThumb {
		t.Fatalf("thumbnail_url = %v, want preserved helix URL", metrics["thumbnail_url"])
	}
	if metrics["thumbnail_source"] != "helix" {
		t.Fatalf("thumbnail_source = %v", metrics["thumbnail_source"])
	}
	if metrics["thumbnail_status"] != "ready" {
		t.Fatalf("thumbnail_status = %v", metrics["thumbnail_status"])
	}
}

func TestUpsertSocialItemPreservesGoodTextOnPlaceholderUpsert(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	expires := time.Now().Add(24 * time.Hour)
	goodText := "Mitch Jones gets cooked by the new AI summary"
	id, err := st.UpsertSocialItem(ctx, SocialItem{
		Source:     "reddit",
		Kind:       "post",
		ExternalID: "t3_placeholder_guard",
		URL:        "https://reddit.com/r/LivestreamFail/comments/abc/mitch_jones_gets_cooked/",
		Text:       goodText,
		ExpiresAt:  expires,
	}, json.RawMessage("{}"), []byte("snap1"))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if _, err := st.UpsertSocialItem(ctx, SocialItem{
		Source:     "reddit",
		Kind:       "post",
		ExternalID: "t3_placeholder_guard",
		URL:        "https://reddit.com/r/LivestreamFail/comments/abc/mitch_jones_gets_cooked/",
		Text:       "17 comments",
		ExpiresAt:  expires,
	}, json.RawMessage("{}"), []byte("snap2")); err != nil {
		t.Fatalf("placeholder upsert: %v", err)
	}
	var stored string
	if err := st.pool.QueryRow(ctx, `SELECT text FROM social_items WHERE id = $1`, id).Scan(&stored); err != nil {
		t.Fatalf("read text: %v", err)
	}
	if stored != goodText {
		t.Fatalf("text = %q, want preserved %q", stored, goodText)
	}
}
