package token

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamclone/internal/upstream"
)

func newTestClient(srv *httptest.Server) *Client {
	return New(upstream.Endpoints{TwitchGQLURL: srv.URL, TwitchClientID: "cid", UserAgent: "ua"})
}

func TestLiveSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OperationName string         `json:"operationName"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.OperationName != "PlaybackAccessTokenLive" {
			t.Fatalf("unexpected operation: %q", body.OperationName)
		}
		if _, ok := body.Variables["vodID"]; ok {
			t.Fatal("live token request must not send vodID")
		}
		if _, ok := body.Variables["isVod"]; ok {
			t.Fatal("live token request must not send isVod")
		}
		if body.Variables["login"] != "x" {
			t.Fatalf("unexpected login: %+v", body.Variables)
		}
		w.Write([]byte(`{"data":{"streamPlaybackAccessToken":{"value":"{\"v\":1}","signature":"abc"}}}`))
	}))
	defer srv.Close()

	tok, err := newTestClient(srv).Live(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Value != `{"v":1}` || tok.Signature != "abc" {
		t.Fatalf("unexpected token: %+v", tok)
	}
}

func TestLiveSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).Live(context.Background(), "x")
	if !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}

func TestLiveTokenCache(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Write([]byte(`{"data":{"streamPlaybackAccessToken":{"value":"{\"v\":1}","signature":"abc"}}}`))
	}))
	defer srv.Close()

	client := newTestClient(srv)
	ctx := context.Background()
	if _, err := client.Live(ctx, "ninja"); err != nil {
		t.Fatalf("first live: %v", err)
	}
	if _, err := client.Live(ctx, "ninja"); err != nil {
		t.Fatalf("second live: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", calls)
	}
}
