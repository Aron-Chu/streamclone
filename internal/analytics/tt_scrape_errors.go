package analytics

import (
	"errors"
	"strings"
)

// TT scrape outcome reason labels (Prometheus analytics_tt_scrape_* metrics).
const (
	TTScrapeReasonOK                 = "ok"
	TTScrapeReasonCloudflareChallenge = "cloudflare_challenge"
	TTScrapeReasonTimeoutNavigation  = "timeout_navigation"
	TTScrapeReasonTimeoutHighcharts  = "timeout_highcharts"
	TTScrapeReasonMissingMetaECS     = "missing_meta_ecs"
	TTScrapeReasonHTTP403            = "http_403"
	TTScrapeReasonHTTP429            = "http_429"
	TTScrapeReasonEmptyViewerSeries  = "empty_viewer_series"
	TTScrapeReasonPartialChart       = "partial_chart"
	TTScrapeReasonFlatTail           = "flat_tail"
	TTScrapeReasonParseError         = "parse_error"
	TTScrapeReasonBrowserCrash       = "browser_crash"
	TTScrapeReasonProxyError         = "proxy_error"
	TTScrapeReasonScraperUnreachable = "scraper_unreachable"
	TTScrapeReasonScraperAPIError    = "scraper_api_error"
	TTScrapeReasonEmptyHTML          = "empty_html"
	TTScrapeReasonScrapeBackoff      = "scrape_backoff"
	TTScrapeReasonOther              = "other"
)

const (
	ttScrapePathDirectHTTP = "direct_http"
	ttScrapePathBrowser    = "browser"
	ttScrapePathUnknown    = "unknown"
)

// ClassifyTTScrapeError maps sync/scraper errors to a stable failure reason for metrics.
func ClassifyTTScrapeError(err error) string {
	if err == nil {
		return TTScrapeReasonOK
	}
	if errors.Is(err, errTrackerAccessProtected) {
		return TTScrapeReasonCloudflareChallenge
	}
	msg := strings.ToLower(err.Error())

	switch {
	case errors.Is(err, errTTScrapeBackoff),
		strings.Contains(msg, "tt scrape backoff active"):
		return TTScrapeReasonScrapeBackoff
	case strings.Contains(msg, "cloudflare_challenge_or_block"),
		strings.Contains(msg, "cloudflare challenge"),
		strings.Contains(msg, "access_protected"),
		strings.Contains(msg, "tracker access protected"):
		return TTScrapeReasonCloudflareChallenge
	case strings.Contains(msg, "highcharts") && strings.Contains(msg, "timeout"):
		return TTScrapeReasonTimeoutHighcharts
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "page.goto: timeout"),
		strings.Contains(msg, "client.timeout exceeded"):
		return TTScrapeReasonTimeoutNavigation
	case strings.Contains(msg, "status 403"), strings.Contains(msg, "returned status: 403"):
		return TTScrapeReasonHTTP403
	case strings.Contains(msg, "status 429"), strings.Contains(msg, "returned status: 429"):
		return TTScrapeReasonHTTP429
	case strings.Contains(msg, "missing viewer chart"),
		strings.Contains(msg, "blocked or incomplete"),
		strings.Contains(msg, "missing meta#ecs"),
		strings.Contains(msg, "did not contain twitchtracker stream data"):
		return TTScrapeReasonMissingMetaECS
	case strings.Contains(msg, "empty viewer"),
		strings.Contains(msg, "no viewer points"):
		return TTScrapeReasonEmptyViewerSeries
	case strings.Contains(msg, "partial chart"),
		strings.Contains(msg, "chart coverage"):
		return TTScrapeReasonPartialChart
	case strings.Contains(msg, "flat tail"),
		strings.Contains(msg, "flat chart"):
		return TTScrapeReasonFlatTail
	case strings.Contains(msg, "browser has been closed"),
		strings.Contains(msg, "window is null"),
		strings.Contains(msg, "context or browser has been closed"),
		strings.Contains(msg, "execution context was destroyed"),
		strings.Contains(msg, "browsercontext.new_page"):
		return TTScrapeReasonBrowserCrash
	case strings.Contains(msg, "proxy"):
		return TTScrapeReasonProxyError
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "scraper service unreachable"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "actively refused"),
		strings.Contains(msg, " post \"http://scraper:") && strings.Contains(msg, "eof"):
		return TTScrapeReasonScraperUnreachable
	case strings.Contains(msg, "scraper api returned status"),
		strings.Contains(msg, "scraper scrape failed"):
		return TTScrapeReasonScraperAPIError
	case strings.Contains(msg, "scrape returned empty html"),
		strings.Contains(msg, "empty html"):
		return TTScrapeReasonEmptyHTML
	case strings.Contains(msg, "parse"),
		strings.Contains(msg, "twitchtracker parse"):
		return TTScrapeReasonParseError
	default:
		return TTScrapeReasonOther
	}
}
