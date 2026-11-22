package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/emote/api"
	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/preload"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/seeder"
	"streamclone/internal/emote/store"
	"streamclone/internal/metadata/helix"
	"streamclone/internal/redisutil/redistest"
)

// fakeUpstream serves every emote provider, Helix and the metadata stream
// list from one server, under a path prefix per service, and counts requests.
type fakeUpstream struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

var upstreamRoutes = map[string]string{
	"/oauth/token":     `{"access_token":"test-token","expires_in":3600}`,
	"/meta/v1/streams": `{"items":[{"login":"chana"},{"login":"chanb"}]}`,

	"/helix/users":                           `{"data":[{"id":"1001","login":"chana"},{"id":"1002","login":"chanb"}]}`,
	"/helix/chat/emotes?broadcaster_id=1001": `{"data":[{"id":"tw-a1","name":"ChanaHype","format":["static"],"images":{"url_1x":"https://example.invalid/tw-a1/1.0"}}]}`,
	"/helix/chat/emotes?broadcaster_id=1002": `{"data":[{"id":"tw-b1","name":"ChanbHype","format":["static"],"images":{"url_1x":"https://example.invalid/tw-b1/1.0"}}]}`,
	"/helix/chat/emotes/global":              `{"data":[{"id":"25","name":"Kappa","format":["static"],"images":{"url_1x":"https://example.invalid/25/1.0"}}]}`,

	"/7tv/users/twitch/1001":  `{"user":{"id":"u-a","username":"chana","display_name":"ChanA"},"emote_set":{"id":"7set-a","emotes":[{"id":"7a-wave","name":"Wave"},{"id":"7a1","name":"SevenA"}]}}`,
	"/7tv/users/twitch/1002":  `{"user":{"id":"u-b","username":"chanb","display_name":"ChanB"},"emote_set":{"id":"7set-b","emotes":[{"id":"7b1","name":"SevenB"}]}}`,
	"/7tv/emote-sets/global":  `{"emotes":[{"id":"7g-clap","name":"Clap"}]}`,
	"/ffz/room/id/1001":       `{"room":{"id":"chana","display_name":"ChanA"},"sets":{"500":{"id":500,"emoticons":[{"id":9001,"name":"FfzA","urls":{"1":"https://example.invalid/ffz/9001/1"}}]}}}`,
	"/ffz/room/id/1002":       `{"room":{"id":"chanb","display_name":"ChanB"},"sets":{"501":{"id":501,"emoticons":[{"id":9002,"name":"FfzB","urls":{"1":"https://example.invalid/ffz/9002/1"}}]}}}`,
	"/ffz/set/global":         `{"default_sets":[3],"sets":{"3":{"id":3,"emoticons":[{"id":25927,"name":"CatBag","urls":{"1":"https://example.invalid/ffz/25927/1"}}]}}}`,
	"/bttv/users/twitch/1001": `{"channelEmotes":[{"id":"bttv-a-clap","code":"Clap","imageType":"png","userId":"bu-a"},{"id":"bttv-a1","code":"BttvA","imageType":"png","userId":"bu-a"}],"sharedEmotes":[]}`,
	"/bttv/users/twitch/1002": `{"channelEmotes":[{"id":"bttv-b1","code":"BttvB","imageType":"png","userId":"bu-b"}],"sharedEmotes":[]}`,
	"/bttv/emotes/global":     `[{"id":"bttv-g-wave","code":"Wave","imageType":"png"},{"id":"bttv-g1","code":"FeelsBirthdayMan","imageType":"png"}]`,
}

var globalEmotePaths = []string{"/7tv/emote-sets/global", "/ffz/set/global", "/bttv/emotes/global", "/helix/chat/emotes/global"}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	up := &fakeUpstream{hits: map[string]int{}}
	up.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up.mu.Lock()
		up.hits[r.URL.Path]++
		up.mu.Unlock()
		body, ok := upstreamRoutes[r.URL.Path]
		if !ok {
			body, ok = upstreamRoutes[r.URL.Path+"?"+r.URL.RawQuery]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(up.Close)
	return up
}

func (up *fakeUpstream) count(path string) int {
	up.mu.Lock()
	defer up.mu.Unlock()
	return up.hits[path]
}

func (up *fakeUpstream) reset() {
	up.mu.Lock()
	defer up.mu.Unlock()
	up.hits = map[string]int{}
}

type emoteStack struct {
	pool   *pgxpool.Pool
	redis  *redistest.Server
	up     *fakeUpstream
	twitch *helix.Client
	seed   *seeder.Seeder
	api    *httptest.Server
}

// newEmoteStack wires the emote service the way cmd/emote does, against the
// test database, an in-process Redis and fake upstreams. Renders are not
// eager, so imports are metadata-only and need no object storage.
func newEmoteStack(t *testing.T) *emoteStack {
	t.Helper()
	skipIfShort(t)
	ctx := context.Background()
	pool := setupPG(t, ctx)
	applyMigrations(t, ctx, pool)
	// The hosted schema adds these provider-load columns in a migration that
	// streamclone's own migrations do not have yet.
	if _, err := pool.Exec(ctx, `ALTER TABLE channel_emote_providers
		ADD COLUMN IF NOT EXISTS expected_count INT NOT NULL DEFAULT 0,
		ADD COLUMN IF NOT EXISTS imported_count INT NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("add provider load columns: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := store.New(pool)
	rsrv := redistest.New()
	rdb := rsrv.Client()
	t.Cleanup(func() { _ = rdb.Close() })
	d := dict.New(rdb, "/emotes")
	rq := render.NewQueue(st, nil, render.Config{}, log)
	up := newFakeUpstream(t)
	tw := helix.New(up.URL+"/helix", up.URL+"/oauth/token", "client", "secret", "test")
	seed := seeder.NewWithRenderQueue(st, nil, d, rq, log, up.URL+"/7tv", up.URL+"/7tv-cdn", up.URL+"/ffz", up.URL+"/bttv", tw, 4)
	h := api.NewWithRenderQueue(st, nil, d, seed, rq, log, "test-token")
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &emoteStack{pool: pool, redis: rsrv, up: up, twitch: tw, seed: seed, api: srv}
}

type ensureResult struct {
	status int
	State  string `json:"state"`
}

func (s *emoteStack) ensure(t *testing.T, login, twitchID string, providers []string) ensureResult {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"twitch_id": twitchID, "providers": providers})
	resp, err := http.Post(s.api.URL+"/v1/channels/"+login+"/emotes/ensure", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("ensure %s: %v", login, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := ensureResult{status: resp.StatusCode}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("ensure %s: decode %q: %v", login, raw, err)
		}
	} else {
		t.Fatalf("ensure %s: status %d: %s", login, resp.StatusCode, raw)
	}
	return out
}

// ensureUntilReady calls ensure until it answers 200 ready; a first ensure
// seeds in the background and answers 202.
func (s *emoteStack) ensureUntilReady(t *testing.T, login, twitchID string, providers []string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last ensureResult
	for time.Now().Before(deadline) {
		last = s.ensure(t, login, twitchID, providers)
		if last.status == http.StatusOK && last.State == "ready" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ensure %s never became ready: last %d %q", login, last.status, last.State)
}

type dictEntry struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

func (s *emoteStack) dictionary(t *testing.T, login string) map[string]dictEntry {
	t.Helper()
	out := map[string]dictEntry{}
	for name, raw := range s.redis.Hash("channel:emotes:" + login) {
		var e dictEntry
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatalf("dictionary %s %s: %v", login, name, err)
		}
		out[name] = e
	}
	return out
}

func (s *emoteStack) activeSet(t *testing.T, login string) (id, name string) {
	t.Helper()
	err := s.pool.QueryRow(context.Background(), `
		SELECT es.id, es.name FROM channels c
		JOIN emote_sets es ON es.id = c.active_emote_set_id
		WHERE c.login = $1`, login).Scan(&id, &name)
	if err != nil {
		t.Fatalf("active set for %s: %v", login, err)
	}
	return id, name
}

func requireProviders(t *testing.T, label string, dictionary map[string]dictEntry, want map[string]string) {
	t.Helper()
	for name, provider := range want {
		got, ok := dictionary[name]
		if !ok {
			t.Errorf("%s: dictionary has no %s (want %s)", label, name, provider)
			continue
		}
		if got.Provider != provider {
			t.Errorf("%s: %s comes from %s, want %s", label, name, got.Provider, provider)
		}
	}
}

// channelAEmotes is every channel emote of chana, plus the two same-name
// collisions: the channel's BTTV Clap beats the 7TV global Clap, and the
// channel's 7TV Wave beats the BTTV global Wave.
var channelAEmotes = map[string]string{
	"SevenA":    "seventv",
	"FfzA":      "ffz",
	"BttvA":     "bttv",
	"ChanaHype": "twitch",
	"Clap":      "bttv",
	"Wave":      "seventv",
}

// A channel that an ensure loaded with every provider keeps its BTTV and
// Twitch emotes after a preload cycle, a channel the preloader warms first
// gets BTTV and Twitch too, and the cycle downloads each provider's global
// list once.
func TestEmotePreloadKeepsEveryProvider(t *testing.T) {
	s := newEmoteStack(t)

	s.ensureUntilReady(t, "chana", "1001", nil)
	requireProviders(t, "after ensure", s.dictionary(t, "chana"), channelAEmotes)
	setBefore, name := s.activeSet(t, "chana")
	if name != seeder.CanonicalSetName("chana") {
		t.Fatalf("active set after ensure = %q, want %q", name, seeder.CanonicalSetName("chana"))
	}

	s.up.reset()
	p := preload.NewRosterPreloader(s.up.URL+"/meta", 2, nil, s.seed, s.twitch, nil)
	warmed, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("preload: %v", err)
	}
	if warmed != 2 {
		t.Fatalf("preload warmed %d channels, want 2", warmed)
	}
	for _, path := range globalEmotePaths {
		if got := s.up.count(path); got != 1 {
			t.Errorf("%s fetched %d times in one preload cycle, want 1", path, got)
		}
	}

	requireProviders(t, "chana after preload", s.dictionary(t, "chana"), channelAEmotes)
	requireProviders(t, "chanb after preload", s.dictionary(t, "chanb"), map[string]string{
		"SevenB":    "seventv",
		"FfzB":      "ffz",
		"BttvB":     "bttv",
		"ChanbHype": "twitch",
		// Globals reach every channel.
		"Clap":             "seventv",
		"Wave":             "bttv",
		"Kappa":            "twitch",
		"CatBag":           "ffz",
		"FeelsBirthdayMan": "bttv",
	})

	setAfter, name := s.activeSet(t, "chana")
	if setAfter != setBefore {
		t.Fatalf("preload switched chana's active set from %s to %s (%q)", setBefore, setAfter, name)
	}
	if _, name := s.activeSet(t, "chanb"); name != seeder.CanonicalSetName("chanb") {
		t.Fatalf("chanb active set = %q, want %q", name, seeder.CanonicalSetName("chanb"))
	}

	// The ensure path agrees the preloaded channels are loaded: no reseed.
	for _, ch := range []struct{ login, id string }{{"chana", "1001"}, {"chanb", "1002"}} {
		if got := s.ensure(t, ch.login, ch.id, nil); got.status != http.StatusOK || got.State != "ready" {
			t.Fatalf("ensure %s after preload = %d %q, want 200 ready", ch.login, got.status, got.State)
		}
	}
}

// A channel left on an older subset set (the preloader's 7TV+FFZ set, or a
// watch page's 7TV+Twitch set) moves to its canonical set on the next
// ensure, whatever providers that ensure asks for, and gets every provider.
func TestEmoteEnsureMovesASubsetSetToTheCanonicalSet(t *testing.T) {
	s := newEmoteStack(t)
	ctx := context.Background()
	st := store.New(s.pool)
	if err := st.UpsertChannel(ctx, "1001", "chana", "ChanA"); err != nil {
		t.Fatal(err)
	}
	legacy, err := st.UpsertEmoteSetByOwnerName(ctx, "chana provider emotes (seventv+ffz)", "1001")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetActiveEmoteSet(ctx, "1001", legacy); err != nil {
		t.Fatal(err)
	}

	s.ensureUntilReady(t, "chana", "1001", []string{"seventv", "twitch"})

	id, name := s.activeSet(t, "chana")
	if id == legacy || name != seeder.CanonicalSetName("chana") {
		t.Fatalf("active set = %s %q, want the canonical set", id, name)
	}
	requireProviders(t, "after a two-provider ensure", s.dictionary(t, "chana"), channelAEmotes)

	// A later ensure with any provider subset leaves the canonical set alone.
	if got := s.ensure(t, "chana", "1001", []string{"seventv"}); got.status != http.StatusOK || got.State != "ready" {
		t.Fatalf("7TV-only ensure = %d %q, want 200 ready", got.status, got.State)
	}
	if again, againName := s.activeSet(t, "chana"); again != id {
		t.Fatalf("a 7TV-only ensure switched the active set to %s (%q)", again, againName)
	}
}
