package seeder

import (
	"context"
	"sync"

	"streamclone/internal/metadata/helix"
)

// GlobalCatalog holds each provider's global emote list for one preload
// cycle, so the cycle downloads each list once instead of once per channel.
//
// A failed fetch is kept for the cycle as well: during a provider outage the
// cycle makes one request to that provider, not one per channel. The next
// cycle starts with a new catalog and fetches again. The first caller's
// context bounds each fetch, so share a catalog only between seeds that run
// under one context, as a preload cycle does.
//
// The lists are shared between callers and must not be modified.
//
// A nil *GlobalCatalog is valid and fetches on every call; ensure seeds use
// that, so a single channel's seed behaves as before.
type GlobalCatalog struct {
	sevenTV onceValue[[]sevenTVEmote]
	ffz     onceValue[map[string]ffzSet]
	bttv    onceValue[[]bttvEmote]
	twitch  onceValue[[]helix.ChatEmote]
}

// NewGlobalCatalog returns an empty catalog for one preload cycle.
func NewGlobalCatalog() *GlobalCatalog {
	return &GlobalCatalog{}
}

type onceValue[T any] struct {
	once sync.Once
	val  T
	err  error
}

func (o *onceValue[T]) get(fetch func() (T, error)) (T, error) {
	o.once.Do(func() { o.val, o.err = fetch() })
	return o.val, o.err
}

func (s *Seeder) sevenTVGlobal(ctx context.Context, g *GlobalCatalog) ([]sevenTVEmote, error) {
	if g == nil {
		return s.fetchSevenTVGlobal(ctx)
	}
	return g.sevenTV.get(func() ([]sevenTVEmote, error) { return s.fetchSevenTVGlobal(ctx) })
}

func (s *Seeder) ffzGlobalSets(ctx context.Context, g *GlobalCatalog) (map[string]ffzSet, error) {
	if g == nil {
		return s.fetchFFZGlobalSets(ctx)
	}
	return g.ffz.get(func() (map[string]ffzSet, error) { return s.fetchFFZGlobalSets(ctx) })
}

func (s *Seeder) bttvGlobal(ctx context.Context, g *GlobalCatalog) ([]bttvEmote, error) {
	if g == nil {
		return s.fetchBTTVGlobal(ctx)
	}
	return g.bttv.get(func() ([]bttvEmote, error) { return s.fetchBTTVGlobal(ctx) })
}

func (s *Seeder) twitchGlobal(ctx context.Context, g *GlobalCatalog) ([]helix.ChatEmote, error) {
	if g == nil {
		return s.twitch.GlobalEmotes(ctx)
	}
	return g.twitch.get(func() ([]helix.ChatEmote, error) { return s.twitch.GlobalEmotes(ctx) })
}
