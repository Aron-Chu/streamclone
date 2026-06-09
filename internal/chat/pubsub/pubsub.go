package pubsub

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Publisher interface {
	Publish(ctx context.Context, channel string, data []byte) error
}

type Subscriber interface {
	Subscribe(ctx context.Context, channel string, handler func([]byte)) (func(), error)
}

type Client struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
}

func chatKey(channel string) string {
	return fmt.Sprintf("chat:%s", channel)
}

func (c *Client) Publish(ctx context.Context, channel string, data []byte) error {
	return c.rdb.Publish(ctx, chatKey(channel), data).Err()
}

func (c *Client) Subscribe(ctx context.Context, channel string, handler func([]byte)) (func(), error) {
	sub := c.rdb.Subscribe(ctx, chatKey(channel))
	ch := sub.Channel()
	go func() {
		for msg := range ch {
			handler([]byte(msg.Payload))
		}
	}()
	return func() { _ = sub.Close() }, nil
}
