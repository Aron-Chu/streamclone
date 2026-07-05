package seeder

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSevenTVZeroWidthFromAPIFlags(t *testing.T) {
	em := sevenTVEmote{Name: "RainTime", Flags: 1}
	em.Data.Flags = 256
	if !sevenTVZeroWidth(em) {
		t.Fatalf("expected overlay emote to be zero width")
	}
	em = sevenTVEmote{Name: "Clap"}
	if sevenTVZeroWidth(em) {
		t.Fatalf("expected regular emote not to be zero width")
	}
}

func TestSortRemoteEmotesPrioritizesUsableStaticEmotes(t *testing.T) {
	emotes := []remoteEmote{
		{Name: "wide", ZeroWidth: true},
		{Name: "dance", Animated: true},
		{Name: "alpha"},
		{Name: "Bravo"},
	}

	sortRemoteEmotes(emotes)

	got := []string{emotes[0].Name, emotes[1].Name, emotes[2].Name, emotes[3].Name}
	want := []string{"alpha", "Bravo", "dance", "wide"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %q want %q; full=%v", i, got[i], want[i], got)
		}
	}
}

func newTestSeeder(baseURL string) *Seeder {
	return &Seeder{
		apiURL:  baseURL,
		cdnURL:  baseURL,
		ffzURL:  baseURL,
		bttvURL: baseURL,
		hc:      &http.Client{},
	}
}

func TestFetchBTTVUserNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	_, err := s.fetchBTTVUser(context.Background(), "12345")
	if !errors.Is(err, errProviderNotFound) {
		t.Fatalf("expected errProviderNotFound, got %v", err)
	}
}

func TestFetchBTTVUserServerErrorIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	_, err := s.fetchBTTVUser(context.Background(), "12345")
	if err == nil || errors.Is(err, errProviderNotFound) {
		t.Fatalf("expected hard failure, got %v", err)
	}
}

func TestFetchFFZRoomNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	_, err := s.fetchFFZRoom(context.Background(), "somechannel", "12345")
	if !errors.Is(err, errProviderNotFound) {
		t.Fatalf("expected errProviderNotFound, got %v", err)
	}
}

func TestFetchFFZRoomPrefersHardFailureOverNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/room/id/12345" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	_, err := s.fetchFFZRoom(context.Background(), "somechannel", "12345")
	if err == nil || errors.Is(err, errProviderNotFound) {
		t.Fatalf("expected hard failure to win over 404, got %v", err)
	}
}

func TestFetchSevenTVUserNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	_, err := s.fetchSevenTVUser(context.Background(), "12345")
	if !errors.Is(err, errProviderNotFound) {
		t.Fatalf("expected errProviderNotFound, got %v", err)
	}
}

func TestFetchFFZGlobalSetsFiltersToDefaultSets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/set/global" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"default_sets": [3],
			"sets": {
				"3": {"id": 3, "emoticons": [{"id": 25927, "name": "CatBag", "urls": {"1": "https://cdn.frankerfacez.com/emote/25927/1"}}]},
				"4330": {"id": 4330, "emoticons": [{"id": 999, "name": "SubOnly", "urls": {"1": "https://cdn.frankerfacez.com/emote/999/1"}}]}
			}
		}`))
	}))
	defer srv.Close()

	s := newTestSeeder(srv.URL)
	sets, err := s.fetchFFZGlobalSets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 default set, got %d", len(sets))
	}
	set, ok := sets["3"]
	if !ok {
		t.Fatalf("expected set 3 to be kept, got %v", sets)
	}
	if len(set.Emoticons) != 1 || set.Emoticons[0].Name != "CatBag" {
		t.Fatalf("unexpected emoticons: %v", set.Emoticons)
	}
}

func TestNormalizeImportConcurrency(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "minimum", in: 0, want: 1},
		{name: "keeps valid", in: 8, want: 8},
		{name: "caps large", in: 100, want: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeImportConcurrency(tt.in); got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}
