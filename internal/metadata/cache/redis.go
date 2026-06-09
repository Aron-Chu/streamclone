package cache

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type redisStore struct {
	c *goredis.Client
}

func NewRedisStore(c *goredis.Client) Store {
	return &redisStore{c: c}
}

func (r *redisStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.c.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, ErrNotFound
	}
	return val, err
}

func (r *redisStore) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	return r.c.Set(ctx, key, val, ttl).Err()
}
