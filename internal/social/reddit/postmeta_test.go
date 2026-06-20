package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPostJSONURL(t *testing.T) {
	got := postJSONURL("https://old.reddit.com/r/LivestreamFail/comments/1uaby3n/example_title/")
	want := "https://old.reddit.com/r/LivestreamFail/comments/1uaby3n/example_title.json?raw_json=1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

type redditTestTransport struct {
	base     http.RoundTripper
	testHost string
}

func (t redditTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.EqualFold(req.URL.Hostname(), "old.reddit.com") {
		req = req.Clone(req.Context())
		req.URL.Scheme = "http"
		req.URL.Host = t.testHost
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func TestFetchPostMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !stringsHasSuffix(r.URL.Path, ".json") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{
			"data": {
				"children": [{
					"data": {
						"thumbnail": "https://preview.redd.it/abc.jpg",
						"url": "https://clips.twitch.tv/ExampleClipSlug",
						"score": 591,
						"num_comments": 20,
						"preview": {
							"images": [{
								"source": {"url": "https://preview.redd.it/hero.jpg?width=640\u0026format=pjpg"}
							}]
						}
					}
				}]
			}
		}]`))
	}))
	defer srv.Close()

	testURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: redditTestTransport{testHost: testURL.Host}}
	postURL := "https://old.reddit.com/r/LivestreamFail/comments/1uaby3n/example/"
	meta, ok := FetchPostMeta(context.Background(), client, "test/1.0", postURL)
	if !ok {
		t.Fatal("expected metadata")
	}
	if meta.Thumbnail != "https://preview.redd.it/hero.jpg?width=640&format=pjpg" {
		t.Fatalf("thumb=%q", meta.Thumbnail)
	}
	if meta.Score != 591 || meta.Comments != 20 {
		t.Fatalf("score=%d comments=%d", meta.Score, meta.Comments)
	}
	if meta.ExternalURL != "https://clips.twitch.tv/ExampleClipSlug" {
		t.Fatalf("external=%q", meta.ExternalURL)
	}
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
