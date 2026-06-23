package analytics

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifyTTScrapeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: TTScrapeReasonOK},
		{name: "access protected", err: fmt.Errorf("%w: cloudflare", errTrackerAccessProtected), want: TTScrapeReasonCloudflareChallenge},
		{name: "cf block string", err: errors.New("scraper scrape failed: cloudflare_challenge_or_block"), want: TTScrapeReasonCloudflareChallenge},
		{name: "deadline", err: errors.New(`Post "http://scraper:8000/v2/scrape": context deadline exceeded`), want: TTScrapeReasonTimeoutNavigation},
		{name: "goto timeout", err: errors.New("scraper scrape failed: Page.goto: Timeout 120000ms exceeded"), want: TTScrapeReasonTimeoutNavigation},
		{name: "403 direct", err: errors.New("status 403"), want: TTScrapeReasonHTTP403},
		{name: "missing chart", err: errors.New("scraper scrape failed: TwitchTracker stream page blocked or incomplete (missing viewer chart)"), want: TTScrapeReasonMissingMetaECS},
		{name: "browser crash", err: errors.New("scraper scrape failed: BrowserContext.new_page: Target page, context or browser has been closed"), want: TTScrapeReasonBrowserCrash},
		{name: "window null", err: errors.New("BrowserContext.new_page: Protocol error (Browser.newPage): window is null"), want: TTScrapeReasonBrowserCrash},
		{name: "connection refused", err: errors.New(`dial tcp 172.18.0.6:8000: connect: connection refused`), want: TTScrapeReasonScraperUnreachable},
		{name: "scraper api 502", err: errors.New("scraper API returned status 502: bad gateway"), want: TTScrapeReasonScraperAPIError},
		{name: "empty html", err: errors.New("scraper scrape returned empty html"), want: TTScrapeReasonEmptyHTML},
		{name: "proxy", err: errors.New("proxy authentication failed"), want: TTScrapeReasonProxyError},
		{name: "backoff", err: fmt.Errorf("%w (cloudflare_challenge)", errTTScrapeBackoff), want: TTScrapeReasonScrapeBackoff},
		{name: "other", err: errors.New("something unexpected"), want: TTScrapeReasonOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTTScrapeError(tc.err); got != tc.want {
				t.Fatalf("ClassifyTTScrapeError() = %q, want %q", got, tc.want)
			}
		})
	}
}
