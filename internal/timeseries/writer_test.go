package timeseries

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type captureSink struct {
	mu      sync.Mutex
	batches int
	rollups int
	err     error
}

func (s *captureSink) WriteRollups(ctx context.Context, rollups []Rollup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches++
	s.rollups += len(rollups)
	return s.err
}

func TestNoopWriter(t *testing.T) {
	var w Writer = NoopWriter{}
	w.EnqueueRollups([]Rollup{{StreamID: "s1"}})
	if err := w.WriteRollups(context.Background(), []Rollup{{StreamID: "s1"}}); err != nil {
		t.Fatalf("WriteRollups: %v", err)
	}
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBuildInfluxLineProtocol(t *testing.T) {
	ts := time.Unix(1710000000, 0).UTC()
	started := time.Unix(1709999000, 0).UTC()
	body := BuildInfluxLineProtocol([]Rollup{{
		ChannelLogin:      "ohnepixel",
		StreamID:          "316963854947",
		StreamTitle:       "Opening 1000 cases",
		StreamCategory:    "Counter-Strike",
		StreamStartedAt:   started,
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
		"stream_activity_1m,channel_login=ohnepixel,stream_id=316963854947,stream_started=1709999000,stream_title=Opening\\ 1000\\ cases,stream_category=Counter-Strike viewer_avg=36900i,viewer_max=54100i,chat_count=658i,total_emote_count=833i,seventv_emote_count=555i,unique_emote_count=2i 1710000000",
		"emote_usage_1m,channel_login=ohnepixel,stream_id=316963854947,stream_started=1709999000,stream_title=Opening\\ 1000\\ cases,stream_category=Counter-Strike,provider=seventv,emote_id=abc123,emote_name=OMEGALUL count=42i 1710000000",
		"emote_usage_1m,channel_login=ohnepixel,stream_id=316963854947,stream_started=1709999000,stream_title=Opening\\ 1000\\ cases,stream_category=Counter-Strike,provider=twitch,emote_id=def456,emote_name=Kappa count=7i 1710000000",
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

func TestAsyncWriterWriteRollupsUpdatesStatus(t *testing.T) {
	sink := &captureSink{}
	w := &AsyncWriter{
		backend:      DefaultBackend,
		writeTimeout: time.Second,
		sink:         sink,
		status: Status{
			Enabled:    true,
			Configured: true,
			Backend:    DefaultBackend,
			State:      "idle",
		},
	}

	err := w.WriteRollups(context.Background(), []Rollup{{
		ChannelLogin: "xqc",
		StreamID:     "stream-1",
		MinuteTS:     time.Unix(1710000000, 0).UTC(),
		ChatCount:    12,
	}})
	if err != nil {
		t.Fatalf("WriteRollups: %v", err)
	}
	if sink.batches != 1 || sink.rollups != 1 {
		t.Fatalf("sink got batches=%d rollups=%d, want 1/1", sink.batches, sink.rollups)
	}
	status := w.Status()
	if status.State != "ready" || status.Attempts != 1 || status.Failures != 0 || status.LastWriteAt == nil {
		t.Fatalf("status = %+v, want ready successful write", status)
	}
}

func TestAsyncWriterBackfillStatusTransitions(t *testing.T) {
	w := &AsyncWriter{
		status: Status{
			Enabled:    true,
			Configured: true,
			Backend:    DefaultBackend,
			State:      "idle",
		},
	}

	w.StartBackfill(2, 5)
	w.AddBackfillProgress(3)
	status := w.Status()
	if status.BackfillState != "running" || status.BackfillStreams != 2 || status.BackfillRollups != 5 || status.BackfillExported != 3 || status.BackfillStartedAt == nil {
		t.Fatalf("running status = %+v", status)
	}

	w.FinishBackfill(nil)
	status = w.Status()
	if status.State != "ready" || status.BackfillState != "completed" || status.BackfillCompletedAt == nil || status.BackfillLastError != "" {
		t.Fatalf("completed status = %+v", status)
	}
}

func TestAsyncWriterBackfillFailureMarksStatus(t *testing.T) {
	w := &AsyncWriter{
		status: Status{
			Enabled:    true,
			Configured: true,
			Backend:    DefaultBackend,
			State:      "idle",
		},
	}

	w.StartBackfill(1, 2)
	w.FinishBackfill(errors.New("influx offline"))
	status := w.Status()
	if status.State != "degraded" || status.BackfillState != "failed" || status.BackfillLastError != "influx offline" || status.LastError != "influx offline" {
		t.Fatalf("failed status = %+v", status)
	}
}
