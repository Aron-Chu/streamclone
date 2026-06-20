package reddit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamclone/internal/config"
	"streamclone/internal/social"
)

func TestSearchOfficialOAuthPreservesMetadata(t *testing.T) {
	var sawToken bool
	var sawSearch bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			sawToken = true
			user, pass, ok := r.BasicAuth()
			if !ok || user != "client-id" || pass != "client-secret" {
				t.Fatalf("missing oauth basic auth: user=%q ok=%v", user, ok)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "oauth-token",
				"expires_in":   3600,
			})
		case "/r/LivestreamFail/search":
			sawSearch = true
			if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
				t.Fatalf("authorization = %q", got)
			}
			if got := r.URL.Query().Get("q"); got != "caseoh" {
				t.Fatalf("q = %q, want caseoh", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"children": []map[string]any{
						{
							"data": map[string]any{
								"id":              "abc123",
								"title":           "CaseOh clip reaches LSF",
								"url":             "https://clips.twitch.tv/CaseOhClip",
								"permalink":       "/r/LivestreamFail/comments/abc123/story/",
								"thumbnail":       "https://preview.redd.it/caseoh.jpg",
								"author":          "poster",
								"subreddit":       "LivestreamFail",
								"link_flair_text": "CaseOh",
								"score":           5500,
								"num_comments":    302,
								"created_utc":     1760000000,
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	src := NewSource(config.Config{
		RedditProvider:      "official",
		RedditAPIURL:        server.URL,
		RedditOAuthAPIURL:   server.URL,
		RedditTokenURL:      server.URL + "/token",
		RedditClientID:      "client-id",
		RedditClientSecret:  "client-secret",
		RedditHTMLFallback:  false,
		SocialRetentionDays: 30,
		RedditCommercialOK:  true,
	})
	page, err := src.Search(t.Context(), social.Query{
		Entity: social.EntityRef{TwitchLogin: "caseoh"},
		Budget: social.Budget{MaxItems: 4},
	})
	if err != nil {
		t.Fatalf("search oauth: %v", err)
	}
	if !sawToken || !sawSearch {
		t.Fatalf("token=%v search=%v", sawToken, sawSearch)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(page.Items), page.Items)
	}
	item := page.Items[0]
	if item.Provenance.SourceAPI != "reddit_oauth" {
		t.Fatalf("source api = %q, want reddit_oauth", item.Provenance.SourceAPI)
	}
	if item.ExternalID != "abc123" || item.Author != "poster" {
		t.Fatalf("metadata not preserved: %+v", item)
	}
	if item.FlairText != "CaseOh" || item.EntityTwitchLogin != "caseoh" || item.EntityDisplayName != "CaseOh" {
		t.Fatalf("entity/flair not preserved: %+v", item)
	}
	if item.Metrics["score"] != 5500 || item.Metrics["comments"] != 302 {
		t.Fatalf("metrics not preserved: %+v", item.Metrics)
	}
	if len(item.Media) != 1 || item.Media[0].Kind != "image" || item.Media[0].URL != "https://preview.redd.it/caseoh.jpg" {
		t.Fatalf("thumbnail media not preserved: %+v", item.Media)
	}
}
