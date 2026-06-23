package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ttBackoffKeyGlobalPrefix = "tt:scrape:backoff:global:"
	ttBackoffKeyStreamPrefix = "tt:scrape:backoff:stream:"
)

var errTTScrapeBackoff = fmt.Errorf("tt scrape backoff active")

func ttScrapeBackoffTTL(reason string) time.Duration {
	switch reason {
	case TTScrapeReasonCloudflareChallenge, TTScrapeReasonHTTP403:
		return 30 * time.Minute
	case TTScrapeReasonBrowserCrash:
		return 5 * time.Minute
	case TTScrapeReasonScraperUnreachable:
		return 2 * time.Minute
	case TTScrapeReasonTimeoutNavigation, TTScrapeReasonTimeoutHighcharts:
		return 10 * time.Minute
	default:
		return 5 * time.Minute
	}
}

func ttScrapeGlobalBackoffReasons() []string {
	return []string{
		TTScrapeReasonCloudflareChallenge,
		TTScrapeReasonHTTP403,
		TTScrapeReasonBrowserCrash,
		TTScrapeReasonScraperUnreachable,
	}
}

func isTTScrapeGlobalBackoffReason(reason string) bool {
	for _, r := range ttScrapeGlobalBackoffReasons() {
		if r == reason {
			return true
		}
	}
	return false
}

func (s *SyncService) checkTTScrapeBackoff(ctx context.Context, streamID string) (blocked bool, reason string) {
	if s == nil || !s.ttScrapeBackoffEnabled || s.rdb == nil {
		return false, ""
	}
	for _, r := range ttScrapeGlobalBackoffReasons() {
		key := ttBackoffKeyGlobalPrefix + r
		if n, err := s.rdb.Exists(ctx, key).Result(); err == nil && n > 0 {
			return true, r
		}
	}
	if streamID == "" {
		return false, ""
	}
	key := ttBackoffKeyStreamPrefix + streamID
	val, err := s.rdb.Get(ctx, key).Result()
	if err == nil && val != "" {
		return true, val
	}
	if err != nil && err != redis.Nil {
		s.log.Warn("tt scrape backoff lookup failed", "stream_id", streamID, "err", err)
	}
	return false, ""
}

func (s *SyncService) recordTTScrapeBackoff(ctx context.Context, streamID, reason string) {
	if s == nil || !s.ttScrapeBackoffEnabled || s.rdb == nil || reason == "" || reason == TTScrapeReasonOK {
		return
	}
	ttl := ttScrapeBackoffTTL(reason)
	if isTTScrapeGlobalBackoffReason(reason) {
		key := ttBackoffKeyGlobalPrefix + reason
		if err := s.rdb.Set(ctx, key, "1", ttl).Err(); err != nil {
			s.log.Warn("failed to set global tt scrape backoff", "reason", reason, "err", err)
		}
		return
	}
	if streamID == "" {
		return
	}
	key := ttBackoffKeyStreamPrefix + streamID
	if err := s.rdb.Set(ctx, key, reason, ttl).Err(); err != nil {
		s.log.Warn("failed to set stream tt scrape backoff", "stream_id", streamID, "reason", reason, "err", err)
	}
}
