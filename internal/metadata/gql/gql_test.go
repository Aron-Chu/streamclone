package gql

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamclone/internal/upstream"
)

func newClient(srv *httptest.Server) *Client {
	return New(upstream.Endpoints{TwitchGQLURL: srv.URL, TwitchClientID: "cid", UserAgent: "ua"}, NewStaticProvider("cid", "ua"))
}

func TestTopStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"streams":{"edges":[{"cursor":"c1","node":{"id":"1","title":"T","viewersCount":100,"previewImageURL":"https://thumb.tv/%{width}x%{height}.jpg","broadcaster":{"login":"user1","displayName":"User1"},"game":{"name":"Fortnite"}}}]}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).TopStreams(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	s := page.Items[0]
	if s.Login != "user1" || s.Category != "Fortnite" || s.ViewersCount != 100 {
		t.Fatalf("unexpected stream: %+v", s)
	}
	if s.ThumbnailURL != "https://thumb.tv/{width}x{height}.jpg" {
		t.Fatalf("thumbnail not normalized: %s", s.ThumbnailURL)
	}
	if page.Cursor != "c1" {
		t.Fatalf("expected cursor c1, got %s", page.Cursor)
	}
}

func TestTopStreamsSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).TopStreams(context.Background(), 10, "")
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestTopStreamsToleratesBadNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"streams":{"edges":[
			{"cursor":"c0","node":null},
			{"cursor":"c1","node":{"id":"1","title":null,"viewersCount":100,"previewImageURL":null,"broadcaster":{"login":"good1","displayName":"Good1"},"game":null}},
			{"cursor":"c2","node":{"id":"2","title":"No game","viewersCount":50,"previewImageURL":"https://thumb.tv/a.jpg","broadcaster":{"login":"good2","displayName":"Good2"}}},
			{"cursor":"c3","node":{"id":"3","title":"Missing bc","viewersCount":10,"previewImageURL":"https://thumb.tv/b.jpg","broadcaster":null}},
			{"cursor":"c4","node":{"id":"4","viewersCount":"not-a-number","previewImageURL":"https://thumb.tv/c.jpg","broadcaster":{"login":"bad","displayName":"Bad"}}}
		]}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).TopStreams(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(page.Items), page.Items)
	}
	if page.Items[0].Login != "good1" || page.Items[0].Category != "" {
		t.Fatalf("unexpected first stream: %+v", page.Items[0])
	}
	if page.Items[1].Login != "good2" || page.Items[1].Category != "" {
		t.Fatalf("unexpected second stream: %+v", page.Items[1])
	}
	if page.Cursor != "c4" {
		t.Fatalf("expected cursor from last edge c4, got %s", page.Cursor)
	}
}

func TestTopStreamsMultiPageCursor(t *testing.T) {
	const page1 = `{"data":{"streams":{"edges":[`
	const page1Body = `{"cursor":"page1-end","node":{"id":"101","title":"Page1","viewersCount":9000,"previewImageURL":"https://thumb.tv/p1.jpg","broadcaster":{"login":"p1stream","displayName":"P1"},"game":{"name":"Fortnite"}}}`
	const page2 = `{"data":{"streams":{"edges":[`
	// Page-2 fixture mimics live Twitch: null game, null preview, missing broadcaster, one good node.
	const page2Body = `
		{"cursor":"page2-skip","node":null},
		{"cursor":"page2-null-game","node":{"id":"201","title":"No category","viewersCount":800,"previewImageURL":null,"broadcaster":{"login":"p2a","displayName":"P2A"},"game":null}},
		{"cursor":"page2-no-bc","node":{"id":"202","title":"Ghost","viewersCount":700,"previewImageURL":"https://thumb.tv/x.jpg","broadcaster":null}},
		{"cursor":"page2-end","node":{"id":"203","title":"Page2 good","viewersCount":600,"previewImageURL":"https://thumb.tv/p2.jpg","broadcaster":{"login":"p2b","displayName":"P2B"},"game":{"name":"Just Chatting"}}}
	`

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1:
			w.Write([]byte(page1 + page1Body + `]}}}`))
		case 2:
			w.Write([]byte(page2 + page2Body + `]}}}`))
		default:
			t.Errorf("unexpected request count %d", call)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := newClient(srv)
	page1Result, err := client.TopStreams(context.Background(), 25, "")
	if err != nil {
		t.Fatalf("page 1 error: %v", err)
	}
	if len(page1Result.Items) != 1 || page1Result.Items[0].Login != "p1stream" {
		t.Fatalf("unexpected page 1: %+v", page1Result)
	}
	if page1Result.Cursor != "page1-end" {
		t.Fatalf("expected page1-end cursor, got %s", page1Result.Cursor)
	}

	page2Result, err := client.TopStreams(context.Background(), 25, page1Result.Cursor)
	if err != nil {
		t.Fatalf("page 2 error: %v", err)
	}
	if len(page2Result.Items) != 2 {
		t.Fatalf("expected 2 page-2 items, got %d: %+v", len(page2Result.Items), page2Result.Items)
	}
	if page2Result.Items[0].Login != "p2a" || page2Result.Items[0].Category != "" {
		t.Fatalf("unexpected first page-2 stream: %+v", page2Result.Items[0])
	}
	if page2Result.Items[1].Login != "p2b" || page2Result.Items[1].Category != "Just Chatting" {
		t.Fatalf("unexpected second page-2 stream: %+v", page2Result.Items[1])
	}
	if page2Result.Cursor != "page2-end" {
		t.Fatalf("expected page2-end cursor, got %s", page2Result.Cursor)
	}
	if call != 2 {
		t.Fatalf("expected 2 gql calls, got %d", call)
	}
}

func TestCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"games":{"edges":[{"cursor":"c2","node":{"id":"2","name":"Fortnite","viewersCount":123456,"boxArtURL":"https://box.tv/%{width}x%{height}.jpg"}}]}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).Categories(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "Fortnite" {
		t.Fatalf("unexpected categories: %+v", page.Items)
	}
	if page.Items[0].Viewers != 123456 {
		t.Fatalf("unexpected category viewers: %+v", page.Items[0])
	}
	if page.Items[0].ThumbnailURL != "https://box.tv/{width}x{height}.jpg" {
		t.Fatalf("category thumbnail not normalized: %s", page.Items[0].ThumbnailURL)
	}
}

func TestCategoriesMissingViewersDefaultsToZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"games":{"edges":[{"cursor":"c2","node":{"id":"2","name":"Fortnite","boxArtURL":"https://box.tv/%{width}x%{height}.jpg"}}]}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).Categories(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Viewers != 0 {
		t.Fatalf("unexpected categories: %+v", page.Items)
	}
}

func TestCategoriesSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).Categories(context.Background(), 10, "")
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestCategoryStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"game":{"streams":{"edges":[{"cursor":"c3","node":{"id":"3","title":"Play","viewersCount":50,"previewImageURL":"https://t.tv/img.jpg","broadcaster":{"login":"user2","displayName":"User2"}}}]}}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).CategoryStreams(context.Background(), "2", 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Login != "user2" {
		t.Fatalf("unexpected streams: %+v", page.Items)
	}
}

func TestCategoryStreamsSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).CategoryStreams(context.Background(), "2", 10, "")
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"searchFor":{"channels":{"items":[{"id":"4","login":"u3","displayName":"U3","profileImageURL":"https://avatar.tv/u3.png","stream":{"id":"9001","title":"Live title","viewersCount":1234,"previewImageURL":"https://thumb.tv/%{width}x%{height}.jpg","game":{"name":"Just Chatting"}}},{"id":"5","login":"off","displayName":"Offline","profileImageURL":"https://avatar.tv/off.png"}]},"games":{"items":[{"id":"g1","name":"Fortnite","boxArtURL":"https://box.tv/%{width}x%{height}.jpg"}]}}}}`))
	}))
	defer srv.Close()

	res, err := newClient(srv).Search(context.Background(), "u3", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Streams) != 2 {
		t.Fatalf("unexpected search streams: %+v", res.Streams)
	}
	live := res.Streams[0]
	if live.Login != "u3" || !live.IsLive || live.ViewersCount != 1234 || live.Title != "Live title" {
		t.Fatalf("unexpected live search result: %+v", live)
	}
	if live.ThumbnailURL != "https://thumb.tv/{width}x{height}.jpg" {
		t.Fatalf("live thumbnail not normalized: %s", live.ThumbnailURL)
	}
	offline := res.Streams[1]
	if offline.Login != "off" || offline.IsLive || offline.ProfileImageURL == "" {
		t.Fatalf("unexpected offline search result: %+v", offline)
	}
	if len(res.Categories) != 1 || res.Categories[0].Name != "Fortnite" {
		t.Fatalf("unexpected search categories: %+v", res.Categories)
	}
	if res.Categories[0].ThumbnailURL != "https://box.tv/{width}x{height}.jpg" {
		t.Fatalf("category thumbnail not normalized: %s", res.Categories[0].ThumbnailURL)
	}
}

func TestSearchSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).Search(context.Background(), "x", 10)
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"user":{"id":"99","login":"streamer","displayName":"Streamer"}}}`))
	}))
	defer srv.Close()

	ch, err := newClient(srv).Channel(context.Background(), "streamer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch.ID != "99" || ch.Login != "streamer" {
		t.Fatalf("unexpected channel: %+v", ch)
	}
}

func TestChannelSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"user":null}}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).Channel(context.Background(), "nobody")
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestForbiddenRetry(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		if count == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"data":{"user":{"id":"1","login":"x","displayName":"X"}}}`))
	}))
	defer srv.Close()

	ch, err := newClient(srv).Channel(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch.ID != "1" {
		t.Fatalf("unexpected channel: %+v", ch)
	}
	if count != 2 {
		t.Fatalf("expected 2 requests, got %d", count)
	}
}
