package dict

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Entry struct {
	URL       string `json:"u"`
	ZeroWidth bool   `json:"zw"`
	ID        string `json:"id,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

type Dict struct {
	rdb     *redis.Client
	cdnBase string
}

func New(rdb *redis.Client, cdnBase string) *Dict {
	return &Dict{rdb: rdb, cdnBase: cdnBase}
}

func (d *Dict) Ping(ctx context.Context) error {
	return d.rdb.Ping(ctx).Err()
}

func channelKey(login string) string {
	return fmt.Sprintf("channel:emotes:%s", login)
}

func deltaChannel(channel string) string {
	return fmt.Sprintf("emotes:delta:%s", channel)
}

func (d *Dict) EmoteURL(emoteID, scale string) string {
	return fmt.Sprintf("%s/%s/%s.webp", d.cdnBase, emoteID, scale)
}

func (d *Dict) Rebuild(ctx context.Context, login string, emotes []EmoteEntry) error {
	key := channelKey(login)
	pipe := d.rdb.Pipeline()
	pipe.Del(ctx, key)
	for _, e := range emotes {
		val, err := marshalEntry(d.EmoteURL(e.EmoteID, "1x"), e.ZeroWidth, e.EmoteID, e.Provider)
		if err != nil {
			return err
		}
		pipe.HSet(ctx, key, e.Name, val)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	ev, err := marshalDelta("reload", "", "", false)
	if err != nil {
		return err
	}
	return d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
}

func (d *Dict) AddEmote(ctx context.Context, login, name, emoteID string, zeroWidth bool) error {
	val, err := marshalEntry(d.EmoteURL(emoteID, "1x"), zeroWidth, emoteID, "custom")
	if err != nil {
		return err
	}
	if err := d.rdb.HSet(ctx, channelKey(login), name, val).Err(); err != nil {
		return err
	}
	ev, err := marshalDelta("add", name, d.EmoteURL(emoteID, "1x"), zeroWidth)
	if err != nil {
		return err
	}
	return d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
}

func (d *Dict) RemoveEmote(ctx context.Context, login, name string) error {
	if err := d.rdb.HDel(ctx, channelKey(login), name).Err(); err != nil {
		return err
	}
	ev, err := marshalDelta("remove", name, "", false)
	if err != nil {
		return err
	}
	return d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
}

type EmoteEntry struct {
	Name      string
	EmoteID   string
	ZeroWidth bool
	Provider  string
}

func marshalEntry(url string, zw bool, id string, provider string) (string, error) {
	b, err := json.Marshal(Entry{URL: url, ZeroWidth: zw, ID: id, Provider: provider})
	return string(b), err
}

type deltaEvent struct {
	Action string `json:"action"`
	Emote  struct {
		Name      string `json:"name"`
		URL       string `json:"u,omitempty"`
		ZeroWidth bool   `json:"zw,omitempty"`
	} `json:"emote"`
}

func marshalDelta(action, name, url string, zw bool) (string, error) {
	ev := deltaEvent{Action: action}
	ev.Emote.Name = name
	ev.Emote.URL = url
	ev.Emote.ZeroWidth = zw
	b, err := json.Marshal(ev)
	return string(b), err
}

func MarshalEntry(url string, zw bool) (string, error) {
	return marshalEntry(url, zw, "", "")
}

func MarshalEntryWithMetadata(url string, zw bool, id string, provider string) (string, error) {
	return marshalEntry(url, zw, id, provider)
}

func MarshalDelta(action, name, url string, zw bool) (string, error) {
	return marshalDelta(action, name, url, zw)
}

func IdempotencyKey(emoteID, sourceHash string) string {
	return emoteID + ":" + sourceHash
}
