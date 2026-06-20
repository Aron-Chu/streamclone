package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/metadata/cache"
	"streamclone/internal/metadata/gql"
	"streamclone/internal/metadata/model"
	"streamclone/internal/social/reddit"
	"streamclone/internal/upstream"
)

type testStore struct {
	data map[string][]byte
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newTestCache() *cache.Cache {
	return cache.New(&testStore{data: map[string][]byte{}}, time.Minute, time.Hour)
}

func (s *testStore) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := s.data[key]
	if !ok {
		return nil, cache.ErrNotFound
	}
	return value, nil
}

func (s *testStore) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	s.data[key] = val
	return nil
}

type fakeGQL struct {
	pages []gql.Page[gql.Stream]
	about gql.ChannelAbout
}

func (f *fakeGQL) TopStreams(_ context.Context, limit int, cursor string) (gql.Page[gql.Stream], error) {
	if len(f.pages) == 0 {
		return gql.Page[gql.Stream]{}, nil
	}
	if cursor == "" {
		return f.pages[0], nil
	}
	if len(f.pages) > 1 {
		return f.pages[1], nil
	}
	return gql.Page[gql.Stream]{}, nil
}

func (f *fakeGQL) Categories(context.Context, int, string) (gql.Page[gql.Category], error) {
	return gql.Page[gql.Category]{}, nil
}

func (f *fakeGQL) CategoryStreams(context.Context, string, int, string) (gql.Page[gql.Stream], error) {
	return gql.Page[gql.Stream]{}, nil
}

func (f *fakeGQL) Search(context.Context, string, int) (gql.SearchResult, error) {
	return gql.SearchResult{}, nil
}

func (f *fakeGQL) Channel(context.Context, string) (gql.Channel, error) {
	return gql.Channel{ID: "1", Login: "streamer", DisplayName: "Streamer"}, nil
}

func (f *fakeGQL) ChannelAbout(context.Context, string) (gql.ChannelAbout, error) {
	return f.about, nil
}

type fakeHelix struct {
	query      model.ClipQuery
	badges     model.ChatBadgeCatalog
	topStreams []gql.Stream
}

func (f *fakeHelix) Enabled() bool { return true }

func (f *fakeHelix) TopLiveStreams(_ context.Context, limit int, _ string) (gql.Page[gql.Stream], error) {
	if f.topStreams == nil {
		return gql.Page[gql.Stream]{}, nil
	}
	items := f.topStreams
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return gql.Page[gql.Stream]{Items: items}, nil
}

func (f *fakeHelix) ChannelDetails(context.Context, string) (model.ChannelDetails, error) {
	return model.ChannelDetails{ID: "b1", Login: "streamer", DisplayName: "Streamer", UpdatedAt: time.Now().UnixMilli()}, nil
}

func (f *fakeHelix) ChatBadges(context.Context, string) (model.ChatBadgeCatalog, error) {
	if f.badges.Badges == nil {
		return model.ChatBadgeCatalog{Badges: map[string]model.ChatBadge{}}, nil
	}
	return f.badges, nil
}

func (f *fakeHelix) Clips(_ context.Context, broadcasterID string, query model.ClipQuery) (model.ClipsResponse, error) {
	f.query = query
	return model.ClipsResponse{
		Items:  []model.ClipCard{{ID: "clip1", Title: broadcasterID}},
		Cursor: "next",
	}, nil
}

func (f *fakeHelix) ArchivedStreamHistory(context.Context, string, int) ([]model.StreamStat, error) {
	return nil, nil
}

func TestFetchChannelBadgesReturnsCatalog(t *testing.T) {
	h := New(newTestCache(), &fakeGQL{}).WithHelix(&fakeHelix{
		badges: model.ChatBadgeCatalog{
			Badges: map[string]model.ChatBadge{
				"moderator/1": {SetID: "moderator", VersionID: "1", Title: "Moderator", ImageURL1X: "https://static-cdn.jtvnw.net/badges/v1/mod/1"},
			},
			UpdatedAt: 123,
		},
	})
	catalog, err := h.fetchChannelBadges(context.Background(), "streamer")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Channel != "streamer" || catalog.Sources[0].State != "ready" {
		t.Fatalf("unexpected badge response: %+v", catalog)
	}
	if catalog.Badges["moderator/1"].Title != "Moderator" {
		t.Fatalf("missing moderator badge: %+v", catalog.Badges)
	}
}

func TestParsePeriod(t *testing.T) {
	period, start, end := parsePeriod("24h")
	if period != "24h" || start == nil || end == nil {
		t.Fatalf("unexpected 24h period: %s %v %v", period, start, end)
	}
	period, start, end = parsePeriod("all")
	if period != "all" || start != nil || end != nil {
		t.Fatalf("unexpected all period: %s %v %v", period, start, end)
	}
}

func TestRandomStreamReturnsOneFromPool(t *testing.T) {
	h := New(newTestCache(), &fakeGQL{pages: []gql.Page[gql.Stream]{{
		Items: []gql.Stream{
			{ID: "1", Login: "one", Title: "One"},
			{ID: "2", Login: "two", Title: "Two"},
		},
	}}})
	r := chi.NewRouter()
	h.Mount(r)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/streams/random?pool=2", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Stream   gql.Stream `json:"stream"`
		PoolSize int        `json:"poolSize"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PoolSize != 2 || body.Stream.Login == "" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

type pagingFakeGQL struct {
	pageSize   int
	totalItems int
	calls      []struct {
		limit  int
		cursor string
	}
}

func (f *pagingFakeGQL) TopStreams(_ context.Context, limit int, cursor string) (gql.Page[gql.Stream], error) {
	f.calls = append(f.calls, struct {
		limit  int
		cursor string
	}{limit, cursor})
	if f.pageSize <= 0 {
		f.pageSize = 25
	}
	pageIndex := 0
	if cursor != "" {
		pageIndex, _ = strconv.Atoi(cursor)
	}
	start := pageIndex * f.pageSize
	if start >= f.totalItems {
		return gql.Page[gql.Stream]{}, nil
	}
	end := min(start+limit, f.totalItems)
	items := make([]gql.Stream, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, gql.Stream{
			ID:    strconv.Itoa(i),
			Login: fmt.Sprintf("streamer%d", i),
			Title: fmt.Sprintf("Stream %d", i),
		})
	}
	nextCursor := ""
	if end < f.totalItems {
		nextCursor = strconv.Itoa(pageIndex + 1)
	}
	return gql.Page[gql.Stream]{Items: items, Cursor: nextCursor}, nil
}

func (f *pagingFakeGQL) Categories(context.Context, int, string) (gql.Page[gql.Category], error) {
	return gql.Page[gql.Category]{}, nil
}

func (f *pagingFakeGQL) CategoryStreams(context.Context, string, int, string) (gql.Page[gql.Stream], error) {
	return gql.Page[gql.Stream]{}, nil
}

func (f *pagingFakeGQL) Search(context.Context, string, int) (gql.SearchResult, error) {
	return gql.SearchResult{}, nil
}

func (f *pagingFakeGQL) Channel(context.Context, string) (gql.Channel, error) {
	return gql.Channel{}, nil
}

func (f *pagingFakeGQL) ChannelAbout(context.Context, string) (gql.ChannelAbout, error) {
	return gql.ChannelAbout{}, nil
}

func TestStreamsAggregatesHighLimitAcrossPages(t *testing.T) {
	fake := &pagingFakeGQL{pageSize: 25, totalItems: 220}
	h := New(newTestCache(), fake)
	r := chi.NewRouter()
	h.Mount(r)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/streams?limit=200", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	var page gql.Page[gql.Stream]
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 200 {
		t.Fatalf("expected 200 items, got %d", len(page.Items))
	}
	if page.Items[0].Login != "streamer0" || page.Items[199].Login != "streamer199" {
		t.Fatalf("unexpected item range: first=%+v last=%+v", page.Items[0], page.Items[199])
	}
	if len(fake.calls) < 8 {
		t.Fatalf("expected at least 8 paginated GQL calls, got %d: %+v", len(fake.calls), fake.calls)
	}
	for _, call := range fake.calls {
		if call.limit != 25 {
			t.Fatalf("expected page size 25, got %+v", call)
		}
	}
}

func helixTopStreams(n int) []gql.Stream {
	items := make([]gql.Stream, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, gql.Stream{
			ID:           strconv.Itoa(i),
			Login:        fmt.Sprintf("helix%d", i),
			DisplayName:  fmt.Sprintf("Helix %d", i),
			Title:        fmt.Sprintf("Stream %d", i),
			ViewersCount: n - i,
			Category:     "Just Chatting",
			ThumbnailURL: fmt.Sprintf("https://static.example/%d.jpg", i),
			IsLive:       true,
		})
	}
	return items
}

func TestFetchTopStreamsUsesHelixPrimaryForHighLimit(t *testing.T) {
	hx := &fakeHelix{topStreams: helixTopStreams(200)}
	gqlFake := &pagingFakeGQL{pageSize: 25, totalItems: 220}
	h := New(newTestCache(), gqlFake).WithHelix(hx)

	page, err := h.fetchTopStreams(context.Background(), 200, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 200 {
		t.Fatalf("expected 200 items, got %d", len(page.Items))
	}
	if page.Items[0].Login != "helix0" || page.Items[199].Login != "helix199" {
		t.Fatalf("unexpected helix range: first=%+v last=%+v", page.Items[0], page.Items[199])
	}
	if len(gqlFake.calls) != 0 {
		t.Fatalf("expected no GQL calls when Helix is primary, got %+v", gqlFake.calls)
	}
}

func TestFetchTopStreamsHelixBackfillOnGQLSchemaFailure(t *testing.T) {
	hx := &fakeHelix{topStreams: helixTopStreams(200)}
	gqlFake := &schemaFailGQL{pageSize: 25, firstPageItems: 25, totalItems: 220}
	h := New(newTestCache(), gqlFake).WithHelix(hx)

	page, err := h.fetchTopStreamsGQL(context.Background(), 200, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 200 {
		t.Fatalf("expected 200 items after Helix backfill, got %d", len(page.Items))
	}
	if len(gqlFake.calls) != 2 {
		t.Fatalf("expected 2 GQL calls before schema failure, got %d: %+v", len(gqlFake.calls), gqlFake.calls)
	}
	if page.Items[0].Login != "streamer0" {
		t.Fatalf("expected first item from GQL page 1, got %+v", page.Items[0])
	}
	if page.Items[24].Login != "streamer24" {
		t.Fatalf("expected GQL tail before backfill, got %+v", page.Items[24])
	}
	if page.Items[25].Login != "helix0" {
		t.Fatalf("expected Helix backfill after GQL failure, got %+v", page.Items[25])
	}
}

type schemaFailGQL struct {
	pageSize       int
	firstPageItems int
	totalItems     int
	calls          []struct {
		limit  int
		cursor string
	}
}

func (f *schemaFailGQL) TopStreams(_ context.Context, limit int, cursor string) (gql.Page[gql.Stream], error) {
	f.calls = append(f.calls, struct {
		limit  int
		cursor string
	}{limit, cursor})
	if f.pageSize <= 0 {
		f.pageSize = 25
	}
	if cursor == "" {
		items := make([]gql.Stream, 0, f.firstPageItems)
		for i := 0; i < f.firstPageItems; i++ {
			items = append(items, gql.Stream{
				ID:    strconv.Itoa(i),
				Login: fmt.Sprintf("streamer%d", i),
				Title: fmt.Sprintf("Stream %d", i),
			})
		}
		return gql.Page[gql.Stream]{Items: items, Cursor: "page2"}, nil
	}
	return gql.Page[gql.Stream]{}, upstream.ErrUpstreamSchema
}

func (f *schemaFailGQL) Categories(context.Context, int, string) (gql.Page[gql.Category], error) {
	return gql.Page[gql.Category]{}, nil
}

func (f *schemaFailGQL) CategoryStreams(context.Context, string, int, string) (gql.Page[gql.Stream], error) {
	return gql.Page[gql.Stream]{}, nil
}

func (f *schemaFailGQL) Search(context.Context, string, int) (gql.SearchResult, error) {
	return gql.SearchResult{}, nil
}

func (f *schemaFailGQL) Channel(context.Context, string) (gql.Channel, error) {
	return gql.Channel{}, nil
}

func (f *schemaFailGQL) ChannelAbout(context.Context, string) (gql.ChannelAbout, error) {
	return gql.ChannelAbout{}, nil
}

func TestParseLimitCapsAtMax(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/streams?limit=999", nil)
	if got := parseLimit(req); got != maxLimit {
		t.Fatalf("expected maxLimit %d, got %d", maxLimit, got)
	}
}

func TestChannelClipsBuildsPeriodQuery(t *testing.T) {
	hx := &fakeHelix{}
	h := New(newTestCache(), &fakeGQL{}).WithHelix(hx)
	r := chi.NewRouter()
	h.Mount(r)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/channels/streamer/clips?period=7d&cursor=abc&limit=25", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if hx.query.Period != "7d" || hx.query.Cursor != "abc" || hx.query.Limit != 25 {
		t.Fatalf("unexpected clip query: %+v", hx.query)
	}
	if hx.query.StartedAt == nil || hx.query.EndedAt == nil {
		t.Fatalf("expected date range: %+v", hx.query)
	}
}

func TestChannelDetailsAddsOptionalGQLAboutPanels(t *testing.T) {
	hx := &fakeHelix{}
	h := New(newTestCache(), &fakeGQL{about: gql.ChannelAbout{
		Panels:      []gql.AboutPanel{{ID: "p1", Title: "Schedule", Description: "Weekdays", LinkURL: "https://example.com"}},
		SocialLinks: []gql.SocialLink{{ID: "s1", Title: "Twitter", URL: "https://example.com/social"}},
	}}).WithHelix(hx)
	details, err := h.fetchChannelDetails(context.Background(), "streamer")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.AboutPanels) != 1 || details.AboutPanels[0].Title != "Schedule" {
		t.Fatalf("expected about panel, got %+v", details.AboutPanels)
	}
	if len(details.SocialLinks) != 1 || details.SocialLinks[0].Title != "Twitter" {
		t.Fatalf("expected social link, got %+v", details.SocialLinks)
	}
	last := details.Sources[len(details.Sources)-1]
	if last.Source != "twitch_gql_about_panels" || last.State != "ready" {
		t.Fatalf("expected ready about source, got %+v", details.Sources)
	}
}

func TestParseRedditHTMLListing(t *testing.T) {
	posts := reddit.ParseHTMLListing(`<a href="/r/LivestreamFail/comments/abc123/post_slug/">Funny streamer clip</a>`, "https://www.reddit.com", "streamer")
	if len(posts) != 1 {
		t.Fatalf("expected post, got %+v", posts)
	}
	if posts[0].ID != "abc123" || posts[0].Title != "Funny streamer clip" || len(posts[0].StreamerTags) != 1 {
		t.Fatalf("unexpected post: %+v", posts[0])
	}
}

func TestRedditLSFHTMLFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("blocked by network security"))
		case "/r/LivestreamFail/search":
			if got := r.URL.Query().Get("t"); got != "month" {
				t.Fatalf("expected month period, got %q", got)
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<a href="/r/LivestreamFail/comments/abc123/post_slug/">Recovered post</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, HTMLFallback: true, Provider: "auto"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "30d", "top")
	last := statuses[len(statuses)-1]
	if last.State != "fallback" || len(posts) != 1 {
		t.Fatalf("expected fallback post, got statuses=%+v posts=%+v", statuses, posts)
	}
}

func TestRedditLSFPublicJSONBuildsQuery(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Fatalf("expected user agent, got %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"children":[{"data":{"id":"abc123","title":"Clip","url":"https://clips.twitch.tv/x","permalink":"/r/LivestreamFail/comments/abc123/post/","thumbnail":"default","author":"poster","subreddit":"LivestreamFail","link_flair_text":"Streamer","score":12,"num_comments":3,"created_utc":1700000000}}]}}`))
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "public_json"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "new")
	if len(posts) != 1 || posts[0].Thumbnail != "" || posts[0].Permalink != srv.URL+"/r/LivestreamFail/comments/abc123/post/" {
		t.Fatalf("unexpected posts: %+v", posts)
	}
	if posts[0].FlairText != "Streamer" || len(posts[0].StreamerTags) != 1 {
		t.Fatalf("expected flair tag, got %+v", posts[0])
	}
	if statuses[0].Provider != "public_json" || statuses[0].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
	if gotQuery.Get("q") != "streamer" || gotQuery.Get("sort") != "new" || gotQuery.Get("t") != "week" || gotQuery.Get("restrict_sr") != "1" {
		t.Fatalf("unexpected query: %s", gotQuery.Encode())
	}
}

func TestRedditLSFOfficialUsesOAuthToken(t *testing.T) {
	var sawBearer bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			if user, pass, ok := r.BasicAuth(); !ok || user != "rid" || pass != "secret" {
				t.Fatalf("unexpected reddit auth")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"reddit-token","expires_in":3600}`))
		case "/r/LivestreamFail/search":
			sawBearer = r.Header.Get("Authorization") == "Bearer reddit-token"
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"children":[{"data":{"id":"def456","title":"Official post","url":"https://reddit.com/x","permalink":"/r/LivestreamFail/comments/def456/post/","subreddit":"LivestreamFail"}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).WithExternalSources("", srv.URL, "test-agent").WithRedditOptions(RedditOptions{
		Provider:     "official",
		BaseURL:      srv.URL,
		OAuthAPIURL:  srv.URL,
		TokenURL:     srv.URL + "/api/v1/access_token",
		ClientID:     "rid",
		ClientSecret: "secret",
	})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "24h", "top")
	if !sawBearer || len(posts) != 1 || statuses[0].Provider != "official" || statuses[0].State != "ready" {
		t.Fatalf("expected official ready, sawBearer=%v posts=%+v statuses=%+v", sawBearer, posts, statuses)
	}
}

func TestRedditHTMLFallbackDisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("blocked by network security"))
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "auto"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if len(posts) != 0 {
		t.Fatalf("expected no posts, got %+v", posts)
	}
	last := statuses[len(statuses)-1]
	if last.Provider != "html" || last.State != "unavailable" {
		t.Fatalf("expected html disabled status, got %+v", statuses)
	}
}

func TestRedditLSFAutoUsesThirdPartyBeforeHTML(t *testing.T) {
	var sawThirdParty bool
	var sawHTML bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.WriteHeader(http.StatusForbidden)
		case "/third-party":
			sawThirdParty = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[{"id":"tp1","title":"Third party post","url":"https://clips.twitch.tv/x","permalink":"/r/LivestreamFail/comments/tp1/post/","score":4}]}`))
		case "/r/LivestreamFail/search":
			sawHTML = true
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<a href="/r/LivestreamFail/comments/html1/post/">HTML post</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "auto", HTMLFallback: true, ThirdPartyURL: srv.URL + "/third-party"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if !sawThirdParty || sawHTML || len(posts) != 1 || posts[0].ID != "tp1" {
		t.Fatalf("expected third-party before html, sawThirdParty=%v sawHTML=%v posts=%+v", sawThirdParty, sawHTML, posts)
	}
	if statuses[len(statuses)-1].Provider != "third_party" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestRedditLSFAutoUsesFirecrawlBeforeHTML(t *testing.T) {
	var sawFirecrawl bool
	var sawHTML bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.WriteHeader(http.StatusForbidden)
		case "/firecrawl":
			sawFirecrawl = true
			if r.Header.Get("Authorization") != "Bearer fc-test" {
				t.Fatalf("expected firecrawl auth, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"html":"<a href=\"/r/LivestreamFail/comments/fc1/post/\">Firecrawl post</a>"}}`))
		case "/r/LivestreamFail/search":
			sawHTML = true
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<a href="/r/LivestreamFail/comments/html1/post/">HTML post</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "auto", HTMLFallback: true, FirecrawlURL: srv.URL + "/firecrawl", FirecrawlKey: "fc-test"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if !sawFirecrawl || sawHTML || len(posts) != 1 || posts[0].ID != "fc1" {
		t.Fatalf("expected firecrawl before html, sawFirecrawl=%v sawHTML=%v posts=%+v", sawFirecrawl, sawHTML, posts)
	}
	if statuses[len(statuses)-1].Provider != "scraper" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestRedditLSFOffUsesPublicJSONWithoutScraper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/LivestreamFail/search.json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"children":[{"data":{"id":"json1","title":"ohnepixel clutch","url":"https://reddit.com/x","permalink":"/r/LivestreamFail/comments/json1/post/","subreddit":"LivestreamFail","score":12,"num_comments":3,"created_utc":1700000000}}]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "off"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "ohnepixel", "7d", "hot")
	if len(posts) != 1 || posts[0].ID != "json1" {
		t.Fatalf("expected public json fetch, got posts=%+v statuses=%+v", posts, statuses)
	}
	if statuses[0].Provider != "public_json" || statuses[0].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestRedditLSFOffFallsBackToHotFeedWhenSearchEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"children":[]}}`))
		case "/r/LivestreamFail/hot.json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"children":[{"data":{"id":"hot1","title":"ohnepixel loses it on stream","url":"https://reddit.com/x","permalink":"/r/LivestreamFail/comments/hot1/post/","subreddit":"LivestreamFail","score":99,"num_comments":8,"created_utc":1700000000}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "off"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "ohnepixel", "7d", "hot")
	if len(posts) != 1 || posts[0].ID != "hot1" {
		t.Fatalf("expected hot fallback, got posts=%+v statuses=%+v", posts, statuses)
	}
	if statuses[len(statuses)-1].Provider != "public_json_hot" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestRedditLSFOffAutoEnablesWhenScraperReady(t *testing.T) {
	var sawScrape bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.WriteHeader(http.StatusForbidden)
		case "/v2/scrape":
			sawScrape = true
			if r.Header.Get("Authorization") != "Bearer fc-test" {
				t.Fatalf("expected scraper auth, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"html":"<shreddit-post permalink=\"/r/LivestreamFail/comments/sc1/post/\" post-title=\"streamer big moment\"></shreddit-post>"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", "https://reddit.test", "test-agent").
		WithRedditOptions(RedditOptions{
			BaseURL:    "https://reddit.test",
			Provider:   "off",
			ScraperURL: srv.URL + "/v2/scrape",
			ScraperKey: "fc-test",
		})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if !sawScrape || len(posts) != 1 || posts[0].ID != "sc1" {
		t.Fatalf("expected scraper search auto-enable, sawScrape=%v posts=%+v", sawScrape, posts)
	}
	if statuses[len(statuses)-1].Provider != "scraper" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestParseRedditHTMLListingShredditPost(t *testing.T) {
	body := `<shreddit-post permalink="/r/LivestreamFail/comments/abc123/some_title/" post-title="ohnepixel throws keyboard"></shreddit-post>`
	posts := reddit.ParseHTMLListing(body, "https://www.reddit.com", "ohnepixel")
	if len(posts) != 1 || posts[0].ID != "abc123" {
		t.Fatalf("expected shreddit post parse, got %+v", posts)
	}
}

func TestRedditLSFOffBlockedJSONUsesScraperHotFeed(t *testing.T) {
	var sawHotScrape bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.WriteHeader(http.StatusForbidden)
		case "/r/LivestreamFail/hot.json":
			w.WriteHeader(http.StatusForbidden)
		case "/v2/scrape":
			var payload struct {
				URL string `json:"url"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if strings.Contains(payload.URL, "/r/LivestreamFail/hot") {
				sawHotScrape = true
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"success":true,"data":{"html":"<a href=\"/r/LivestreamFail/comments/hotsc1/post/\">ohnepixel big fail</a>"}}`))
				return
			}
			w.WriteHeader(http.StatusGatewayTimeout)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{
			BaseURL:      srv.URL,
			Provider:     "off",
			HTMLFallback: false,
			ScraperURL:   srv.URL + "/v2/scrape",
			ScraperKey:   "fc-test",
		})
	posts, statuses := h.fetchRedditLSF(context.Background(), "ohnepixel", "24h", "hot")
	if !sawHotScrape || len(posts) != 1 || posts[0].ID != "hotsc1" {
		t.Fatalf("expected scraper hot fallback, sawHotScrape=%v posts=%+v statuses=%+v", sawHotScrape, posts, statuses)
	}
	if statuses[len(statuses)-1].Provider != "scraper_hot" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestRedditLSFOffWhenJSONBlockedAndScraperOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			w.WriteHeader(http.StatusForbidden)
		case "/r/LivestreamFail/hot.json":
			w.WriteHeader(http.StatusForbidden)
		case "/v2/scrape":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{
			BaseURL:      srv.URL,
			Provider:     "off",
			HTMLFallback: false,
			ScraperURL:   srv.URL + "/v2/scrape",
			ScraperKey:   "fc-test",
		})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if len(posts) != 0 {
		t.Fatalf("expected no posts, got %+v", posts)
	}
	if len(statuses) == 0 || statuses[0].Provider != "public_json" || statuses[0].State != "blocked" {
		t.Fatalf("expected blocked public json first, got %+v", statuses)
	}
}

func TestRedditLSFFirecrawlProviderSkipsOfficialAndJSON(t *testing.T) {
	var sawFirecrawl bool
	var sawJSON bool
	var sawHTML bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/LivestreamFail/search.json":
			sawJSON = true
			w.WriteHeader(http.StatusForbidden)
		case "/firecrawl":
			sawFirecrawl = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"html":"<a href=\"/r/LivestreamFail/comments/fc2/post/\">Firecrawl direct post</a>"}}`))
		case "/r/LivestreamFail/search":
			sawHTML = true
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<a href="/r/LivestreamFail/comments/html2/post/">HTML post</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "firecrawl", HTMLFallback: true, FirecrawlURL: srv.URL + "/firecrawl", FirecrawlKey: "fc-test"})
	posts, statuses := h.fetchRedditLSF(context.Background(), "streamer", "7d", "top")
	if !sawFirecrawl || sawJSON || sawHTML || len(posts) != 1 || posts[0].ID != "fc2" {
		t.Fatalf("expected direct firecrawl path, sawFirecrawl=%v sawJSON=%v sawHTML=%v posts=%+v", sawFirecrawl, sawJSON, sawHTML, posts)
	}
	if len(statuses) != 1 || statuses[0].Provider != "scraper" || statuses[0].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestStatsDerivedDoesNotInventHistory(t *testing.T) {
	stats := &model.TwitchTrackerSummary{MinutesStreamed: 120, AvgViewers: 100, MaxViewers: 250, HoursWatched: 300, Followers: 10}
	history := buildStreamHistory(nil, "7d")
	if len(history) != 0 {
		t.Fatalf("expected no invented stream history, got %+v", history)
	}
	derived := buildStatsDerived(stats, []model.ClipCard{{ID: "c1"}}, []model.RedditPost{{ID: "r1"}, {ID: "r2"}}, history)
	if derived == nil || derived.HoursStreamed != 2 || derived.ViewerHoursPerStreamHour != 150 || derived.PeakToAverageRatio != 2.5 || derived.ClipsLoaded != 1 || derived.LSFPostsLoaded != 2 || derived.HasRealStreamHistory {
		t.Fatalf("unexpected derived stats: %+v", derived)
	}
}

func TestParseTwitchTrackerStreamsTable(t *testing.T) {
	body := `<table id="streams"><tbody>
	<tr class="odd">
		<td nowrap data-order="2026-06-04 00:00"><a href="/xqc/streams/319638832474"><span>03/Jun/2026 17:00</span></a></td>
		<td data-order="354"><span>5.9 <small>hrs</small></span></td>
		<td><span>22,200</span></td>
		<td><span>27,141</span></td>
		<td><span>298</span></td>
		<td><span>0</span></td>
		<td class="status">🦎LIVE🦎CLICK🦎DRAMA🦎NEWS</td>
		<td class="games" data-order="3"><img data-original-title="Just Chatting"><img data-original-title="Don’t Sleep with the Fishes"></td>
	</tr>
	<tr class="even">
		<td nowrap data-order="2026-06-02 20:23"><a href="/xqc/streams/319618799450"><span>02/Jun/2026 13:23</span></a></td>
		<td data-order="551"><span>9.2 <small>hrs</small></span></td>
		<td><span>27,154</span></td>
		<td><span>41,720</span></td>
		<td><span>538</span></td>
		<td><span>0</span></td>
		<td class="status">STATE OF PLAY</td>
		<td class="games" data-order="6"><img data-original-title="Just Chatting"><img data-original-title="Special Events"></td>
	</tr>
	</tbody></table>`

	history := parseTwitchTrackerStreamsTable(body)
	if len(history) != 2 {
		t.Fatalf("expected 2 history rows, got %+v", history)
	}
	if history[0].ID != "319638832474" || history[0].Title != "🦎LIVE🦎CLICK🦎DRAMA🦎NEWS" || history[0].Category != "Just Chatting" {
		t.Fatalf("unexpected first history row: %+v", history[0])
	}
	if history[0].DurationMinutes != 354 || history[0].AvgViewers != 22200 || history[0].PeakViewers != 27141 || history[0].HoursWatched != 130980 {
		t.Fatalf("unexpected first row stats: %+v", history[0])
	}
	if history[0].StartedAt != "2026-06-04T00:00:00Z" || history[0].EndedAt != "2026-06-04T05:54:00Z" {
		t.Fatalf("unexpected first row times: %+v", history[0])
	}
}

func TestBuildStreamHistoryFiltersPeriod(t *testing.T) {
	now := time.Now().UTC()
	history := buildStreamHistory([]model.StreamStat{
		{ID: "recent", StartedAt: now.Add(-2 * time.Hour).Format(time.RFC3339)},
		{ID: "older", StartedAt: now.AddDate(0, 0, -10).Format(time.RFC3339)},
	}, "7d")
	if len(history) != 1 || history[0].ID != "recent" {
		t.Fatalf("expected only recent row, got %+v", history)
	}
}

func TestBuildStatsTimelineUsesHistoryOrSummary(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	points := buildStatsTimeline(&model.TwitchTrackerSummary{AvgViewers: 100, MaxViewers: 250}, []model.StreamStat{
		{ID: "newer", StartedAt: now.Format(time.RFC3339), AvgViewers: 200, PeakViewers: 400, HoursWatched: 600},
		{ID: "older", StartedAt: now.AddDate(0, 0, -1).Format(time.RFC3339), AvgViewers: 100, PeakViewers: 250, HoursWatched: 300},
	})
	if len(points) != 2 || points[0].Label != "Jun 5" || points[0].AvgViewers != 100 || points[1].Label != "Jun 6" || points[1].PeakViewers != 400 {
		t.Fatalf("unexpected history points: %+v", points)
	}
	points = buildStatsTimeline(&model.TwitchTrackerSummary{AvgViewers: 100, MaxViewers: 250}, nil)
	if len(points) != 2 || points[0].Label != "Average" || points[0].AvgViewers != 100 || points[1].Label != "Peak" || points[1].AvgViewers != 250 {
		t.Fatalf("unexpected summary points: %+v", points)
	}
}

func TestFetchTwitchTrackerStreamHistoryFallsBackToFirecrawl(t *testing.T) {
	startedAt := time.Now().UTC().Add(-6 * time.Hour).Truncate(time.Minute)
	streamsHTML := fmt.Sprintf(`<table id="streams"><tbody><tr><td data-order="%s"><a href="/streamer/streams/123"><span>%s</span></a></td><td data-order="180"><span>3 <small>hrs</small></span></td><td><span>12,345</span></td><td><span>23,456</span></td><td><span>78</span></td><td><span>0</span></td><td class="status">Test title</td><td class="games"><img data-original-title="Just Chatting"></td></tr></tbody></table>`, startedAt.Format("2006-01-02 15:04"), startedAt.Format("02/Jan/2006 15:04"))

	var sawFirecrawl bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/streamer/streams":
			w.WriteHeader(http.StatusForbidden)
		case "/firecrawl":
			sawFirecrawl = true
			if r.Header.Get("Authorization") != "Bearer fc-test" {
				t.Fatalf("expected firecrawl auth, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"html":` + strconv.Quote(streamsHTML) + `}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources(srv.URL+"/api", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{FirecrawlURL: srv.URL + "/firecrawl", FirecrawlKey: "fc-test"})

	history, status := h.fetchTwitchTrackerStreamHistory(context.Background(), "streamer", "all")
	if !sawFirecrawl || len(history) != 1 {
		t.Fatalf("expected firecrawl-backed history, sawFirecrawl=%v history=%+v", sawFirecrawl, history)
	}
	if status.Provider != "scraper" || status.State != "ready" {
		t.Fatalf("unexpected history status: %+v", status)
	}
}

func TestNormalizeSort(t *testing.T) {
	values := url.Values{}
	values.Set("sort", normalizeSort("new"))
	if values.Get("sort") != "new" {
		t.Fatalf("unexpected sort: %s", values.Get("sort"))
	}
	if normalizeSort("weird") != "top" {
		t.Fatal("bad sort should default to top")
	}
}

func TestParseYouTubeURL(t *testing.T) {
	ref, ok := parseYouTubeURL("https://www.youtube.com/@streamer")
	if !ok || ref.Kind != "handle" || ref.Value != "streamer" {
		t.Fatalf("unexpected handle ref: %+v ok=%v", ref, ok)
	}
	ref, ok = parseYouTubeURL("https://www.youtube.com/channel/UCabc123")
	if !ok || ref.Kind != "channel_id" || ref.Value != "UCabc123" {
		t.Fatalf("unexpected channel ref: %+v ok=%v", ref, ok)
	}
}

func TestParseYouTubeChannelHTML(t *testing.T) {
	body := `"subscriberCountText":{"simpleText":"1.2M subscribers"}` +
		`"videoId":"12345678901"` +
		`"title":{"simpleText":"Latest upload"}`
	info := parseYouTubeChannelHTML(body, youtubeRef{Kind: "handle", Value: "streamer"}, 5)
	if info == nil || info.SubscriberCount == nil || *info.SubscriberCount != 1200000 {
		t.Fatalf("unexpected subscriber parse: %+v", info)
	}
	if len(info.LatestVideos) != 1 || info.LatestVideos[0].ID != "12345678901" || info.LatestVideos[0].Title != "Latest upload" {
		t.Fatalf("unexpected videos: %+v", info.LatestVideos)
	}
}

func TestYouTubeAPIProviderLoadsChannelAndVideos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youtube/v3/channels":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("forHandle") == "streamer" {
				w.Write([]byte(`{"items":[{"id":"UCstreamer"}]}`))
				return
			}
			if r.URL.Query().Get("id") == "UCstreamer" {
				w.Write([]byte(`{"items":[{"id":"UCstreamer","snippet":{"title":"Streamer YT","customUrl":"@streamer","thumbnails":{"default":{"url":"https://img.example/yt.jpg"}}},"statistics":{"subscriberCount":"42000","videoCount":"12","hiddenSubscriberCount":false}}]}`))
				return
			}
		case "/youtube/v3/search":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[{"id":{"videoId":"vid001"},"snippet":{"title":"New VOD","publishedAt":"2026-06-01T00:00:00Z","thumbnails":{"medium":{"url":"https://img.example/vid.jpg"}}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithYouTubeOptions(YouTubeOptions{Provider: "api", APIKey: "yt-test", APIURL: srv.URL + "/youtube/v3"})
	info, statuses := h.fetchYouTubeInfo(context.Background(), youtubeRef{Kind: "handle", Value: "streamer"}, 5)
	if info == nil || info.Title != "Streamer YT" || info.SubscriberCount == nil || *info.SubscriberCount != 42000 {
		t.Fatalf("unexpected youtube info: %+v", info)
	}
	if info.VideoCount == nil || *info.VideoCount != 12 || len(info.LatestVideos) != 1 || info.LatestVideos[0].ID != "vid001" {
		t.Fatalf("unexpected videos/count: count=%v videos=%+v", info.VideoCount, info.LatestVideos)
	}
	if len(statuses) != 1 || statuses[0].Provider != "api" || statuses[0].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestYouTubeAutoFallsBackToScrape(t *testing.T) {
	var sawScrape bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youtube/v3/channels":
			w.WriteHeader(http.StatusForbidden)
		case "/firecrawl":
			sawScrape = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["siteProfile"] != "social_public" {
				t.Fatalf("expected social_public siteProfile, got %#v", payload["siteProfile"])
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"html":"\"subscriberCountText\":{\"simpleText\":\"900 subscribers\"}\"videoId\":\"zzzzzzzzzzz\"\"title\":{\"simpleText\":\"Scraped video\"}"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithYouTubeOptions(YouTubeOptions{Provider: "auto", APIKey: "yt-test", APIURL: srv.URL + "/youtube/v3"}).
		WithRedditOptions(RedditOptions{FirecrawlURL: srv.URL + "/firecrawl", FirecrawlKey: "fc-test"})
	info, statuses := h.fetchYouTubeInfo(context.Background(), youtubeRef{Kind: "handle", Value: "streamer"}, 5)
	if !sawScrape || info == nil || len(info.LatestVideos) != 1 || info.LatestVideos[0].Title != "Scraped video" {
		t.Fatalf("expected scrape fallback, sawScrape=%v info=%+v", sawScrape, info)
	}
	if len(statuses) < 2 || statuses[len(statuses)-1].Provider != "scrape" || statuses[len(statuses)-1].State != "ready" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestYouTubeResolveFromTwitchSocialLink(t *testing.T) {
	h := New(newTestCache(), &fakeGQL{about: gql.ChannelAbout{
		SocialLinks: []gql.SocialLink{{URL: "https://www.youtube.com/@fromtwitch"}},
	}}).WithYouTubeOptions(YouTubeOptions{Provider: "off"})
	ref, source, err := h.resolveYouTubeRef(context.Background(), "streamer", "", "")
	if err != nil || ref.Kind != "handle" || ref.Value != "fromtwitch" || source != "from twitch about social links" {
		t.Fatalf("unexpected resolve: ref=%+v source=%q err=%v", ref, source, err)
	}
}

func TestChannelYouTubeRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/channels" {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("forHandle") == "streamer" {
				w.Write([]byte(`{"items":[{"id":"UCstreamer"}]}`))
				return
			}
			w.Write([]byte(`{"items":[{"id":"UCstreamer","snippet":{"title":"Streamer YT","customUrl":"@streamer","thumbnails":{"default":{"url":"https://img.example/yt.jpg"}}},"statistics":{"subscriberCount":"1000","videoCount":"2","hiddenSubscriberCount":false}}]}`))
			return
		}
		if r.URL.Path == "/youtube/v3/search" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithYouTubeOptions(YouTubeOptions{Provider: "api", APIKey: "yt-test", APIURL: srv.URL + "/youtube/v3"})
	r := chi.NewRouter()
	h.Mount(r)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/channels/streamer/youtube?handle=streamer", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	var body model.YouTubeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Channel != "streamer" || body.YouTube == nil || body.YouTube.Title != "Streamer YT" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestRedditLSFCachedMissAutoWarms(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	h := New(newTestCache(), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "off"})
	h.scraperAPIKey = ""

	posts, sources := h.fetchRedditLSFCached(context.Background(), "ohnepixel", "7d", "top", false)
	if len(posts) != 0 {
		t.Fatalf("expected no posts on cache miss, got %d", len(posts))
	}
	if len(sources) != 1 || !strings.Contains(sources[0].Message, "fetching from Reddit") {
		t.Fatalf("expected warming source on cache miss, got %+v", sources)
	}
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected background warm on cache miss")
	}
}

func TestRedditLSFCachedStaleRefreshReturnsPostsWhileWarming(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	store := &testStore{data: map[string][]byte{}}
	key := redditLSFCacheKey("ohnepixel", "7d", "top")
	stalePosts := []model.RedditPost{{ID: "abc123", Title: "Stale clip", Provider: "public_json"}}
	payload, err := json.Marshal(redditLSFCacheValue{
		Items:     stalePosts,
		Sources:   []model.SourceStatus{{Source: "reddit_lsf", Provider: "public_json", State: "ready"}},
		Period:    "7d",
		Sort:      "top",
		UpdatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.data[key+":stale"] = payload

	h := New(cache.New(store, time.Minute, time.Hour), &fakeGQL{}).
		WithExternalSources("", srv.URL, "test-agent").
		WithRedditOptions(RedditOptions{BaseURL: srv.URL, Provider: "off"})
	h.scraperAPIKey = ""

	posts, sources := h.fetchRedditLSFCached(context.Background(), "ohnepixel", "7d", "top", true)
	if len(posts) != 1 || posts[0].ID != "abc123" {
		t.Fatalf("expected stale posts while warming, got %+v", posts)
	}
	if len(sources) != 1 || !strings.Contains(sources[0].Message, "fetching from Reddit") {
		t.Fatalf("expected warming source, got %+v", sources)
	}
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected background warm on stale refresh")
	}
}
