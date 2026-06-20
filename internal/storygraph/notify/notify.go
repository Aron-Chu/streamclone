package notify

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// Publisher fans out story state changes (Phase 3).
type Publisher struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Publisher {
	return &Publisher{rdb: rdb}
}

// PublishStateChange emits a story update event to Redis for chat WS delivery.
func (p *Publisher) PublishStateChange(ctx context.Context, clusterID int64, event string) error {
	if p.rdb == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"type":       "pulse_wire_story",
		"clusterId":  clusterID,
		"event":      event,
	})
	return p.rdb.Publish(ctx, "pulse_wire:notify", payload).Err()
}
