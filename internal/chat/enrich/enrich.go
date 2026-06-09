package enrich

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/tokenize"
	"streamclone/internal/metrics"
)

type Enricher struct {
	rdb      *redis.Client
	debounce time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	dicts    map[string]*tokenize.ChannelDict
}

func New(rdb *redis.Client, debounceMS int, logger *slog.Logger) *Enricher {
	return &Enricher{
		rdb:      rdb,
		debounce: time.Duration(debounceMS) * time.Millisecond,
		log:      logger,
		dicts:    make(map[string]*tokenize.ChannelDict),
	}
}

func (e *Enricher) Invalidate(channel string) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		return
	}
	e.mu.Lock()
	delete(e.dicts, channel)
	e.mu.Unlock()
}

func (e *Enricher) Tokenize(channel, text string) []batch.Fragment {
	started := time.Now()
	fragments := e.ensure(channel).Tokenize(text)
	metrics.TokenizeSeconds.Observe(time.Since(started).Seconds())
	return fragments
}

func (e *Enricher) ensure(channel string) *tokenize.ChannelDict {
	e.mu.Lock()
	if d, ok := e.dicts[channel]; ok {
		e.mu.Unlock()
		return d
	}
	d := &tokenize.ChannelDict{}
	e.dicts[channel] = d
	e.mu.Unlock()

	t, err := e.loadTrie(context.Background(), channel)
	if err != nil {
		e.log.Error("dict load failed", "channel", channel, "err", err)
		t = tokenize.NewTrie()
	}
	d.Swap(t)
	go e.consumeDeltas(channel, d)
	return d
}

func (e *Enricher) loadTrie(ctx context.Context, channel string) (*tokenize.Trie, error) {
	m, err := e.rdb.HGetAll(ctx, "channel:emotes:"+channel).Result()
	if err != nil {
		return nil, err
	}
	t := tokenize.NewTrie()
	for name, v := range m {
		var ent struct {
			U        string `json:"u"`
			Zw       bool   `json:"zw"`
			ID       string `json:"id"`
			Provider string `json:"provider"`
		}
		if json.Unmarshal([]byte(v), &ent) == nil {
			t.Insert(name, tokenize.Emote{ID: ent.ID, URL: ent.U, Zw: ent.Zw, Provider: ent.Provider})
		}
	}
	return t, nil
}

func (e *Enricher) consumeDeltas(channel string, dict *tokenize.ChannelDict) {
	ctx := context.Background()
	sub := e.rdb.Subscribe(ctx, "emotes:delta:"+channel)
	defer sub.Close()
	ch := sub.Channel()

	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
			if timer == nil {
				timer = time.NewTimer(e.debounce)
				timerC = timer.C
			} else {
				timer.Reset(e.debounce)
			}
		case <-timerC:
			timer = nil
			timerC = nil
			t, err := e.loadTrie(ctx, channel)
			if err != nil {
				e.log.Error("dict rebuild failed", "channel", channel, "err", err)
				continue
			}
			dict.Swap(t)
			frame, _ := json.Marshal(map[string]string{"type": "emote_delta", "channel": channel})
			e.rdb.Publish(ctx, "chat:"+channel, frame)
		}
	}
}
