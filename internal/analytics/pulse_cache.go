package analytics

import (
	"context"
	"log/slog"
	"strings"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics/heatmap"
)

// InvalidatePulseBFFCache clears the public Pulse BFF cache for a login.
func InvalidatePulseBFFCache(ctx context.Context, rdb *redis.Client, login string, log *slog.Logger) {
	login = normalizeLogin(login)
	if rdb != nil && login != "" {
		if err := rdb.Del(ctx, extPulseCachePrefix+login).Err(); err != nil && log != nil {
			log.Warn("pulse bff cache invalidation failed", "login", login, "err", err)
		}
	}
}

// InvalidatePulseHeatmapCache clears heatmap rollup cache for a stream.
func InvalidatePulseHeatmapCache(ctx context.Context, heatmapCache *heatmap.Cache, streamID string, log *slog.Logger) {
	streamID = strings.TrimSpace(streamID)
	if heatmapCache != nil && streamID != "" {
		heatmapCache.Invalidate(ctx, streamID)
	}
}

// InvalidatePortalGamesCache clears the portal /games response cache for a stream.
func InvalidatePortalGamesCache(ctx context.Context, rdb *redis.Client, streamID string, log *slog.Logger) {
	streamID = strings.TrimSpace(streamID)
	if rdb != nil && streamID != "" {
		key := portalAnalyticsCachePrefix + "games:" + streamID
		if err := rdb.Del(ctx, key).Err(); err != nil && log != nil {
			log.Warn("portal games cache invalidation failed", "stream_id", streamID, "err", err)
		}
	}
}

// InvalidatePulseCaches clears public Pulse/BFF and heatmap caches after rollup writes.
// Cache invalidation is best-effort so Redis never becomes required for live collection.
func InvalidatePulseCaches(ctx context.Context, rdb *redis.Client, heatmapCache *heatmap.Cache, login, streamID string, log *slog.Logger) {
	InvalidatePulseBFFCache(ctx, rdb, login, log)
	InvalidatePulseHeatmapCache(ctx, heatmapCache, streamID, log)
}
