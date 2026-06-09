package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 5, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("boom")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryExhausts(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return errors.New("boom")
	})
	if err == nil || calls != 3 {
		t.Fatalf("expected failure after 3 calls, got err=%v calls=%d", err, calls)
	}
}

func TestBreakerOpensAndRecovers(t *testing.T) {
	b := NewBreaker(2, 20*time.Millisecond)
	fail := func() error { return errors.New("boom") }

	_ = b.Do(fail)
	_ = b.Do(fail)
	if err := b.Do(fail); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}

	time.Sleep(25 * time.Millisecond)
	if err := b.Do(func() error { return nil }); err != nil {
		t.Fatalf("expected recovery after cooldown, got %v", err)
	}
}
