package analytics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	pulseRLWatchPrefix    = "sp:rl:watch:"
	pulseRLBackfillPrefix = "sp:rl:backfill:"
	pulseRLSummaryPrefix  = pulseSummaryRLKeyPrefix
)

// PulseRateLimiter applies Redis fixed-window counters for hosted principals.
type PulseRateLimiter struct {
	rdb             *redis.Client
	watchPerMin     int
	backfillPerHour int
	summaryPerMin   int
	testAllowFn     func(key string, limit int) int64
}

func NewPulseRateLimiter(rdb *redis.Client, watchPerMin, backfillPerHour int) *PulseRateLimiter {
	return &PulseRateLimiter{
		rdb:             rdb,
		watchPerMin:     watchPerMin,
		backfillPerHour: backfillPerHour,
		summaryPerMin:   pulseSummaryRateLimitPerMin,
	}
}

func (h *Handler) WithRateLimiter(l *PulseRateLimiter) *Handler {
	h.rateLimiter = l
	return h
}

func (l *PulseRateLimiter) AllowWatch(ctx context.Context, principalID string) (bool, time.Duration) {
	return l.allow(ctx, pulseRLWatchPrefix+principalID, l.watchPerMin, time.Minute)
}

func (l *PulseRateLimiter) AllowBackfill(ctx context.Context, principalID string) (bool, time.Duration) {
	return l.allow(ctx, pulseRLBackfillPrefix+principalID, l.backfillPerHour, time.Hour)
}

func (l *PulseRateLimiter) AllowSummary(ctx context.Context, principalID string) (bool, time.Duration) {
	limit := l.summaryPerMin
	if limit <= 0 {
		limit = pulseSummaryRateLimitPerMin
	}
	return l.allow(ctx, pulseRLSummaryPrefix+principalID, limit, time.Minute)
}

func (l *PulseRateLimiter) AllowPortalSummary(ctx context.Context, principalID string) (bool, time.Duration) {
	return l.allow(ctx, portalAnalyticsRLPrefix+principalID, portalAnalyticsRLPerMin, time.Minute)
}

func rateLimitExceeded(count int64, limit int) bool {
	return limit > 0 && count > int64(limit)
}

func (l *PulseRateLimiter) allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration) {
	if l == nil || limit <= 0 {
		return true, 0
	}
	if l.testAllowFn != nil {
		if rateLimitExceeded(l.testAllowFn(key, limit), limit) {
			return false, window
		}
		return true, 0
	}
	if l.rdb == nil {
		return true, 0
	}
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true, 0
	}
	if count == 1 {
		_ = l.rdb.Expire(ctx, key, window).Err()
	}
	if count > int64(limit) {
		ttl, _ := l.rdb.TTL(ctx, key).Result()
		if ttl <= 0 {
			ttl = window
		}
		return false, ttl
	}
	return true, 0
}

func (h *Handler) enforceWatchRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if h.rateLimiter == nil || !h.pulseHosted.Hosted {
		return true
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		return true
	}
	allowed, retryAfter := h.rateLimiter.AllowWatch(r.Context(), principal.ID)
	if allowed {
		return true
	}
	if retryAfter <= 0 {
		retryAfter = time.Minute
	}
	seconds := int(retryAfter.Seconds() + 0.999)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":             "rate_limited",
		"hint":              fmt.Sprintf("Watch limit exceeded; retry in %ds", seconds),
		"scope":             "watch",
		"retryAfterSeconds": seconds,
	})
	return false
}

func (h *Handler) enforceBackfillRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if h.rateLimiter == nil || !h.pulseHosted.Hosted {
		return true
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		return true
	}
	allowed, retryAfter := h.rateLimiter.AllowBackfill(r.Context(), principal.ID)
	if allowed {
		return true
	}
	if retryAfter <= 0 {
		retryAfter = time.Hour
	}
	seconds := int(retryAfter.Seconds() + 0.999)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":             "rate_limited",
		"hint":              fmt.Sprintf("Backfill limit exceeded; retry in %ds", seconds),
		"scope":             "backfill",
		"retryAfterSeconds": seconds,
	})
	return false
}
