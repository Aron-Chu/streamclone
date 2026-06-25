package analytics

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestResolveSilverBackfillOutcomeScrapeBackoffDefersWithoutAttemptBurn(t *testing.T) {
	now := time.Date(2026, 6, 25, 4, 0, 0, 0, time.UTC)
	job := BackfillJob{Attempt: 2, ExportStatus: "pending", Tier: "silver"}
	syncErr := fmt.Errorf("%w (%s)", errTTScrapeBackoff, TTScrapeReasonScraperUnreachable)
	got := resolveSilverBackfillOutcome(job, syncErr, now)
	if got.status != "queued" || !got.requeue {
		t.Fatalf("backoff should requeue, got %+v", got)
	}
	if got.attempt != 2 {
		t.Fatalf("attempt = %d, want 2 (no burn during global backoff)", got.attempt)
	}
	want := now.Add(2 * time.Minute)
	if !got.nextRunAt.Equal(want) {
		t.Fatalf("next_run_at = %v, want %v", got.nextRunAt, want)
	}
	if !strings.Contains(got.errMsg, "tt_reason=scrape_backoff") {
		t.Fatalf("errMsg = %q, want tt_reason tag", got.errMsg)
	}
}

func TestResolveSilverBackfillOutcomeScraperUnreachableBoundedRetry(t *testing.T) {
	now := time.Now()
	errUnreachable := errors.New(`Post "http://scraper:8000/v2/scrape": dial tcp 172.18.0.6:8000: connect: connection refused`)

	job := BackfillJob{Attempt: 0, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, errUnreachable, now)
	if got.status != "queued" || !got.requeue || got.attempt != 1 {
		t.Fatalf("first transport failure should requeue once, got %+v", got)
	}
	if !strings.Contains(got.errMsg, "tt_reason=scraper_unreachable") {
		t.Fatalf("errMsg = %q", got.errMsg)
	}

	job.Attempt = maxSilverScrapeAttempts - 1
	got = resolveSilverBackfillOutcome(job, errUnreachable, now)
	if got.status != "failed" || got.requeue {
		t.Fatalf("final transport attempt should terminal fail, got %+v", got)
	}
	if !strings.Contains(got.errMsg, "terminal reason=scraper_unreachable") {
		t.Fatalf("errMsg = %q", got.errMsg)
	}
}

func TestResolveSilverBackfillOutcomeBrowserCrashBoundedRetry(t *testing.T) {
	now := time.Now()
	errCrash := errors.New("scraper scrape failed: BrowserContext.new_page: window is null")
	job := BackfillJob{Attempt: maxSilverScrapeAttempts - 1, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, errCrash, now)
	if got.status != "failed" || got.requeue {
		t.Fatalf("browser crash at max attempts should fail, got %+v", got)
	}
	if classifyBackfillScrapeFailure(errCrash) != TTScrapeReasonBrowserCrash {
		t.Fatalf("classification = %q", classifyBackfillScrapeFailure(errCrash))
	}
}

func TestResolveSilverBackfillOutcomeTimeoutNavigationRetriable(t *testing.T) {
	now := time.Now()
	errTimeout := errors.New("scraper scrape failed: Page.goto: Timeout 120000ms exceeded")
	job := BackfillJob{Attempt: 0, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, errTimeout, now)
	if got.status != "queued" || !got.requeue {
		t.Fatalf("navigation timeout should requeue, got %+v", got)
	}
	if classifyBackfillScrapeFailure(errTimeout) != TTScrapeReasonTimeoutNavigation {
		t.Fatalf("classification = %q", classifyBackfillScrapeFailure(errTimeout))
	}

	job.Attempt = 1
	got = resolveSilverBackfillOutcome(job, errTimeout, now)
	if got.status != "queued" || got.attempt != 2 {
		t.Fatalf("second timeout should requeue, got %+v", got)
	}
}

func TestResolveSilverBackfillOutcomeHTTP403SeparateFromTransport(t *testing.T) {
	err403 := errors.New("status 403")
	if !isSilverSourceBlockReason(TTScrapeReasonHTTP403) {
		t.Fatal("403 should be source block")
	}
	if isSilverTransportReason(TTScrapeReasonHTTP403) {
		t.Fatal("403 should not be transport")
	}
	job := BackfillJob{Attempt: 1, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, err403, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("403 at attempt 2 should terminal fail, got %+v", got)
	}
}

func TestResolveSilverBackfillOutcomeSuccessUnchanged(t *testing.T) {
	job := BackfillJob{Attempt: 0, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, nil, time.Now())
	if got.status != "done" || got.requeue {
		t.Fatalf("success = %+v", got)
	}
}

func TestResolveSilverBackfillOutcomeNonRetriableFailsImmediately(t *testing.T) {
	errParse := errors.New("twitchtracker parse failed: bad json")
	job := BackfillJob{Attempt: 0, ExportStatus: "pending", Tier: "silver"}
	got := resolveSilverBackfillOutcome(job, errParse, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("parse error should fail immediately, got %+v", got)
	}
}

func TestGoldTierStillUsesGenericOutcome(t *testing.T) {
	job := BackfillJob{Attempt: 0, ExportStatus: "pending", Tier: "gold"}
	syncErr := errors.New("tracker access protected")
	got := resolveBackfillOutcome(job, syncErr, time.Now())
	if got.status != "failed" || got.requeue {
		t.Fatalf("gold non-timeout should fail immediately, got %+v", got)
	}
}

func TestScrapeBackoffNestedReason(t *testing.T) {
	err := fmt.Errorf("%w (%s)", errTTScrapeBackoff, TTScrapeReasonBrowserCrash)
	if got := scrapeBackoffNestedReason(err); got != TTScrapeReasonBrowserCrash {
		t.Fatalf("nested = %q", got)
	}
}
