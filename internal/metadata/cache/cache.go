package cache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("cache: not found")

type StaleResult struct {
	Data  []byte
	Stale bool
}

type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
}

type Cache struct {
	store    Store
	freshTTL time.Duration
	staleTTL time.Duration
}

func New(store Store, freshTTL, staleTTL time.Duration) *Cache {
	return &Cache{store: store, freshTTL: freshTTL, staleTTL: staleTTL}
}

func staleKey(key string) string { return key + ":stale" }

func (c *Cache) Set(ctx context.Context, key string, val []byte) error {
	err1 := c.store.Set(ctx, key, val, c.freshTTL)
	err2 := c.store.Set(ctx, staleKey(key), val, c.staleTTL)
	if err1 != nil {
		return err1
	}
	return err2
}

func (c *Cache) Get(ctx context.Context, key string) (StaleResult, error) {
	val, err := c.store.Get(ctx, key)
	if err == nil {
		return StaleResult{Data: val, Stale: false}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		stale, serr := c.store.Get(ctx, staleKey(key))
		if serr == nil {
			return StaleResult{Data: stale, Stale: true}, nil
		}
	}
	stale, serr := c.store.Get(ctx, staleKey(key))
	if serr == nil {
		return StaleResult{Data: stale, Stale: true}, nil
	}
	return StaleResult{}, fmt.Errorf("%w: %s", ErrNotFound, key)
}

func (c *Cache) GetFresh(ctx context.Context, key string) ([]byte, bool) {
	val, err := c.store.Get(ctx, key)
	if err != nil {
		return nil, false
	}
	return val, true
}
