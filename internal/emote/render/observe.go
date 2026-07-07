package render

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// StartObserveConsumer drains chat-observed render hints from Redis.
func StartObserveConsumer(ctx context.Context, rdb *redis.Client, q *Queue, log *slog.Logger) {
	if rdb == nil || q == nil {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			result, err := rdb.BLPop(ctx, 2*time.Second, ObserveRedisKey).Result()
			if err != nil {
				if err == redis.Nil || ctx.Err() != nil {
					continue
				}
				if log != nil {
					log.Warn("emote render observe pop failed", "err", err)
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if len(result) < 2 {
				continue
			}
			var payload ObservePayload
			if err := json.Unmarshal([]byte(result[1]), &payload); err != nil {
				if log != nil {
					log.Warn("emote render observe decode failed", "err", err)
				}
				continue
			}
			handleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := q.HandleObservePayload(handleCtx, payload); err != nil && log != nil {
				log.Warn("emote render observe enqueue failed", "emote_id", payload.EmoteID, "err", err)
			}
			cancel()
		}
	}()
}

// PublishObserve pushes a chat-observed render hint for the emote service to consume.
func PublishObserve(ctx context.Context, rdb *redis.Client, payload ObservePayload) error {
	return PublishObserveDeduped(ctx, rdb, payload)
}
