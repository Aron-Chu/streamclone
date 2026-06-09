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

func TestCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"games":{"edges":[{"cursor":"c2","node":{"id":"2","name":"Fortnite","boxArtURL":"https://box.tv/%{width}x%{height}.jpg"}}]}}}`))
	}))
	defer srv.Close()

	page, err := newClient(srv).Categories(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "Fortnite" {
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
