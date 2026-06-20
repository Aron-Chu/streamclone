package eventapi

import (
	"testing"
	"time"
)

func TestParseDispatchSetID(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		wantID string
		wantOK bool
	}{
		{
			name:   "emote_set.update dispatch",
			data:   `{"op":0,"d":{"type":"emote_set.update","body":{"id":"60ae8d9ff39a7552b658b60d","actor":{}}}}`,
			wantID: "60ae8d9ff39a7552b658b60d",
			wantOK: true,
		},
		{
			name:   "trims whitespace",
			data:   `{"op":0,"d":{"type":"emote_set.update","body":{"id":"  abc123  "}}}`,
			wantID: "abc123",
			wantOK: true,
		},
		{
			name:   "non-dispatch op (hello)",
			data:   `{"op":1,"d":{"heartbeat_interval":25000,"session_id":"x"}}`,
			wantOK: false,
		},
		{
			name:   "heartbeat op",
			data:   `{"op":2,"d":{"count":1}}`,
			wantOK: false,
		},
		{
			name:   "different dispatch type",
			data:   `{"op":0,"d":{"type":"user.update","body":{"id":"abc"}}}`,
			wantOK: false,
		},
		{
			name:   "missing id",
			data:   `{"op":0,"d":{"type":"emote_set.update","body":{"actor":{}}}}`,
			wantOK: false,
		},
		{
			name:   "malformed json",
			data:   `{"op":0,"d":`,
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := parseDispatchSetID([]byte(tc.data))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (id=%q)", ok, tc.wantOK, id)
			}
			if ok && id != tc.wantID {
				t.Fatalf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

func TestReapIdleRemovesStaleSubscriptions(t *testing.T) {
	s := &Subscriber{
		subs:    make(map[string]subscription),
		idleTTL: 30 * time.Minute,
		enabled: true,
	}
	now := time.Now()
	s.subs["fresh"] = subscription{login: "fresh", providerSetID: "set-fresh", lastActive: now}
	s.subs["stale"] = subscription{login: "stale", providerSetID: "set-stale", lastActive: now.Add(-time.Hour)}

	s.reapIdle()

	if _, ok := s.subs["stale"]; ok {
		t.Fatalf("expected stale subscription to be reaped")
	}
	if _, ok := s.subs["fresh"]; !ok {
		t.Fatalf("expected fresh subscription to survive")
	}
}

func TestReapIdleDisabledWhenTTLZero(t *testing.T) {
	s := &Subscriber{
		subs:    make(map[string]subscription),
		idleTTL: 0,
	}
	s.subs["stale"] = subscription{login: "stale", lastActive: time.Now().Add(-time.Hour)}
	s.reapIdle()
	if _, ok := s.subs["stale"]; !ok {
		t.Fatalf("expected no reaping when idleTTL is zero")
	}
}
