package preload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"streamclone/internal/emote/seeder"
)

type seedCall struct {
	login     string
	twitchID  string
	providers []seeder.Provider
	globals   *seeder.GlobalCatalog
}

type fakeChannelSeeder struct {
	mu    sync.Mutex
	calls []seedCall
}

func (f *fakeChannelSeeder) SeedChannelProvidersWithGlobals(_ context.Context, login, twitchID string, providers []seeder.Provider, globals *seeder.GlobalCatalog) ([]seeder.ProviderResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, seedCall{login: login, twitchID: twitchID, providers: providers, globals: globals})
	return nil, nil
}

type fakeResolver map[string]string

func (fakeResolver) Enabled() bool { return true }

func (f fakeResolver) UserIDsByLogin(context.Context, []string) (map[string]string, error) {
	return f, nil
}

func newTestPreloader(t *testing.T, seed channelSeeder) *RosterPreloader {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/streams" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"login":"chana"},{"login":"chanb"}]}`))
	}))
	t.Cleanup(srv.Close)
	return &RosterPreloader{
		metadataURL: srv.URL,
		topN:        2,
		seed:        seed,
		twitch:      fakeResolver{"chana": "1001", "chanb": "1002"},
		httpClient:  srv.Client(),
	}
}

// The preloader seeds every provider into the channel's one canonical set,
// the same set an ensure uses, so a preload never switches a channel to a set
// without BTTV or Twitch emotes.
func TestRunOnceSeedsTheCanonicalProviders(t *testing.T) {
	fake := &fakeChannelSeeder{}
	p := newTestPreloader(t, fake)
	warmed, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if warmed != 2 || len(fake.calls) != 2 {
		t.Fatalf("warmed %d with %d seeds, want 2 and 2", warmed, len(fake.calls))
	}
	for _, call := range fake.calls {
		if !reflect.DeepEqual(call.providers, seeder.CanonicalProviders()) {
			t.Fatalf("%s seeded with %v, want %v", call.login, call.providers, seeder.CanonicalProviders())
		}
	}
}

// Every channel in one cycle shares one global catalog, and the next cycle
// starts a new one so globals stay fresh.
func TestRunOnceSharesOneGlobalCatalogPerCycle(t *testing.T) {
	fake := &fakeChannelSeeder{}
	p := newTestPreloader(t, fake)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 4 {
		t.Fatalf("got %d seeds, want 4", len(fake.calls))
	}
	first, second := fake.calls[0].globals, fake.calls[2].globals
	if first == nil || second == nil {
		t.Fatal("a preload seed ran without a global catalog")
	}
	if fake.calls[1].globals != first || fake.calls[3].globals != second {
		t.Fatal("channels in one cycle got different catalogs")
	}
	if first == second {
		t.Fatal("two cycles shared one catalog")
	}
}
