package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

func Retry(ctx context.Context, attempts int, base time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(base * time.Duration(1<<i)):
		}
	}
	return err
}

var ErrOpen = errors.New("circuit breaker open")

type Breaker struct {
	mu        sync.Mutex
	failures  int
	threshold int
	cooldown  time.Duration
	openUntil time.Time
}

func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	return &Breaker{threshold: threshold, cooldown: cooldown}
}

func (b *Breaker) Do(fn func() error) error {
	b.mu.Lock()
	if time.Now().Before(b.openUntil) {
		b.mu.Unlock()
		return ErrOpen
	}
	b.mu.Unlock()

	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.failures++
		if b.failures >= b.threshold {
			b.openUntil = time.Now().Add(b.cooldown)
			b.failures = 0
		}
		return err
	}
	b.failures = 0
	return nil
}
