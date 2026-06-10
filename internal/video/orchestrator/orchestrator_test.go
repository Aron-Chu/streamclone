package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/upstream"
	"streamclone/internal/video/registry"
	"streamclone/internal/video/token"
	"streamclone/internal/video/usher"
)

type fakeStream struct {
	killed chan struct{}
	once   sync.Once
}

func newFakeStream() *fakeStream  { return &fakeStream{killed: make(chan struct{})} }
func (f *fakeStream) Wait() error { <-f.killed; return nil }
func (f *fakeStream) Kill()       { f.once.Do(func() { close(f.killed) }) }

type fakeToken struct{}

func (fakeToken) Live(context.Context, string) (token.Token, error) {
	return token.Token{Value: "v", Signature: "s"}, nil
}

type failingToken struct{ err error }

func (f failingToken) Live(context.Context, string) (token.Token, error) {
	return token.Token{}, f.err
}

type fakeUsher struct{}

func (fakeUsher) Discover(context.Context, string, string, string) ([]usher.Rendition, error) {
	return []usher.Rendition{{Name: "720p60"}}, nil
}

func newOrch(maxStreams int, spawned *int32) *Orchestrator {
	return New(Options{
		Token:       fakeToken{},
		Usher:       fakeUsher{},
		Registry:    registry.New(),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RTMPBase:    "mediamtx:1935",
		HLSBase:     "http://localhost:8888",
		MaxStreams:  maxStreams,
		IdleTimeout: time.Hour,
		Spawn: func(string, string, string, string, io.Writer) (registry.Streamer, error) {
			atomic.AddInt32(spawned, 1)
			return newFakeStream(), nil
		},
	})
}

func post(h *Orchestrator, path, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestStartDedupe(t *testing.T) {
	var spawned int32
	h := newOrch(5, &spawned)
	if rec := post(h, "/v1/stream/start", `{"channel":"ninja"}`); rec.Code != 200 {
		t.Fatalf("first start code %d", rec.Code)
	}
	if rec := post(h, "/v1/stream/start", `{"channel":"ninja"}`); rec.Code != 200 {
		t.Fatalf("second start code %d", rec.Code)
	}
	if spawned != 1 {
		t.Fatalf("expected 1 spawn, got %d", spawned)
	}
	s, _ := h.o.Registry.Get("ninja")
	if s.Listeners() != 2 {
		t.Fatalf("expected 2 listeners, got %d", s.Listeners())
	}
}

func TestStartCapacity(t *testing.T) {
	var spawned int32
	h := newOrch(1, &spawned)
	if rec := post(h, "/v1/stream/start", `{"channel":"chan_one"}`); rec.Code != 200 {
		t.Fatalf("chan_one code %d", rec.Code)
	}
	if rec := post(h, "/v1/stream/start", `{"channel":"chan_two"}`); rec.Code != 503 {
		t.Fatalf("expected 503 at capacity, got %d", rec.Code)
	}
}

func TestStartInvalidChannel(t *testing.T) {
	var spawned int32
	h := newOrch(5, &spawned)
	if rec := post(h, "/v1/stream/start", `{"channel":"ab"}`); rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if spawned != 0 {
		t.Fatal("invalid channel must not spawn")
	}
}

func TestStartTokenErrorStructuredJSON(t *testing.T) {
	var spawned int32
	h := newOrch(5, &spawned)
	h.o.Token = failingToken{err: fmt.Errorf("%w: schema changed", upstream.ErrPlaybackToken)}

	rec := post(h, "/v1/stream/start", `{"channel":"ninja"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "upstream_token_failed" || !body.Retryable {
		t.Fatalf("unexpected body: %+v", body)
	}
	if spawned != 0 {
		t.Fatal("token failures must not spawn workers")
	}
}

func TestStartHLSReadinessTimeout(t *testing.T) {
	hls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer hls.Close()

	var spawned int32
	h := New(Options{
		Token:           fakeToken{},
		Usher:           fakeUsher{},
		Registry:        registry.New(),
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RTMPBase:        "mediamtx:1935",
		HLSBase:         "http://localhost:8888",
		HLSProbeBase:    hls.URL,
		HLSProbeTimeout: 25 * time.Millisecond,
		MaxStreams:      5,
		IdleTimeout:     time.Hour,
		Spawn: func(string, string, string, string, io.Writer) (registry.Streamer, error) {
			atomic.AddInt32(&spawned, 1)
			return newFakeStream(), nil
		},
	})

	rec := post(h, "/v1/stream/start", `{"channel":"ninja"}`)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d: %s", rec.Code, rec.Body.String())
	}
	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "hls_not_ready" || !body.Retryable {
		t.Fatalf("unexpected body: %+v", body)
	}
	if spawned != 1 {
		t.Fatalf("expected one attempted spawn, got %d", spawned)
	}
}

func TestStartHLSReadinessTimeoutWhenVariantUnauthorized(t *testing.T) {
	oldInterval := hlsProbeInterval
	oldWindow := hlsStabilityWindow
	hlsProbeInterval = 5 * time.Millisecond
	hlsStabilityWindow = 20 * time.Millisecond
	defer func() {
		hlsProbeInterval = oldInterval
		hlsStabilityWindow = oldWindow
	}()

	hls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/index.m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nmain_stream.m3u8?session=stale\n"))
		case strings.HasSuffix(r.URL.Path, "/main_stream.m3u8"):
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer hls.Close()

	var spawned int32
	h := New(Options{
		Token:           fakeToken{},
		Usher:           fakeUsher{},
		Registry:        registry.New(),
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RTMPBase:        "mediamtx:1935",
		HLSBase:         "http://localhost:8888",
		HLSProbeBase:    hls.URL,
		HLSProbeTimeout: 60 * time.Millisecond,
		MaxStreams:      5,
		IdleTimeout:     time.Hour,
		Spawn: func(string, string, string, string, io.Writer) (registry.Streamer, error) {
			atomic.AddInt32(&spawned, 1)
			return newFakeStream(), nil
		},
	})

	rec := post(h, "/v1/stream/start", `{"channel":"ninja"}`)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d: %s", rec.Code, rec.Body.String())
	}
	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "hls_not_ready" || !body.Retryable {
		t.Fatalf("unexpected body: %+v", body)
	}
	if spawned != 1 {
		t.Fatalf("expected one attempted spawn, got %d", spawned)
	}
}

func TestReapReleasesCapacity(t *testing.T) {
	var spawned int32
	h := newOrch(1, &spawned)
	post(h, "/v1/stream/start", `{"channel":"chan_one"}`)
	s, _ := h.o.Registry.Get("chan_one")
	s.Leave("")

	for _, s := range h.o.Registry.Reap(time.Now().Add(2*time.Hour), h.o.IdleTimeout) {
		s.Stream().Kill()
		h.release(s)
	}
	if rec := post(h, "/v1/stream/start", `{"channel":"chan_two"}`); rec.Code != 200 {
		t.Fatalf("expected capacity freed, got %d", rec.Code)
	}
	if spawned != 2 {
		t.Fatalf("expected 2 spawns, got %d", spawned)
	}
}

func TestStatusSnapshot(t *testing.T) {
	var spawned int32
	h := newOrch(5, &spawned)
	post(h, "/v1/stream/start", `{"channel":"ninja"}`)

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest("GET", "/v1/stream/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"sessions"`) {
		t.Fatalf("expected sessions snapshot, got %s", rec.Body.String())
	}
}

func TestDiagnosticsActiveStreamIncludesProbeAndVersion(t *testing.T) {
	hls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-PART-INF:PART-TARGET=0.2\n#EXT-X-MEDIA-SEQUENCE:9\n"))
	}))
	defer hls.Close()

	var spawned int32
	h := New(Options{
		Token:          fakeToken{},
		Usher:          fakeUsher{},
		Registry:       registry.New(),
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		RTMPBase:       "mediamtx:1935",
		HLSBase:        "http://localhost:8888",
		HLSProbeBase:   hls.URL,
		MaxStreams:     5,
		IdleTimeout:    time.Hour,
		BackendVersion: "test-version",
		Spawn: func(string, string, string, string, io.Writer) (registry.Streamer, error) {
			atomic.AddInt32(&spawned, 1)
			return newFakeStream(), nil
		},
	})
	post(h, "/v1/stream/start", `{"channel":"ninja"}`)

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest("GET", "/v1/stream/diagnostics?channel=ninja", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	var body diagnosticsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Active || body.BackendVersion != "test-version" || !body.HLSProbe.Ready || body.HLSProbe.TargetDuration != "2" {
		t.Fatalf("unexpected diagnostics: %+v", body)
	}
	if body.LatencyMode != "stable" || body.LiveEdge != 3 {
		t.Fatalf("expected stable latency diagnostics, got mode=%q edge=%d", body.LatencyMode, body.LiveEdge)
	}
}

func TestStartLatencyModeMapsLiveEdge(t *testing.T) {
	hls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write([]byte("#EXTM3U\n"))
	}))
	defer hls.Close()

	var capturedLiveEdge string
	h := New(Options{
		Token:          fakeToken{},
		Usher:          fakeUsher{},
		Registry:       registry.New(),
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		RTMPBase:       "mediamtx:1935",
		HLSBase:        "http://localhost:8888",
		HLSProbeBase:   hls.URL,
		HLSProbeTimeout: 200 * time.Millisecond,
		MaxStreams:     5,
		IdleTimeout:    time.Hour,
		Spawn: func(_, _, _, liveEdge string, _ io.Writer) (registry.Streamer, error) {
			capturedLiveEdge = liveEdge
			return newFakeStream(), nil
		},
	})

	rec := post(h, "/v1/stream/start", `{"channel":"ninja","latency_mode":"instant"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedLiveEdge != "1" {
		t.Fatalf("expected live edge 1, got %q", capturedLiveEdge)
	}
	s, ok := h.o.Registry.Get("ninja")
	if !ok || s.LiveEdge != 1 || s.LatencyMode != "instant" {
		t.Fatalf("unexpected session: ok=%v session=%+v", ok, s)
	}
}

func TestSelectRenditionBestChoosesHighestQualityWhenUnsorted(t *testing.T) {
	rends := []usher.Rendition{
		{Name: "360p30", Group: "360p30", Width: 640, Height: 360, FrameRate: 30, Bandwidth: 627900},
		{Name: "160p30", Group: "160p30", Width: 284, Height: 160, FrameRate: 30, Bandwidth: 216299},
		{Name: "audio_only", Group: "audio_only", Bandwidth: 160000},
		{Name: "1080p60", Group: "1080p60", Width: 1920, Height: 1080, FrameRate: 60, Bandwidth: 8042999},
		{Name: "720p60", Group: "720p60", Width: 1280, Height: 720, FrameRate: 60, Bandwidth: 3322199},
	}

	selected := selectRendition(rends, "best")
	if selected == nil || selected.Name != "1080p60" {
		t.Fatalf("expected 1080p60, got %+v", selected)
	}
}
