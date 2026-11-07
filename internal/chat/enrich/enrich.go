package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/parse"
	"streamclone/internal/chat/tokenize"
	"streamclone/internal/metrics"
)

const twitchNativeEmoteURL = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/1.0"

// errDictMissing means the channel has no dictionary in Redis. Dictionaries
// expire (EMOTE_DICTIONARY_TTL) and always hold the global emotes, so a
// missing key means "expired or not rebuilt yet", never "this channel has no
// emotes": a reload keeps the dictionary the enricher already serves.
var errDictMissing = errors.New("channel emote dictionary not in redis")

type Enricher struct {
	rdb      *redis.Client
	debounce time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	dicts    map[string]*tokenize.ChannelDict
	observe  func(channel string, frag batch.Fragment)
}

func New(rdb *redis.Client, debounceMS int, logger *slog.Logger) *Enricher {
	return &Enricher{
		rdb:      rdb,
		debounce: time.Duration(debounceMS) * time.Millisecond,
		log:      logger,
		dicts:    make(map[string]*tokenize.ChannelDict),
	}
}

func (e *Enricher) SetEmoteObserver(fn func(channel string, frag batch.Fragment)) {
	e.observe = fn
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

func (e *Enricher) Tokenize(channel, text string, native []parse.EmoteRange) []batch.Fragment {
	started := time.Now()
	var fragments []batch.Fragment
	if len(native) == 0 {
		fragments = e.ensure(channel).Tokenize(text)
	} else {
		fragments = e.tokenizeWithNative(channel, text, native)
	}
	e.notifyObserved(channel, fragments)
	metrics.TokenizeSeconds.Observe(time.Since(started).Seconds())
	return fragments
}

func (e *Enricher) notifyObserved(channel string, fragments []batch.Fragment) {
	if e.observe == nil {
		return
	}
	for _, frag := range fragments {
		if frag.T != "emote" || frag.ID == "" {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(frag.Provider))
		switch provider {
		case "seventv", "7tv", "ffz", "frankerfacez", "bttv", "betterttv":
			e.observe(channel, frag)
		}
	}
}

func (e *Enricher) tokenizeWithNative(channel, text string, native []parse.EmoteRange) []batch.Fragment {
	runes := []rune(text)
	dict := e.ensure(channel)
	var frags []batch.Fragment
	cursor := 0

	for _, em := range native {
		if em.Start < cursor || em.Start >= len(runes) {
			continue
		}
		end := em.End + 1
		if end > len(runes) {
			end = len(runes)
		}
		if cursor < em.Start {
			frags = append(frags, dict.Tokenize(string(runes[cursor:em.Start]))...)
		}
		name := string(runes[em.Start:end])
		frags = append(frags, batch.Fragment{
			T:        "emote",
			C:        name,
			U:        fmt.Sprintf(twitchNativeEmoteURL, em.ID),
			ID:       em.ID,
			Provider: "twitch",
		})
		cursor = end
	}
	if cursor < len(runes) {
		frags = append(frags, dict.Tokenize(string(runes[cursor:]))...)
	}
	return frags
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
	if errors.Is(err, errDictMissing) {
		// Not built yet (or expired): the emote service's ensure call
		// rebuilds it and the reload delta fills this dictionary.
		t = tokenize.NewTrie()
	} else if err != nil {
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
		metrics.EmoteDictionaryLoads.WithLabelValues("error").Inc()
		return nil, err
	}
	if len(m) == 0 {
		metrics.EmoteDictionaryLoads.WithLabelValues("miss").Inc()
		return nil, errDictMissing
	}
	metrics.EmoteDictionaryLoads.WithLabelValues("hit").Inc()
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
			e.reload(ctx, channel, dict)
		}
	}
}

// reload re-reads the channel dictionary after a delta and swaps it in. When
// the read fails, or the key is missing because it expired or is being
// rebuilt, the dictionary already in memory stays: blanking it would strip
// every third-party emote from the channel's chat until the next rebuild.
// It reports whether a new dictionary was swapped in.
func (e *Enricher) reload(ctx context.Context, channel string, dict *tokenize.ChannelDict) bool {
	t, err := e.loadTrie(ctx, channel)
	if errors.Is(err, errDictMissing) {
		e.log.Warn("dict missing on reload; keeping previous", "channel", channel)
		return false
	}
	if err != nil {
		e.log.Error("dict rebuild failed", "channel", channel, "err", err)
		return false
	}
	dict.Swap(t)
	frame, _ := json.Marshal(map[string]string{"type": "emote_delta", "channel": channel})
	e.rdb.Publish(ctx, "chat:"+channel, frame)
	return true
}
