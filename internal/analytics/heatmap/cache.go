package heatmap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const DefaultCacheTTL = 1 * time.Hour

func CacheKey(streamID string, version string, updatedAtMs int64, window int) string {
	return fmt.Sprintf("heatmap:%s:%s:%d:%d", streamID, version, updatedAtMs, window)
}

type Cache struct {
	rdb    *redis.Client
	logger *slog.Logger
}

func NewCache(rdb *redis.Client, logger *slog.Logger) *Cache {
	if rdb == nil {
		return nil
	}
	return &Cache{rdb: rdb, logger: logger}
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	val, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			c.logger.Warn("heatmap cache get failed", "key", key, "error", err)
		}
		return nil, false
	}
	return val, true
}

func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) {
	if c == nil {
		return
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		c.logger.Warn("heatmap cache set failed", "key", key, "error", err)
	}
}

func (c *Cache) Invalidate(ctx context.Context, streamID string) {
	if c == nil {
		return
	}
	pattern := fmt.Sprintf("heatmap:%s:*", streamID)
	var cursor uint64
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			c.logger.Warn("heatmap cache invalidation scan failed", "pattern", pattern, "error", err)
			return
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				c.logger.Warn("heatmap cache invalidation delete failed", "keys", len(keys), "error", err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}
