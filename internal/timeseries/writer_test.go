package timeseries

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNoopWriter(t *testing.T) {
	var w Writer = NoopWriter{}
	w.EnqueueRollups([]Rollup{{StreamID: "s1"}})
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBuildInfluxLineProtocol(t *testing.T) {
	ts := time.Unix(1710000000, 0).UTC()
	body := BuildInfluxLineProtocol([]Rollup{{
		ChannelLogin:      "ohnepixel",
		StreamID:          "316963854947",
		MinuteTS:          ts,
		ViewerAvg:         36900,
		ViewerMax:         54100,
		ChatCount:         658,
		TotalEmoteCount:   833,
		SevenTVEmoteCount: 555,
		Emotes: map[string]int{
			"seventv:abc123:OMEGALUL": 42,
			"twitch:def456:Kappa":     7,
		},
	}})

	wantParts := []string{
		"stream_activity_1m,channel_login=ohnepixel,stream_id=316963854947 viewer_avg=36900i,viewer_max=54100i,chat_count=658i,total_emote_count=833i,seventv_emote_count=555i 1710000000",
		"emote_usage_1m,channel_login=ohnepixel,stream_id=316963854947,provider=seventv,emote_id=abc123 emote_name=\"OMEGALUL\",count=42i 1710000000",
		"emote_usage_1m,channel_login=ohnepixel,stream_id=316963854947,provider=twitch,emote_id=def456 emote_name=\"Kappa\",count=7i 1710000000",
	}
	for _, part := range wantParts {
		if !strings.Contains(body, part) {
			t.Fatalf("line protocol missing %q\nbody:\n%s", part, body)
		}
	}
}

func TestInfluxSinkWritesRequest(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sink := NewInfluxSink(InfluxConfig{
		URL:    server.URL,
		Token:  "secret",
		Org:    "streamclone",
		Bucket: "streamclone",
		Client: server.Client(),
	})
	err := sink.WriteRollups(context.Background(), []Rollup{{
		ChannelLogin: "xqc",
		StreamID:     "stream-1",
		MinuteTS:     time.Unix(1710000000, 0).UTC(),
		ChatCount:    12,
	}})
	if err != nil {
		t.Fatalf("WriteRollups: %v", err)
	}
	if gotPath != "/api/v2/write?bucket=streamclone&org=streamclone&precision=s" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAuth != "Token secret" {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
	if !strings.Contains(gotBody, "stream_activity_1m,channel_login=xqc,stream_id=stream-1") {
		t.Fatalf("unexpected body: %s", gotBody)
	}
}

func TestInfluxSinkReturnsErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad bucket", http.StatusBadRequest)
	}))
	defer server.Close()

	sink := NewInfluxSink(InfluxConfig{
		URL:    server.URL,
		Org:    "streamclone",
		Bucket: "streamclone",
		Client: server.Client(),
	})
	err := sink.WriteRollups(context.Background(), []Rollup{{
		ChannelLogin: "xqc",
		StreamID:     "stream-1",
		MinuteTS:     time.Unix(1710000000, 0).UTC(),
	}})
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("unexpected error: %v", err)
	}
}
