package reddit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchLSFRecentHotPrefersOldRedditJSONBeforeScraper(t *testing.T) {
	publicJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer publicJSON.Close()

	var scraperCalls atomic.Int32
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scraperCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "BrowserContext.new_page: Target page, context or browser has been closed",
		})
	}))
	defer scraper.Close()

	oldReddit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/r/LivestreamFail/hot.json") {
			t.Fatalf("unexpected old reddit path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"children": []map[string]any{
					{
						"data": map[string]any{
							"id":           "abc123",
							"title":        "Streamer moment reaches LSF",
							"permalink":    "/r/LivestreamFail/comments/abc123/story/",
							"url":          "https://clips.twitch.tv/example",
							"author":       "poster",
							"subreddit":    "LivestreamFail",
							"score":        1200,
							"num_comments": 81,
							"created_utc":  1760000000,
						},
					},
				},
			},
		})
	}))
	defer oldReddit.Close()

	client := New(Options{
		BaseURL:      publicJSON.URL,
		OldRedditURL: oldReddit.URL,
		ScraperURL:   scraper.URL,
		ScraperKey:   "test-key",
		UserAgent:    "streamclone-test/1.0",
	})

	posts, status := client.fetchLSFRecentHot(context.Background(), "", nil)
	if status.Provider != "old_reddit_json" || status.State != "ready" {
		t.Fatalf("status = %+v, want old_reddit_json ready", status)
	}
	if len(posts) != 1 || posts[0].ID != "abc123" {
		t.Fatalf("posts = %+v, want fallback post", posts)
	}
	if got := scraperCalls.Load(); got != 0 {
		t.Fatalf("scraper calls = %d, want 0 when old Reddit JSON succeeds", got)
	}
}

func TestScrapeRedditListingURLRetriesTransientEOF(t *testing.T) {
	var calls atomic.Int32
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"html": `<div><a data-testid="post-title" href="/r/LivestreamFail/comments/retry/story/">Retry title</a></div>`,
			},
		})
	}))
	defer scraper.Close()

	client := New(Options{
		ScraperURL: scraper.URL,
		ScraperKey: "test-key",
		BaseURL:    "https://www.reddit.com",
		UserAgent:  "streamclone-test/1.0",
	})
	posts, status := client.scrapeRedditListingURL(context.Background(), "https://www.reddit.com/r/LivestreamFail/hot", "", "scraper_hot", 1000, nil)
	if status.State != "ready" {
		t.Fatalf("status = %+v, want ready", status)
	}
	if len(posts) != 1 || posts[0].Title != "Retry title" {
		t.Fatalf("posts = %+v, want retry post", posts)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestFetchLSFScraperRespectsBrowserBudget(t *testing.T) {
	var calls atomic.Int32
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"html": `<html><body>no matching posts</body></html>`,
			},
		})
	}))
	defer scraper.Close()

	client := New(Options{
		Provider:   "scraper",
		ScraperURL: scraper.URL,
		ScraperKey: "test-key",
		BaseURL:    "https://www.reddit.com",
		UserAgent:  "streamclone-test/1.0",
	})
	posts, statuses := client.fetchLSF(context.Background(), "caseoh", "7d", "hot", newBrowserFetchBudget(1))
	if len(posts) != 0 {
		t.Fatalf("posts = %+v, want none", posts)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("scraper calls = %d, want 1", got)
	}
	var sawBudgetExhausted bool
	for _, status := range statuses {
		if strings.Contains(status.Message, "browser fetch budget exhausted") {
			sawBudgetExhausted = true
		}
	}
	if !sawBudgetExhausted {
		t.Fatalf("statuses = %+v, want budget exhaustion", statuses)
	}
}

func TestFetchLSFScraperDisabledBudgetDoesNotCallScraper(t *testing.T) {
	var calls atomic.Int32
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer scraper.Close()

	client := New(Options{
		Provider:   "scraper",
		ScraperURL: scraper.URL,
		ScraperKey: "test-key",
		BaseURL:    "https://www.reddit.com",
		UserAgent:  "streamclone-test/1.0",
	})
	posts, statuses := client.fetchLSF(context.Background(), "caseoh", "7d", "hot", newBrowserFetchBudget(-1))
	if len(posts) != 0 {
		t.Fatalf("posts = %+v, want none", posts)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("scraper calls = %d, want 0", got)
	}
	if len(statuses) == 0 || !strings.Contains(statuses[0].Message, "browser fetch budget exhausted") {
		t.Fatalf("statuses = %+v, want budget exhaustion", statuses)
	}
}
