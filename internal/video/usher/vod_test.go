package usher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamclone/internal/upstream"
)

func TestDiscoverVodSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/vod/1234567890.m3u8" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080,FRAME-RATE=30.000
720p60.m3u8
`))
	}))
	defer srv.Close()

	client := New(upstream.Endpoints{TwitchUsherURL: srv.URL, TwitchClientID: "cid", UserAgent: "ua"})
	rends, err := client.DiscoverVod(context.Background(), "1234567890", "tok", "sig")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rends) != 1 || rends[0].Height != 1080 {
		t.Fatalf("unexpected renditions: %+v", rends)
	}
}

func TestDiscoverVodUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := New(upstream.Endpoints{TwitchUsherURL: srv.URL, TwitchClientID: "cid", UserAgent: "ua"})
	_, err := client.DiscoverVod(context.Background(), "1234567890", "tok", "sig")
	if err != ErrVodUnavailable {
		t.Fatalf("expected ErrVodUnavailable, got %v", err)
	}
}
