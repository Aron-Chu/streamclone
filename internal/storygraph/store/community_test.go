package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func insertCommunityRedditPost(t *testing.T, ctx context.Context, st *Store, externalID, title, flair string, postedAt time.Time, score float64) {
	t.Helper()
	metrics, err := json.Marshal(map[string]any{
		"flair": flair,
		"score": score,
	})
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}
	_, err = st.UpsertSocialItem(ctx, SocialItem{
		Source:       "reddit",
		Kind:         "post",
		ExternalID:   externalID,
		URL:          fmt.Sprintf("https://www.reddit.com/r/LivestreamFail/comments/%s/slug/", externalID),
		Text:         title,
		Metrics:      metrics,
		CreatedAtSrc: &postedAt,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}, nil, nil)
	if err != nil {
		t.Fatalf("UpsertSocialItem %q: %v", externalID, err)
	}
}

func TestListCommunityFlairsRanksByCount(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)

	insertCommunityRedditPost(t, ctx, st, "drama-1", "Drama post one", "Drama", now.Add(-2*time.Hour), 100)
	insertCommunityRedditPost(t, ctx, st, "drama-2", "Drama post two", "Drama", now.Add(-3*time.Hour), 90)
	insertCommunityRedditPost(t, ctx, st, "drama-3", "Drama post three", "Drama", now.Add(-4*time.Hour), 80)
	insertCommunityRedditPost(t, ctx, st, "funny-1", "Funny post one", "Funny", now.Add(-90*time.Minute), 70)
	insertCommunityRedditPost(t, ctx, st, "funny-2", "Funny post two", "Funny", now.Add(-100*time.Minute), 60)
	insertCommunityRedditPost(t, ctx, st, "blank-1", "No flair post", "", now.Add(-50*time.Minute), 50)

	flairs, err := st.ListCommunityFlairs(ctx, since, 10)
	if err != nil {
		t.Fatalf("ListCommunityFlairs: %v", err)
	}
	if len(flairs) != 2 {
		t.Fatalf("flair count = %d, want 2: %+v", len(flairs), flairs)
	}
	if flairs[0].Flair != "Drama" || flairs[0].Count != 3 {
		t.Fatalf("first flair = %+v, want Drama x3", flairs[0])
	}
	if flairs[1].Flair != "Funny" || flairs[1].Count != 2 {
		t.Fatalf("second flair = %+v, want Funny x2", flairs[1])
	}
}

func TestListCommunityPostsFlairFilterCaseInsensitive(t *testing.T) {
	ctx, st := setupPreviewStoreTest(t)
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)

	insertCommunityRedditPost(t, ctx, st, "ann-1", "Announcement post", "Announcement", now.Add(-2*time.Hour), 120)
	insertCommunityRedditPost(t, ctx, st, "drama-1", "Drama post", "Drama", now.Add(-90*time.Minute), 200)

	posts, err := st.ListCommunityPosts(ctx, "hot", "", "announcement", since, 10)
	if err != nil {
		t.Fatalf("ListCommunityPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("post count = %d, want 1: %+v", len(posts), posts)
	}
	if posts[0].Flair != "Announcement" {
		t.Fatalf("flair = %q, want Announcement", posts[0].Flair)
	}
}
