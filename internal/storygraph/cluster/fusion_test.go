package cluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/storygraph/store"
)

func setupFusionStoreTest(t *testing.T) (context.Context, *store.Store) {
	t.Helper()
	dsn := os.Getenv("STORYGRAPH_STORE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set STORYGRAPH_STORE_TEST_DATABASE_URL to run storygraph fusion store tests")
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
	applyFusionMigrations(t, ctx, pool)
	return ctx, store.New(pool)
}

func applyFusionMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "migrations")
	for _, name := range []string{
		"000015_story_graph_core.up.sql",
		"000016_story_graph_social.up.sql",
		"000018_evidence_previews.up.sql",
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

func TestEnsureWireStoryFusesByCanonicalURL(t *testing.T) {
	ctx, st := setupFusionStoreTest(t)
	now := time.Now().UTC()
	entityID, err := st.UpsertEntity(ctx, "xqc", "71092938", "xQc", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	clusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "xQc banned from Twitch after DMCA complaint",
		Category: "bans",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	evidenceID, err := st.InsertEvidence(ctx, store.Evidence{
		ClusterID:  clusterID,
		SourceType: "youtube_video",
		SourceURL:  "https://www.youtube.com/watch?v=fused",
		MatchConf:  0.9,
		Weight:     0.7,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence: %v", err)
	}
	previewID, err := st.UpsertEvidencePreview(ctx, store.EvidencePreview{
		CanonicalURL:  "https://www.youtube.com/watch?v=fused",
		Platform:      "youtube",
		ProviderName:  "YouTube",
		Title:         "xQc ban clip",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clusterID, &evidenceID, previewID, "url", "seed"); err != nil {
		t.Fatalf("LinkEvidencePreview: %v", err)
	}
	socialID, err := st.UpsertSocialItem(ctx, store.SocialItem{
		Source:       "reddit",
		Kind:         "post",
		ExternalID:   "reddit-fusion",
		URL:          "https://www.reddit.com/r/LivestreamFail/comments/fusion/title/",
		Text:         "Reddit discusses the xQc incident https://youtu.be/fused",
		EntityID:     &entityID,
		CreatedAtSrc: &now,
		ExpiresAt:    now.Add(7 * 24 * time.Hour),
	}, nil, nil)
	if err != nil {
		t.Fatalf("UpsertSocialItem: %v", err)
	}

	gotID, alreadyLinked, matchKind, err := New(st).EnsureWireStory(ctx, socialID, &entityID, "Reddit discusses the xQc incident https://youtu.be/fused", "reddit", "", []string{"https://www.youtube.com/watch?v=fused"})
	if err != nil {
		t.Fatalf("EnsureWireStory: %v", err)
	}
	if gotID != clusterID || alreadyLinked || matchKind != "canonical_url" {
		t.Fatalf("fusion = (id=%d, already=%v, kind=%s), want (%d, false, canonical_url)", gotID, alreadyLinked, matchKind, clusterID)
	}
}

func TestEnsureWireStoryFusesRedditWithTwitchClipURL(t *testing.T) {
	ctx, st := setupFusionStoreTest(t)
	now := time.Now().UTC()
	entityID, err := st.UpsertEntity(ctx, "emiru", "123", "emiru", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	clipURL := "https://clips.twitch.tv/HyperIntelligentCookieKeepo-hAMEW9U2yJC0kOXf"
	clusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "BASED",
		Category: "news",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	evidenceID, err := st.InsertEvidence(ctx, store.Evidence{
		ClusterID:  clusterID,
		SourceType: "twitch_clip",
		SourceURL:  clipURL,
		MatchConf:  0.9,
		Weight:     0.85,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence: %v", err)
	}
	previewID, err := st.UpsertEvidencePreview(ctx, store.EvidencePreview{
		CanonicalURL:  clipURL,
		Platform:      "twitch",
		ProviderName:  "Twitch",
		Title:         "BASED",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clusterID, &evidenceID, previewID, "url", "seed"); err != nil {
		t.Fatalf("LinkEvidencePreview: %v", err)
	}
	socialID, err := st.UpsertSocialItem(ctx, store.SocialItem{
		Source:       "reddit",
		Kind:         "post",
		ExternalID:   "reddit-clip-fusion",
		URL:          "https://www.reddit.com/r/LivestreamFail/comments/clipfusion/title/",
		Text:         "BASED clip on LSF " + clipURL,
		EntityID:     &entityID,
		CreatedAtSrc: &now,
		ExpiresAt:    now.Add(7 * 24 * time.Hour),
	}, nil, nil)
	if err != nil {
		t.Fatalf("UpsertSocialItem: %v", err)
	}

	gotID, alreadyLinked, matchKind, err := New(st).EnsureWireStory(ctx, socialID, &entityID, "BASED clip on LSF "+clipURL, "reddit", "", []string{clipURL})
	if err != nil {
		t.Fatalf("EnsureWireStory: %v", err)
	}
	if gotID != clusterID || alreadyLinked || matchKind != "canonical_url" {
		t.Fatalf("fusion = (id=%d, already=%v, kind=%s), want (%d, false, canonical_url)", gotID, alreadyLinked, matchKind, clusterID)
	}
}

func TestEnsureWireStoryFusesOnlySimilarEntityTitles(t *testing.T) {
	ctx, st := setupFusionStoreTest(t)
	entityID, err := st.UpsertEntity(ctx, "xqc", "71092938", "xQc", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	clusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "xQc banned from Twitch after DMCA complaint",
		Category: "bans",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	gotID, alreadyLinked, matchKind, err := New(st).EnsureWireStory(ctx, 0, &entityID, "xQc has been banned from Twitch after DMCA complaint", "streamerbans", "", nil)
	if err != nil {
		t.Fatalf("EnsureWireStory positive: %v", err)
	}
	if gotID != clusterID || alreadyLinked || matchKind != "title_similarity" {
		t.Fatalf("title fusion = (id=%d, already=%v, kind=%s), want (%d, false, title_similarity)", gotID, alreadyLinked, matchKind, clusterID)
	}
	unrelatedID, _, unrelatedKind, err := New(st).EnsureWireStory(ctx, 0, &entityID, "xQc wins chess tournament with a brilliant endgame", "reddit", "", nil)
	if err != nil {
		t.Fatalf("EnsureWireStory negative: %v", err)
	}
	if unrelatedID == clusterID || unrelatedKind != "new_story" {
		t.Fatalf("unrelated title fused into id=%d kind=%s, want new story distinct from %d", unrelatedID, unrelatedKind, clusterID)
	}
}

func TestEnsureWireStoryFusesItemDedupRedditIntoClipCluster(t *testing.T) {
	ctx, st := setupFusionStoreTest(t)
	now := time.Now().UTC()
	entityID, err := st.UpsertEntity(ctx, "emiru", "123", "emiru", nil)
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	clipURL := "https://clips.twitch.tv/HyperIntelligentCookieKeepo-hAMEW9U2yJC0kOXf"
	clipClusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "BASED",
		Category: "news",
		State:    "published",
	})
	if err != nil {
		t.Fatalf("InsertCluster clip: %v", err)
	}
	clipEvidenceID, err := st.InsertEvidence(ctx, store.Evidence{
		ClusterID:  clipClusterID,
		SourceType: "twitch_clip",
		SourceURL:  clipURL,
		MatchConf:  0.9,
		Weight:     0.85,
		OccurredAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertEvidence clip: %v", err)
	}
	previewID, err := st.UpsertEvidencePreview(ctx, store.EvidencePreview{
		CanonicalURL:  clipURL,
		Platform:      "twitch",
		ProviderName:  "Twitch",
		Title:         "BASED",
		FetchedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
		PreviewStatus: "ready",
	})
	if err != nil {
		t.Fatalf("UpsertEvidencePreview: %v", err)
	}
	if err := st.LinkEvidencePreview(ctx, clipClusterID, &clipEvidenceID, previewID, "url", "seed"); err != nil {
		t.Fatalf("LinkEvidencePreview: %v", err)
	}

	redditClusterID, err := st.InsertCluster(ctx, store.StoryCluster{
		EntityID: &entityID,
		Title:    "14 comments",
		Category: "news",
		State:    "unverified",
	})
	if err != nil {
		t.Fatalf("InsertCluster reddit: %v", err)
	}
	socialID, err := st.UpsertSocialItem(ctx, store.SocialItem{
		Source:       "reddit",
		Kind:         "post",
		ExternalID:   "reddit-item-dedup-fusion",
		URL:          "https://www.reddit.com/r/LivestreamFail/comments/clipfusion2/title/",
		Text:         "BASED clip on LSF " + clipURL,
		EntityID:     &entityID,
		CreatedAtSrc: &now,
		ExpiresAt:    now.Add(7 * 24 * time.Hour),
	}, nil, nil)
	if err != nil {
		t.Fatalf("UpsertSocialItem: %v", err)
	}
	if _, err := st.InsertEvidence(ctx, store.Evidence{
		ClusterID:  redditClusterID,
		ItemID:     &socialID,
		SourceType: "reddit_thread",
		SourceURL:  "https://www.reddit.com/r/LivestreamFail/comments/clipfusion2/title/",
		MatchConf:  0.58,
		Weight:     0.58,
		OccurredAt: &now,
	}); err != nil {
		t.Fatalf("InsertEvidence reddit: %v", err)
	}

	gotID, alreadyLinked, matchKind, err := New(st).EnsureWireStory(ctx, socialID, &entityID, "BASED clip on LSF "+clipURL, "reddit", "", []string{clipURL})
	if err != nil {
		t.Fatalf("EnsureWireStory: %v", err)
	}
	if gotID != clipClusterID || !alreadyLinked || matchKind != "canonical_url" {
		t.Fatalf("fusion = (id=%d, already=%v, kind=%s), want (%d, true, canonical_url)", gotID, alreadyLinked, matchKind, clipClusterID)
	}
	var evidenceClusterID int64
	if evidenceClusterID, err = st.ClusterByItemID(ctx, socialID); err != nil || evidenceClusterID == 0 {
		t.Fatalf("read reddit evidence cluster: %v id=%d", err, evidenceClusterID)
	}
	if evidenceClusterID != clipClusterID {
		t.Fatalf("reddit evidence cluster = %d, want %d", evidenceClusterID, clipClusterID)
	}
}
