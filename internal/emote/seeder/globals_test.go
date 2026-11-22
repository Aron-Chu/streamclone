package seeder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"streamclone/internal/metadata/helix"
)

// globalsServer serves every provider's global emote endpoint and counts the
// requests per path.
type globalsServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
	fail map[string]bool
}

func newGlobalsServer(t *testing.T) *globalsServer {
	t.Helper()
	gs := &globalsServer{hits: map[string]int{}, fail: map[string]bool{}}
	gs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gs.mu.Lock()
		gs.hits[r.URL.Path]++
		fail := gs.fail[r.URL.Path]
		gs.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600}`))
		case "/emote-sets/global":
			_, _ = w.Write([]byte(`{"emotes":[{"id":"7g1","name":"Clap","data":{"animated":false}}]}`))
		case "/set/global":
			_, _ = w.Write([]byte(`{"default_sets":[3],"sets":{"3":{"id":3,"emoticons":[{"id":25927,"name":"CatBag","urls":{"1":"https://cdn.frankerfacez.com/emote/25927/1"}}]}}}`))
		case "/emotes/global":
			_, _ = w.Write([]byte(`[{"id":"b1","code":"FeelsBirthdayMan","imageType":"png"}]`))
		case "/helix/chat/emotes/global":
			_, _ = w.Write([]byte(`{"data":[{"id":"25","name":"Kappa","format":["static"],"images":{"url_1x":"https://example.invalid/25/1.0"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gs.Close)
	return gs
}

func (gs *globalsServer) count(path string) int {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.hits[path]
}

func (gs *globalsServer) seeder() *Seeder {
	s := newTestSeeder(gs.URL)
	s.twitch = helix.New(gs.URL+"/helix", gs.URL+"/oauth/token", "client", "secret", "test")
	return s
}

var globalPaths = []string{"/emote-sets/global", "/set/global", "/emotes/global", "/helix/chat/emotes/global"}

func fetchAllGlobals(t *testing.T, s *Seeder, g *GlobalCatalog) {
	t.Helper()
	// Errorf, not Fatalf: callers run this from several goroutines.
	ctx := context.Background()
	if got, err := s.sevenTVGlobal(ctx, g); err != nil || len(got) != 1 || got[0].Name != "Clap" {
		t.Errorf("7tv globals = %v, %v", got, err)
	}
	if got, err := s.ffzGlobalSets(ctx, g); err != nil || len(got) != 1 {
		t.Errorf("ffz globals = %v, %v", got, err)
	}
	if got, err := s.bttvGlobal(ctx, g); err != nil || len(got) != 1 || got[0].Code != "FeelsBirthdayMan" {
		t.Errorf("bttv globals = %v, %v", got, err)
	}
	if got, err := s.twitchGlobal(ctx, g); err != nil || len(got) != 1 || got[0].Name != "Kappa" || !got[0].IsGlobal {
		t.Errorf("twitch globals = %v, %v", got, err)
	}
}

// One preload cycle shares one catalog across all its channel seeds, so each
// provider's global list is downloaded once per cycle, not once per channel.
func TestGlobalCatalogFetchesEachProviderOncePerCycle(t *testing.T) {
	gs := newGlobalsServer(t)
	s := gs.seeder()
	g := NewGlobalCatalog()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fetchAllGlobals(t, s, g)
		}()
	}
	wg.Wait()
	for _, path := range globalPaths {
		if got := gs.count(path); got != 1 {
			t.Fatalf("%s fetched %d times in one cycle, want 1", path, got)
		}
	}

	// The next cycle gets a new catalog and fetches again.
	fetchAllGlobals(t, s, NewGlobalCatalog())
	for _, path := range globalPaths {
		if got := gs.count(path); got != 2 {
			t.Fatalf("%s fetched %d times over two cycles, want 2", path, got)
		}
	}
}

// Without a catalog (the ensure path) every seed fetches the globals itself,
// as before.
func TestNilGlobalCatalogFetchesEveryTime(t *testing.T) {
	gs := newGlobalsServer(t)
	s := gs.seeder()
	fetchAllGlobals(t, s, nil)
	fetchAllGlobals(t, s, nil)
	for _, path := range globalPaths {
		if got := gs.count(path); got != 2 {
			t.Fatalf("%s fetched %d times, want 2", path, got)
		}
	}
}

// A failed global fetch is not retried by every channel in the same cycle.
func TestGlobalCatalogKeepsAFailureForTheCycle(t *testing.T) {
	gs := newGlobalsServer(t)
	gs.fail["/emotes/global"] = true
	s := gs.seeder()
	g := NewGlobalCatalog()
	var errs atomic.Int32
	for i := 0; i < 3; i++ {
		if _, err := s.bttvGlobal(context.Background(), g); err != nil {
			errs.Add(1)
		}
	}
	if errs.Load() != 3 {
		t.Fatalf("want the failure reported to all 3 callers, got %d", errs.Load())
	}
	if got := gs.count("/emotes/global"); got != 1 {
		t.Fatalf("/emotes/global fetched %d times after a failure, want 1", got)
	}
}
