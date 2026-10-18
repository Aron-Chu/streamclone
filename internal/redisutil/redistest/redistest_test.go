package redistest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// These pin the server behaviours the writer tests rely on.

func TestRenameCarriesTTLAndReplacesDestination(t *testing.T) {
	s := New()
	c := s.Client()
	defer c.Close()
	ctx := context.Background()
	s.SeedHash("dst", map[string]string{"old": "1"}, 0)
	s.SeedHash("src", map[string]string{"new": "1"}, time.Minute)
	if err := c.Rename(ctx, "src", "dst").Err(); err != nil {
		t.Fatal(err)
	}
	if h := s.Hash("dst"); len(h) != 1 || h["new"] != "1" {
		t.Fatalf("dst = %v", h)
	}
	if ttl, _ := s.TTL("dst"); ttl != time.Minute {
		t.Fatalf("TTL = %v, want 1m", ttl)
	}
	if err := c.Rename(ctx, "missing", "dst").Err(); err == nil {
		t.Fatal("rename of a missing key succeeded")
	}
}

func TestExpireNXOnlySetsMissingTTL(t *testing.T) {
	s := New()
	c := s.Client()
	defer c.Close()
	ctx := context.Background()
	s.SeedHash("k", map[string]string{"f": "v"}, 0)
	if ok, err := c.ExpireNX(ctx, "k", time.Minute).Result(); err != nil || !ok {
		t.Fatalf("first NX: ok=%v err=%v", ok, err)
	}
	if ok, _ := c.ExpireNX(ctx, "k", time.Hour).Result(); ok {
		t.Fatal("NX replaced an existing TTL")
	}
	if ttl, _ := s.TTL("k"); ttl != time.Minute {
		t.Fatalf("TTL = %v", ttl)
	}
	if d, _ := c.TTL(ctx, "k").Result(); d != time.Minute {
		t.Fatalf("TTL command = %v", d)
	}
	if d, _ := c.TTL(ctx, "nokey").Result(); d != -2 {
		t.Fatalf("TTL on missing = %v, want -2", d)
	}
}

func TestQueueTimeRejectionAbortsWholeTransaction(t *testing.T) {
	s := New()
	c := s.Client()
	defer c.Close()
	ctx := context.Background()
	s.SeedHash("k", map[string]string{"f": "v"}, 0)
	s.FailWhen(func(cmd []string) error {
		if cmd[0] == "hset" {
			return Error("OOM command not allowed")
		}
		return nil
	})
	pipe := c.TxPipeline()
	pipe.Del(ctx, "k")
	pipe.HSet(ctx, "k", "g", "w")
	_, err := pipe.Exec(ctx)
	var re redis.Error
	if !errors.As(err, &re) {
		t.Fatalf("Exec err = %v, want a Redis error reply", err)
	}
	if h := s.Hash("k"); h["f"] != "v" {
		t.Fatalf("aborted transaction changed the key: %v", h)
	}
}

func TestScriptsNeedARegisteredStandIn(t *testing.T) {
	s := New()
	c := s.Client()
	defer c.Close()
	ctx := context.Background()
	script := redis.NewScript("return 1")
	if err := script.Run(ctx, c, []string{"k"}).Err(); err == nil {
		t.Fatal("unregistered script ran")
	}
	s.HandleScript("return 1", func(*State, []string, []string) (any, error) { return int64(1), nil })
	if n, err := script.Run(ctx, c, []string{"k"}).Int64(); err != nil || n != 1 {
		t.Fatalf("registered script: n=%d err=%v", n, err)
	}
}

func TestKeysExpireOnTheFakeClock(t *testing.T) {
	s := New()
	s.SeedHash("k", map[string]string{"f": "v"}, time.Second)
	s.Advance(999 * time.Millisecond)
	if _, ok := s.TTL("k"); !ok {
		t.Fatal("expired early")
	}
	s.Advance(time.Millisecond)
	if _, ok := s.TTL("k"); ok {
		t.Fatal("did not expire")
	}
}
