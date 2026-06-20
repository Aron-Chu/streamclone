package ingest

import (
	"errors"
	"testing"
	"time"
)

func TestSpreadBackfillCooldown(t *testing.T) {
	c := newSpreadBackfillCoordinator()
	login := "xqc"
	if _, shouldRun, err := c.request(login); err != nil || !shouldRun {
		t.Fatalf("first request = run=%v err=%v", shouldRun, err)
	}
	c.releaseWithoutRun(login)
	if _, shouldRun, err := c.request(login); !errors.Is(err, ErrSpreadBackfillCooldown) || shouldRun {
		t.Fatalf("second request = run=%v err=%v, want cooldown", shouldRun, err)
	}
}

func TestSpreadBackfillInFlightDedup(t *testing.T) {
	c := newSpreadBackfillCoordinator()
	login := "forsen"
	first, runFirst, err := c.request(login)
	if err != nil || !runFirst {
		t.Fatalf("first request failed: run=%v err=%v", runFirst, err)
	}
	c.mu.Lock()
	c.lastReq[login] = time.Now().Add(-10 * time.Minute)
	c.mu.Unlock()
	second, runSecond, err := c.request(login)
	if err != nil {
		t.Fatalf("in-flight request err=%v", err)
	}
	if runSecond {
		t.Fatal("expected in-flight dedup to skip second run")
	}
	if second.State != "warming" || first.State != "warming" {
		t.Fatalf("states = %#v %#v", first, second)
	}
}

func TestSpreadBackfillCoordinatorFinishSetsReady(t *testing.T) {
	c := newSpreadBackfillCoordinator()
	login := "caseoh"
	_, _, _ = c.request(login)
	c.finish(login)
	meta := c.meta(login)
	if meta.State != "ready" {
		t.Fatalf("state = %q, want ready", meta.State)
	}
	if meta.RequestedAt == nil || time.Since(*meta.RequestedAt) > time.Minute {
		t.Fatalf("requestedAt = %#v", meta.RequestedAt)
	}
}
