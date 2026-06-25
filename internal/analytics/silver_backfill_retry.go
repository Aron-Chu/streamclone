package analytics

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxSilverScrapeAttempts = 3

// resolveSilverBackfillOutcome applies bounded TT scrape retry policy for silver tier jobs.
// It prevents infinite timeout/transport loops while deferring work during active global backoff.
func resolveSilverBackfillOutcome(job BackfillJob, syncErr error, now time.Time) backfillOutcome {
	if syncErr == nil {
		return resolveBackfillOutcome(job, nil, now)
	}

	reason := ClassifyTTScrapeError(syncErr)
	nextAttempt := job.Attempt + 1
	errMsg := annotateSilverScrapeError(reason, syncErr.Error())

	if reason == TTScrapeReasonScrapeBackoff {
		delay := silverScrapeDeferDelay(syncErr, reason)
		return backfillOutcome{
			status:       "queued",
			exportStatus: job.ExportStatus,
			errMsg:       errMsg,
			attempt:      job.Attempt,
			nextRunAt:    now.Add(delay),
			requeue:      true,
		}
	}

	if !isSilverScrapeRetriableReason(reason) {
		return backfillOutcome{
			status:       "failed",
			exportStatus: "failed",
			errMsg:       errMsg,
			attempt:      nextAttempt,
		}
	}

	maxAttempts := maxSilverScrapeAttempts
	if isSilverSourceBlockReason(reason) {
		maxAttempts = 2
	}

	if nextAttempt >= maxAttempts {
		return backfillOutcome{
			status:       "failed",
			exportStatus: "failed",
			errMsg:       fmt.Sprintf("%s [terminal reason=%s attempts=%d]", errMsg, reason, nextAttempt),
			attempt:      nextAttempt,
		}
	}

	return backfillOutcome{
		status:       "queued",
		exportStatus: job.ExportStatus,
		errMsg:       errMsg,
		attempt:      nextAttempt,
		nextRunAt:    now.Add(silverScrapeRetryDelay(reason, nextAttempt)),
		requeue:      true,
	}
}

func annotateSilverScrapeError(reason, msg string) string {
	if msg == "" {
		return fmt.Sprintf("[tt_reason=%s]", reason)
	}
	if strings.Contains(msg, "tt_reason=") {
		return msg
	}
	return fmt.Sprintf("%s [tt_reason=%s]", msg, reason)
}

func isSilverScrapeRetriableReason(reason string) bool {
	switch reason {
	case TTScrapeReasonScraperUnreachable,
		TTScrapeReasonBrowserCrash,
		TTScrapeReasonTimeoutNavigation,
		TTScrapeReasonTimeoutHighcharts,
		TTScrapeReasonScraperAPIError,
		TTScrapeReasonOther:
		return true
	case TTScrapeReasonHTTP403,
		TTScrapeReasonCloudflareChallenge,
		TTScrapeReasonHTTP429:
		// Source/content blocks use shorter bounded retry; not the same as transport wedge.
		return true
	default:
		return false
	}
}

func silverScrapeRetryDelay(reason string, attempt int) time.Duration {
	base := ttScrapeBackoffTTL(reason)
	if base <= 0 {
		base = 5 * time.Minute
	}
	// Cap exponential growth for repeated transport failures on the same job.
	mult := attempt
	if mult < 1 {
		mult = 1
	}
	if mult > 3 {
		mult = 3
	}
	delay := time.Duration(mult) * base
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func silverScrapeDeferDelay(syncErr error, reason string) time.Duration {
	if nested := scrapeBackoffNestedReason(syncErr); nested != "" {
		return ttScrapeBackoffTTL(nested)
	}
	return ttScrapeBackoffTTL(reason)
}

func scrapeBackoffNestedReason(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const prefix = "tt scrape backoff active ("
	if !strings.Contains(msg, prefix) {
		return ""
	}
	start := strings.Index(msg, prefix)
	if start < 0 {
		return ""
	}
	rest := msg[start+len(prefix):]
	end := strings.Index(rest, ")")
	if end <= 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func isSilverTransportReason(reason string) bool {
	switch reason {
	case TTScrapeReasonScraperUnreachable, TTScrapeReasonBrowserCrash:
		return true
	default:
		return false
	}
}

func isSilverSourceBlockReason(reason string) bool {
	switch reason {
	case TTScrapeReasonHTTP403, TTScrapeReasonCloudflareChallenge, TTScrapeReasonHTTP429:
		return true
	default:
		return false
	}
}

// classifyBackfillScrapeFailure exposes TT scrape classification for tests and metrics.
func classifyBackfillScrapeFailure(err error) string {
	if err == nil {
		return TTScrapeReasonOK
	}
	if errors.Is(err, errTTScrapeBackoff) {
		return TTScrapeReasonScrapeBackoff
	}
	return ClassifyTTScrapeError(err)
}
