package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFetchHTMLRetriesTransientEOF(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"html": "<html><body>ok</body></html>",
			},
		})
	}))
	defer srv.Close()

	client := New(Config{URL: srv.URL})
	html, err := client.FetchHTML(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("FetchHTML: %v", err)
	}
	if html != "<html><body>ok</body></html>" {
		t.Fatalf("html = %q", html)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}
