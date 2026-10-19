package enrich

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/chat/tokenize"
	"streamclone/internal/metrics"
	"streamclone/internal/redisutil/redistest"
)

const expiryChannel = "expirychan"

var expiryKey = "channel:emotes:" + expiryChannel

// syncBuffer is a log sink the test can read while the enricher writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func emoteNames(d *tokenize.ChannelDict, text string) []string {
	var out []string
	for _, f := range d.Tokenize(text) {
		if f.T == "emote" {
			out = append(out, f.C)
		}
	}
	return out
}

func newExpiryTestEnricher(t *testing.T) (*Enricher, *redistest.Server, *syncBuffer) {
	t.Helper()
	srv := redistest.New()
	rdb := srv.Client()
	t.Cleanup(func() { _ = rdb.Close() })
	logs := &syncBuffer{}
	e := &Enricher{
		rdb:   rdb,
		log:   slog.New(slog.NewTextHandler(logs, nil)),
		dicts: make(map[string]*tokenize.ChannelDict),
	}
	return e, srv, logs
}

// loadChannel does what ensure does for a channel whose dictionary exists,
// without starting the delta subscriber (the fake has no pub/sub).
func loadChannel(t *testing.T, e *Enricher, channel string) *tokenize.ChannelDict {
	t.Helper()
	trie, err := e.loadTrie(context.Background(), channel)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	d := &tokenize.ChannelDict{}
	d.Swap(trie)
	e.mu.Lock()
	e.dicts[channel] = d
	e.mu.Unlock()
	return d
}

func emoteDeltaFrames(srv *redistest.Server) int {
	n := 0
	for _, m := range srv.Published() {
		if m.Channel == "chat:"+expiryChannel && strings.Contains(m.Payload, "emote_delta") {
			n++
		}
	}
	return n
}

// Channel dictionaries expire in Redis. A delta reload that finds the key
// gone must keep the dictionary the enricher already serves instead of
// swapping in an empty one, which would strip every third-party emote from
// the channel's chat until the next rebuild.
func TestReloadKeepsDictionaryWhenRedisKeyExpired(t *testing.T) {
	e, srv, logs := newExpiryTestEnricher(t)
	srv.SeedHash(expiryKey, map[string]string{
		"KEKW": `{"u":"https://cdn.invalid/kekw/1x.webp","id":"e1","provider":"seventv"}`,
	}, 24*time.Hour)
	d := loadChannel(t, e, expiryChannel)
	if got := emoteNames(d, "hello KEKW"); len(got) != 1 || got[0] != "KEKW" {
		t.Fatalf("initial tokenize emotes = %v, want [KEKW]", got)
	}

	srv.Advance(24 * time.Hour)
	if _, ok := srv.TTL(expiryKey); ok {
		t.Fatal("fake did not expire the dictionary")
	}
	if e.reload(context.Background(), expiryChannel, d) {
		t.Fatal("reload swapped in a dictionary for a missing key")
	}
	if got := emoteNames(d, "hello KEKW"); len(got) != 1 || got[0] != "KEKW" {
		t.Fatalf("after expiry, tokenize emotes = %v; the reload blanked the channel", got)
	}
	if !strings.Contains(logs.String(), "keeping previous") {
		t.Fatalf("no keep-previous warning logged: %q", logs.String())
	}
	if n := emoteDeltaFrames(srv); n != 0 {
		t.Fatalf("published %d emote_delta frames for a reload that changed nothing", n)
	}

	// The next rebuild brings the key back and the reload picks it up.
	srv.SeedHash(expiryKey, map[string]string{
		"KEKW": `{"u":"https://cdn.invalid/kekw/1x.webp","id":"e1","provider":"seventv"}`,
		"Pog":  `{"u":"https://cdn.invalid/pog/1x.webp","id":"e2","provider":"seventv"}`,
	}, 24*time.Hour)
	if !e.reload(context.Background(), expiryChannel, d) {
		t.Fatal("reload after rebuild did not swap")
	}
	if got := emoteNames(d, "KEKW Pog"); len(got) != 2 {
		t.Fatalf("after rebuild, tokenize emotes = %v, want [KEKW Pog]", got)
	}
	if n := emoteDeltaFrames(srv); n != 1 {
		t.Fatalf("emote_delta frames = %d, want 1", n)
	}
}

// A Redis error keeps the previous dictionary, as before.
func TestReloadKeepsDictionaryOnRedisError(t *testing.T) {
	e, srv, logs := newExpiryTestEnricher(t)
	srv.SeedHash(expiryKey, map[string]string{
		"KEKW": `{"u":"https://cdn.invalid/kekw/1x.webp","id":"e1","provider":"seventv"}`,
	}, 24*time.Hour)
	d := loadChannel(t, e, expiryChannel)
	srv.FailWhen(func(cmd []string) error {
		if cmd[0] == "hgetall" {
			return redistest.Error("LOADING Redis is loading the dataset in memory")
		}
		return nil
	})
	if e.reload(context.Background(), expiryChannel, d) {
		t.Fatal("reload swapped despite a Redis error")
	}
	if got := emoteNames(d, "hello KEKW"); len(got) != 1 {
		t.Fatalf("emotes = %v, want [KEKW]", got)
	}
	if !strings.Contains(logs.String(), "dict rebuild failed") {
		t.Fatalf("no error logged: %q", logs.String())
	}
}

// A missing key is reported as errDictMissing and still counted as a miss.
func TestLoadTrieMissingKeyIsMiss(t *testing.T) {
	e, _, _ := newExpiryTestEnricher(t)
	miss := metrics.EmoteDictionaryLoads.WithLabelValues("miss")
	before := testutil.ToFloat64(miss)
	if _, err := e.loadTrie(context.Background(), expiryChannel); !errors.Is(err, errDictMissing) {
		t.Fatalf("loadTrie = %v, want errDictMissing", err)
	}
	if got := testutil.ToFloat64(miss); got != before+1 {
		t.Fatalf("miss loads = %v, want %v", got, before+1)
	}
}
