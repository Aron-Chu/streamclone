package streamerbans

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/social"
)

func TestParseBanPost(t *testing.T) {
	login, headline, ok := ParseBanPost(`Twitch Partner "xqc" has been banned`)
	if !ok || login != "xqc" || !strings.Contains(headline, "xqc") {
		t.Fatalf("parse failed: ok=%v login=%q headline=%q", ok, login, headline)
	}
	_, _, ok = ParseBanPost("random tweet text")
	if ok {
		t.Fatal("expected non-ban text to fail")
	}
}

func TestSearchSidecar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			return
		case "/users/StreamerBans/timeline":
			_, _ = w.Write([]byte(`{"items":[{"id":"123","text":"Twitch Partner \"caseoh\" has been banned","url":"https://x.com/StreamerBans/status/123","createdAt":"2026-06-17T12:00:00Z"}]}`))
			return
		default:
			// Tier 1 HTML must not return live bans; force empty parse so emusks tier is exercised.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		}
	}))
	defer srv.Close()

	cfg := config.Config{
		StreamerbansIngestEnabled: true,
		StreamerbansHomeURL:       srv.URL,
		XUnofficialOK:             true,
		XAuthToken:                "test-token",
		XIngestURL:                srv.URL,
		SocialRetentionDays:       90,
	}
	src := NewSource(cfg)
	if err := src.Healthy(context.Background()); err != nil {
		t.Fatalf("healthy: %v", err)
	}
	page, err := src.Search(context.Background(), social.Query{
		Since:  time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
		Budget: social.Budget{MaxItems: 8},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].EntityTwitchLogin != "caseoh" {
		t.Fatalf("login=%q", page.Items[0].EntityTwitchLogin)
	}
	if page.Items[0].FlairText != "ban" {
		t.Fatalf("flair=%q", page.Items[0].FlairText)
	}
}

func TestParseBanLinesFromHTML(t *testing.T) {
	html := `<html><body>Twitch Partner "ninja" has been banned on streamerbans.com/user/ninja</body></html>`
	lines := parseBanLinesFromHTML(html, "test", http.StatusOK)
	if len(lines) == 0 {
		t.Fatal("expected parsed ban line")
	}
	login, _, ok := ParseBan(lines[0].Text)
	if !ok || login != "ninja" {
		t.Fatalf("login=%q ok=%v", login, ok)
	}
}

func TestParseNextDataFeed(t *testing.T) {
	html := `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"feed":[{"id":"a2109e68-9a2b-41b6-b548-bce55698b299","is_ban":true,"created_at":"2026-06-20T02:30:08.000000Z","suspensionable":{"login_name":"livesdoluan","display_name":"livesdoluan","profile_image_url":"https://static-cdn.jtvnw.net/jtv_user_pictures/livesdoluan-profile_image-abc-300x300.png"}}]}}}</script></html>`
	lines := parseNextDataFeed(html)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	login, _, ok := ParseBan(lines[0].Text)
	if !ok || login != "livesdoluan" {
		t.Fatalf("login=%q ok=%v", login, ok)
	}
	if lines[0].ProfileImageURL == "" {
		t.Fatal("expected profile image url")
	}
	if !strings.Contains(lines[0].Text, "livesdoluan") {
		t.Fatalf("headline=%q", lines[0].Text)
	}
	if lines[0].SourceAPI != "streamerbans.com/next-data" {
		t.Fatalf("source=%q", lines[0].SourceAPI)
	}
	if lines[0].URL != "https://streamerbans.com/user/livesdoluan" {
		t.Fatalf("url=%q", lines[0].URL)
	}
}

func TestParseNextDataFeedSkipsUnbans(t *testing.T) {
	html := `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"feed":[{"id":"99","is_ban":false,"created_at":"2026-06-17T12:00:00Z","suspensionable":{"login_name":"ninja","display_name":"Ninja"}}]}}}</script></html>`
	if len(parseNextDataFeed(html)) != 0 {
		t.Fatal("expected unban rows to be skipped for ban ingest")
	}
}

func TestFetchScrapedTimelineRespectsBrowserBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"success":true,"data":{"html":"Twitch Partner \"ninja\" has been banned"}}`))
	}))
	defer srv.Close()

	src := NewSource(config.Config{ScraperAPIURL: srv.URL, ScraperAPIKey: "test-key"})
	browserFetches := 0
	lines, err := src.fetchScrapedTimeline(context.Background(), 1, &browserFetches)
	if err != nil {
		t.Fatalf("fetch scraped timeline: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	if _, err := src.fetchScrapedTimeline(context.Background(), 1, &browserFetches); err == nil {
		t.Fatal("expected browser fetch budget exhaustion")
	}
	if calls != 1 {
		t.Fatalf("scraper calls = %d, want 1", calls)
	}
}

func TestFetchScrapedTimelineDisabledBudgetDoesNotCallScraper(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	src := NewSource(config.Config{ScraperAPIURL: srv.URL, ScraperAPIKey: "test-key"})
	browserFetches := 0
	if _, err := src.fetchScrapedTimeline(context.Background(), -1, &browserFetches); err == nil {
		t.Fatal("expected browser fetch budget exhaustion")
	}
	if calls != 0 {
		t.Fatalf("scraper calls = %d, want 0", calls)
	}
}
