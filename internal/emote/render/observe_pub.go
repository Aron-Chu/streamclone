package render

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ObserveRedisMaxLen   = 4096
	ObserveSeenTTL       = 10 * time.Minute
	observePublishBudget = 250 * time.Millisecond
	localObserveDedupe   = 30 * time.Second
)

// ObservePublisher dedupes and publishes chat-observed render hints without blocking chat ingestion.
type ObservePublisher struct {
	rdb          *redis.Client
	log          *slog.Logger
	mu           sync.Mutex
	localSeen    map[string]time.Time
	dedupeWindow time.Duration
}

func NewObservePublisher(rdb *redis.Client, log *slog.Logger) *ObservePublisher {
	return &ObservePublisher{
		rdb:          rdb,
		log:          log,
		localSeen:    make(map[string]time.Time),
		dedupeWindow: localObserveDedupe,
	}
}

func (p *ObservePublisher) TryPublish(payload ObservePayload) {
	if p == nil {
		return
	}
	if err := payload.validate(); err != nil {
		return
	}
	scale := strings.TrimSpace(payload.Scale)
	if scale == "" {
		scale = "1x"
	}
	payload.Scale = scale
	key := payload.EmoteID + ":" + scale
	now := time.Now()

	p.mu.Lock()
	if seenAt, ok := p.localSeen[key]; ok && now.Sub(seenAt) < p.dedupeWindow {
		p.mu.Unlock()
		return
	}
	p.localSeen[key] = now
	if len(p.localSeen) > 8192 {
		for k, ts := range p.localSeen {
			if now.Sub(ts) > p.dedupeWindow {
				delete(p.localSeen, k)
			}
		}
	}
	p.mu.Unlock()

	if p.rdb == nil {
		return
	}
	go p.publish(payload)
}

func (p *ObservePublisher) publish(payload ObservePayload) {
	ctx, cancel := context.WithTimeout(context.Background(), observePublishBudget)
	defer cancel()
	if err := PublishObserveDeduped(ctx, p.rdb, payload); err != nil && p.log != nil {
		p.log.Debug("emote render observe publish skipped", "emote_id", payload.EmoteID, "err", err)
	}
}

func observeSeenKey(emoteID, scale string) string {
	return fmt.Sprintf("emote:render:seen:%s:%s", emoteID, scale)
}

// PublishObserveDeduped enqueues a chat-observed hint with Redis SETNX dedupe and bounded list length.
func PublishObserveDeduped(ctx context.Context, rdb *redis.Client, payload ObservePayload) error {
	if rdb == nil {
		return nil
	}
	if err := payload.validate(); err != nil {
		return err
	}
	scale := strings.TrimSpace(payload.Scale)
	if scale == "" {
		scale = "1x"
	}
	ok, err := rdb.SetNX(ctx, observeSeenKey(payload.EmoteID, scale), "1", ObserveSeenTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	pipe := rdb.Pipeline()
	pipe.RPush(ctx, ObserveRedisKey, raw)
	pipe.LTrim(ctx, ObserveRedisKey, -ObserveRedisMaxLen, -1)
	_, err = pipe.Exec(ctx)
	return err
}
