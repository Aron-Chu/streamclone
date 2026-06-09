package registry

import (
	"testing"
	"time"
)

func TestReapZeroListenersAfterIdle(t *testing.T) {
	r := New()
	fresh := &Session{Channel: "fresh"}
	stale := &Session{Channel: "stale"}
	fresh.AddListener("fresh-session")
	r.Add(fresh)
	r.Add(stale)
	stale.lastSeen.Store(time.Now().Add(-time.Hour).UnixNano())

	reaped := r.Reap(time.Now(), time.Minute)
	if len(reaped) != 1 || reaped[0].Channel != "stale" {
		t.Fatalf("expected only stale reaped, got %v", reaped)
	}
	if r.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", r.Len())
	}
	if !reaped[0].Stopped() {
		t.Fatal("reaped session should be marked stopped")
	}
}

func TestLeaveForcesReap(t *testing.T) {
	r := New()
	s := &Session{Channel: "a"}
	s.AddListener("session-a")
	r.Add(s)
	s.Leave("session-a")

	if got := r.Reap(time.Now(), time.Hour); len(got) != 0 {
		t.Fatalf("expected grace period after last leave, got %d", len(got))
	}
	if got := r.Reap(time.Now().Add(2*time.Hour), time.Hour); len(got) != 1 {
		t.Fatalf("expected reap after idle timeout, got %d", len(got))
	}
}

func TestKeepaliveKeepsAlive(t *testing.T) {
	r := New()
	s := &Session{Channel: "a"}
	s.AddListener("session-a")
	r.Add(s)
	s.Touch()

	if got := r.Reap(time.Now(), time.Minute); len(got) != 0 {
		t.Fatalf("expected no reap for fresh session, got %d", len(got))
	}
}
