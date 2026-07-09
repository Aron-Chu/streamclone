package redisutil

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// ClientOptions configures a shared go-redis client with pool limits.
type ClientOptions struct {
	URL          string
	PoolSize     int
	MinIdleConns int
	PoolTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewClient parses REDIS_URL and applies pool/timeouts. Falls back to go-redis defaults when unset.
func NewClient(opts ClientOptions) (*redis.Client, error) {
	opt, err := redis.ParseURL(opts.URL)
	if err != nil {
		return nil, err
	}
	if opts.PoolSize > 0 {
		opt.PoolSize = opts.PoolSize
	}
	if opts.MinIdleConns > 0 {
		opt.MinIdleConns = opts.MinIdleConns
	}
	if opts.PoolTimeout > 0 {
		opt.PoolTimeout = opts.PoolTimeout
	}
	if opts.ReadTimeout > 0 {
		opt.ReadTimeout = opts.ReadTimeout
	}
	if opts.WriteTimeout > 0 {
		opt.WriteTimeout = opts.WriteTimeout
	}
	return redis.NewClient(opt), nil
}
