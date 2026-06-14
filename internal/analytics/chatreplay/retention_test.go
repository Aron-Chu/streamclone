package chatreplay

import (
	"context"
	"testing"
	"time"
)

func TestNewRetentionWorkerDefaults(t *testing.T) {
	cases := []struct {
		name string
		days int
		want time.Duration
	}{
		{"zero falls back to default", 0, DefaultRetentionDays * 24 * time.Hour},
		{"negative falls back to default", -5, DefaultRetentionDays * 24 * time.Hour},
		{"explicit days honored", 7, 7 * 24 * time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := NewRetentionWorker(nil, c.days, nil)
			if w.retention != c.want {
				t.Fatalf("retention = %v, want %v", w.retention, c.want)
			}
			if w.interval > 24*time.Hour {
				t.Fatalf("interval %v exceeds the once-per-24h bound", w.interval)
			}
			if w.log == nil {
				t.Fatal("expected a non-nil logger default")
			}
		})
	}
}

func TestRetentionWorkerStartNilSafe(t *testing.T) {
	// A worker with no backing store must not panic or spawn a loop.
	w := NewRetentionWorker(nil, 90, nil)
	w.Start(context.Background())

	// A worker whose store has a nil pool is likewise a no-op.
	w2 := NewRetentionWorker(NewStore(nil), 90, nil)
	w2.Start(context.Background())

	// purge on a nil-pool store returns 0 without error.
	if _, err := w2.store.PurgeOlderThan(context.Background(), time.Now()); err != nil {
		t.Fatalf("PurgeOlderThan on nil pool: %v", err)
	}
}

func TestPurgeOlderThanNilStore(t *testing.T) {
	var s *Store
	n, err := s.PurgeOlderThan(context.Background(), time.Now())
	if err != nil || n != 0 {
		t.Fatalf("nil store PurgeOlderThan = (%d,%v), want (0,nil)", n, err)
	}
}
