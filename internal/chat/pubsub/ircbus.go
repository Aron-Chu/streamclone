package pubsub

import (
	"context"
	"fmt"
)

// IRCBusChannel is the Redis pub/sub channel for consolidated IRC line fan-out.
// Phase 1 stub: helpers only; chat and analytics still use separate ircconn pools.
const IRCBusChannel = "irc:lines"

// PublishIRCLine publishes a raw Twitch IRC line to the consolidation bus.
// Future consumers: analytics collector, clipper spike detector (replacing duplicate pools).
func (c *Client) PublishIRCLine(ctx context.Context, line string) error {
	return c.rdb.Publish(ctx, IRCBusChannel, line).Err()
}

// SubscribeIRCLines listens for raw IRC lines on the consolidation bus.
func (c *Client) SubscribeIRCLines(ctx context.Context, handler func(string)) (func(), error) {
	sub := c.rdb.Subscribe(ctx, IRCBusChannel)
	ch := sub.Channel()
	go func() {
		for msg := range ch {
			handler(msg.Payload)
		}
	}()
	return func() { _ = sub.Close() }, nil
}

func IRCBusKey(channel string) string {
	return fmt.Sprintf("%s:%s", IRCBusChannel, channel)
}

// PublishIRCLineForChannel is a per-channel bus variant for selective subscriptions.
func (c *Client) PublishIRCLineForChannel(ctx context.Context, channel, line string) error {
	return c.rdb.Publish(ctx, IRCBusKey(channel), line).Err()
}

// SubscribeIRCLinesForChannel listens on the per-channel IRC bus.
func (c *Client) SubscribeIRCLinesForChannel(ctx context.Context, channel string, handler func(string)) (func(), error) {
	sub := c.rdb.Subscribe(ctx, IRCBusKey(channel))
	ch := sub.Channel()
	go func() {
		for msg := range ch {
			handler(msg.Payload)
		}
	}()
	return func() { _ = sub.Close() }, nil
}

// IRCBusPublisher is the narrow interface for future single-pool IRC publishers.
type IRCBusPublisher interface {
	PublishIRCLine(ctx context.Context, line string) error
}

var _ IRCBusPublisher = (*Client)(nil)
