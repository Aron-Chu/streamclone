package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHelixTopLiveStreamsPreservesViewerOrder(t *testing.T) {
	t.Parallel()

	page1 := map[string]any{
		"data": []map[string]any{
			{
				"id": "100", "user_id": "u1", "user_login": "big",
				"user_name": "Big", "game_name": "Game", "title": "T1",
				"viewer_count": 50000, "started_at": "2026-07-04T12:00:00Z",
				"language": "en", "tags": []string{},
			},
			{
				"id": "200", "user_id": "u2", "user_login": "mid",
				"user_name": "Mid", "game_name": "Game", "title": "T2",
				"viewer_count": 20000, "started_at": "2026-07-04T12:00:00Z",
				"language": "en", "tags": []string{},
			},
		},
		"pagination": map[string]any{"cursor": "page2"},
	}
	page2 := map[string]any{
		"data": []map[string]any{
			{
				"id": "300", "user_id": "u3", "user_login": "small",
				"user_name": "Small", "game_name": "Game", "title": "T3",
				"viewer_count": 5000, "started_at": "2026-07-04T12:00:00Z",
				"language": "en", "tags": []string{},
			},
		},
		"pagination": map[string]any{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/streams":
			after := r.URL.Query().Get("after")
			var body map[string]any
			if after == "page2" {
				body = page2
			} else {
				body = page1
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewHelixClient(srv.URL, srv.URL+"/oauth2/token", "cid", "secret", "test")
	streams, err := client.TopLiveStreams(context.Background(), 3)
	if err != nil {
		t.Fatalf("TopLiveStreams() err = %v", err)
	}
	if len(streams) != 3 {
		t.Fatalf("len(streams) = %d, want 3", len(streams))
	}
	if streams[0].Login != "big" || streams[0].ViewerCount != 50000 {
		t.Fatalf("first stream = %+v", streams[0])
	}
	if streams[1].Login != "mid" || streams[1].ViewerCount != 20000 {
		t.Fatalf("second stream = %+v", streams[1])
	}
	if streams[2].Login != "small" || streams[2].ViewerCount != 5000 {
		t.Fatalf("third stream = %+v", streams[2])
	}
}

func TestHelixTopLiveStreamsDedupesLogin(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"data": []map[string]any{
			{"id": "1", "user_id": "u1", "user_login": "dup", "viewer_count": 100, "started_at": "2026-07-04T12:00:00Z"},
			{"id": "2", "user_id": "u1", "user_login": "dup", "viewer_count": 90, "started_at": "2026-07-04T12:00:00Z"},
		},
		"pagination": map[string]any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/streams":
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewHelixClient(srv.URL, srv.URL+"/oauth2/token", "cid", "secret", "test")
	streams, err := client.TopLiveStreams(context.Background(), 5)
	if err != nil {
		t.Fatalf("TopLiveStreams() err = %v", err)
	}
	if len(streams) != 1 || streams[0].ID != "1" {
		t.Fatalf("streams = %+v, want single deduped row", streams)
	}
}
