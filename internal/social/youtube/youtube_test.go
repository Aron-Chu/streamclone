package youtube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/social"
)

func TestParseYouTubeSearchHTML(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixture, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", "search_results.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	results, err := parseYouTubeSearchHTMLLimit(string(fixture), 4)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].VideoID != "dQw4w9WgXcQ" {
		t.Fatalf("unexpected first id %q", results[0].VideoID)
	}
	if results[0].Title != "Kai Cenat reacts to viral clip" {
		t.Fatalf("unexpected first title %q", results[0].Title)
	}
	if results[1].VideoID != "abcdefghijk" {
		t.Fatalf("unexpected second id %q", results[1].VideoID)
	}
	if results[1].Title != "Streamer drama recap" {
		t.Fatalf("unexpected second title %q", results[1].Title)
	}
}

func TestParseYouTubeSearchHTMLFallsBackToWatchAndShortsLinks(t *testing.T) {
	html := `<!doctype html><script>var ytInitialData = {"contents":{"empty":true}};</script>
	<a href="/watch?v=abcdefghijk">watch</a>
	<a href="/shorts=differentbad">ignored</a>
	<a href="/shorts/ZYXWVUTSRQP">short</a>`
	results, err := parseYouTubeSearchHTMLLimit(html, 4)
	if err != nil {
		t.Fatalf("parse fallback links: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 fallback results, got %d", len(results))
	}
	if results[0].VideoID != "abcdefghijk" || results[1].VideoID != "ZYXWVUTSRQP" {
		t.Fatalf("unexpected fallback ids %+v", results)
	}
}

func TestParseYouTubeSearchHTMLEmptyInitialDataKeepsNoVideosError(t *testing.T) {
	html := `<!doctype html><script>var ytInitialData = {"contents":{"empty":true}};</script>`
	_, err := parseYouTubeSearchHTMLLimit(html, 4)
	if err == nil || err.Error() != "no videos in ytInitialData" {
		t.Fatalf("expected no videos in ytInitialData, got %v", err)
	}
}

func TestHealthyAllowsAPIOrScraper(t *testing.T) {
	src := NewSource(config.Config{ScraperAPIURL: "http://scraper:8000/v2/scrape"})
	if err := src.Healthy(t.Context()); err != nil {
		t.Fatalf("expected scraper url to be healthy: %v", err)
	}
	src = NewSource(config.Config{YouTubeAPIKey: "api-key"})
	if err := src.Healthy(t.Context()); err != nil {
		t.Fatalf("expected api key to be healthy: %v", err)
	}
	src = NewSource(config.Config{})
	if err := src.Healthy(t.Context()); err == nil {
		t.Fatal("expected missing providers to be unhealthy")
	}
}

func TestSearchAPIStoresMetadataAndRespectsBudget(t *testing.T) {
	var sawSearch bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawSearch = true
		q := r.URL.Query()
		if got := q.Get("key"); got != "api-key" {
			t.Fatalf("key = %q, want api-key", got)
		}
		if got := q.Get("type"); got != "video" {
			t.Fatalf("type = %q, want video", got)
		}
		if got := q.Get("maxResults"); got != "1" {
			t.Fatalf("maxResults = %q, want 1", got)
		}
		if got := q.Get("q"); got != "CaseOh clip" {
			t.Fatalf("q = %q, want CaseOh clip", got)
		}
		if got := q.Get("publishedAfter"); got != "2026-06-17T20:00:00Z" {
			t.Fatalf("publishedAfter = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": map[string]any{"videoId": "abc123def45"},
					"snippet": map[string]any{
						"title":        "CaseOh reacts to viral LSF clip",
						"channelTitle": "Clips Channel",
						"publishedAt":  "2026-06-18T01:02:03Z",
					},
				},
				{
					"id": map[string]any{"videoId": "second12345"},
					"snippet": map[string]any{
						"title":        "Second video should stay behind budget",
						"channelTitle": "Other Channel",
						"publishedAt":  "2026-06-18T01:03:03Z",
					},
				},
			},
		})
	}))
	defer server.Close()

	src := NewSource(config.Config{
		YouTubeAPIKey:       "api-key",
		YouTubeAPIBaseURL:   server.URL,
		SocialRetentionDays: 30,
	})
	page, err := src.Search(t.Context(), social.Query{
		Entity: social.EntityRef{TwitchLogin: "CaseOh"},
		Since:  time.Date(2026, 6, 17, 20, 0, 0, 0, time.UTC),
		Budget: social.Budget{MaxItems: 1},
	})
	if err != nil {
		t.Fatalf("search api: %v", err)
	}
	if !sawSearch {
		t.Fatal("expected API search request")
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(page.Items), page.Items)
	}
	item := page.Items[0]
	if item.Source != "youtube" || item.Kind != "video" {
		t.Fatalf("unexpected source/kind %+v", item)
	}
	if item.ExternalID != "abc123def45" {
		t.Fatalf("external id = %q", item.ExternalID)
	}
	if item.URL != "https://www.youtube.com/watch?v=abc123def45" {
		t.Fatalf("url = %q", item.URL)
	}
	if item.Author != "Clips Channel" || item.Text != "CaseOh reacts to viral LSF clip" {
		t.Fatalf("metadata not stored: %+v", item)
	}
	if item.EntityTwitchLogin != "caseoh" || item.EntityDisplayName != "CaseOh" {
		t.Fatalf("entity not normalized: %+v", item)
	}
	if item.Provenance.SourceAPI != "youtube_data_v3" || item.Provenance.HTTPStatus != http.StatusOK {
		t.Fatalf("provenance not recorded: %+v", item.Provenance)
	}
	if item.CreatedAt.Format(time.RFC3339) != "2026-06-18T01:02:03Z" {
		t.Fatalf("createdAt = %s", item.CreatedAt.Format(time.RFC3339))
	}
	if item.ExpiresAt.IsZero() {
		t.Fatal("expected retention expiry")
	}
	if len(item.SnapshotSHA256) == 0 {
		t.Fatal("expected snapshot hash")
	}
}

func TestSearchAPIQuotaFailureReturnsSourceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"reason":"quotaExceeded"}}`, http.StatusForbidden)
	}))
	defer server.Close()

	src := NewSource(config.Config{
		YouTubeAPIKey:     "api-key",
		YouTubeAPIBaseURL: server.URL,
	})
	_, err := src.Search(t.Context(), social.Query{
		Entity: social.EntityRef{TwitchLogin: "caseoh"},
		Budget: social.Budget{MaxItems: 4},
	})
	if err == nil {
		t.Fatal("expected quota/rate failure")
	}
	if !strings.Contains(err.Error(), "youtube status 403") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestSearchScrapeRespectsMaxBrowserFetches(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"html": `<!doctype html><script>var ytInitialData = {"contents":{"empty":true}};</script>`,
			},
		})
	}))
	defer server.Close()

	src := NewSource(config.Config{ScraperAPIURL: server.URL})
	_, err := src.Search(t.Context(), social.Query{
		Keywords: []string{"caseoh", "xqc"},
		Budget:   social.Budget{MaxItems: 4, MaxBrowserFetches: 1},
	})
	if err == nil {
		t.Fatal("expected scrape parse failure")
	}
	if calls != 1 {
		t.Fatalf("scraper calls = %d, want 1", calls)
	}
}

func TestSearchScrapeDisabledBudgetDoesNotCallScraper(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	src := NewSource(config.Config{ScraperAPIURL: server.URL})
	page, err := src.Search(t.Context(), social.Query{
		Keywords: []string{"caseoh"},
		Budget:   social.Budget{MaxItems: 4, MaxBrowserFetches: -1},
	})
	if err != nil {
		t.Fatalf("search scrape disabled: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("items = %+v, want empty", page.Items)
	}
	if calls != 0 {
		t.Fatalf("scraper calls = %d, want 0", calls)
	}
}
