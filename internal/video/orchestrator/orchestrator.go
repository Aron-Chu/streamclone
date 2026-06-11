package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"streamclone/internal/metrics"
	"streamclone/internal/resilience"
	"streamclone/internal/upstream"
	"streamclone/internal/video/registry"
	"streamclone/internal/video/token"
	"streamclone/internal/video/usher"
	"streamclone/internal/video/worker"
)

var (
	errBusy                       = errors.New("capacity reached")
	errHLSNotReady                = errors.New("hls playlist not ready")
	errDirectHLSSourceUnavailable = errors.New("direct hls source unavailable for selected quality")
	hlsProbeInterval              = 100 * time.Millisecond
	hlsStabilityWindow            = 100 * time.Millisecond
	backendProbeFastTimeout       = 5 * time.Second
)

type SpawnFunc func(channel, quality, rtmp, liveEdge string, logw io.Writer) (registry.Streamer, error)
type DirectSpawnFunc func(channel, sourceURL, rtmp string, logw io.Writer) (registry.Streamer, error)

type TokenClient interface {
	Live(ctx context.Context, login string) (token.Token, error)
}

type UsherClient interface {
	Discover(ctx context.Context, login, tokenValue, signature string) ([]usher.Rendition, error)
}

type Options struct {
	Token           TokenClient
	Usher           UsherClient
	Registry        *registry.Registry
	Log             *slog.Logger
	RTMPBase        string
	HLSBase         string
	HLSProbeBase    string
	HLSProbeTimeout time.Duration
	MaxStreams      int
	MaxRestarts     int
	IdleTimeout     time.Duration
	BackendVersion  string
	WorkerBackends  []string
	DefaultQuality  string
	Spawn           SpawnFunc
	DirectSpawn     DirectSpawnFunc
}

type Orchestrator struct {
	o       Options
	sf      singleflight.Group
	sem     chan struct{}
	breaker *resilience.Breaker
}

func New(o Options) *Orchestrator {
	if o.Spawn == nil {
		o.Spawn = func(ch, q, rtmp, liveEdge string, logw io.Writer) (registry.Streamer, error) {
			return worker.Start(ch, q, rtmp, liveEdge, logw)
		}
	}
	if o.DirectSpawn == nil {
		o.DirectSpawn = func(ch, src, rtmp string, logw io.Writer) (registry.Streamer, error) {
			return worker.StartDirectHLS(ch, src, rtmp, logw)
		}
	}
	if len(o.WorkerBackends) == 0 {
		o.WorkerBackends = []string{"direct_hls", "streamlink"}
	}
	if o.DefaultQuality == "" {
		o.DefaultQuality = "best"
	}
	if o.MaxRestarts == 0 {
		o.MaxRestarts = 3
	}
	if o.MaxStreams < 1 {
		o.MaxStreams = 1
	}
	if o.HLSProbeTimeout == 0 {
		o.HLSProbeTimeout = 15 * time.Second
	}
	if o.BackendVersion == "" {
		o.BackendVersion = "dev"
	}
	return &Orchestrator{o: o, sem: make(chan struct{}, o.MaxStreams), breaker: resilience.NewBreaker(5, 30*time.Second)}
}

func (h *Orchestrator) Routes(r chi.Router) {
	r.Post("/v1/stream/start", h.start)
	r.Post("/v1/stream/keepalive", h.keepalive)
	r.Post("/v1/stream/stop", h.stop)
	r.Get("/v1/stream/status", h.status)
	r.Get("/v1/stream/diagnostics", h.diagnostics)
	r.Get("/v1/stream/proxy", h.proxyPlaylist)
}

type startReq struct {
	Channel     string `json:"channel"`
	Quality     string `json:"quality"`
	LatencyMode string `json:"latency_mode"`
}

type sessionReq struct {
	Channel   string `json:"channel"`
	SessionID string `json:"session_id"`
}

type statusResp struct {
	Channel           string                    `json:"channel"`
	HLSURL            string                    `json:"hlsUrl"`
	Quality           string                    `json:"quality"`
	Listeners         int64                     `json:"listeners"`
	LastSeen          int64                     `json:"lastSeen"`
	StartedAt         int64                     `json:"startedAt"`
	Renditions        []usher.Rendition         `json:"renditions"`
	SelectedRendition *usher.Rendition          `json:"selectedRendition,omitempty"`
	WorkerBackend     string                    `json:"workerBackend,omitempty"`
	StartupMs         int64                     `json:"startupMs,omitempty"`
	StartupBreakdown  registry.StartupBreakdown `json:"startupBreakdown,omitempty"`
	FallbackAttempted bool                      `json:"fallbackAttempted"`
	QualityRestarted  bool                      `json:"qualityRestarted"`
	SessionID         string                    `json:"session_id,omitempty"`
}

type diagnosticsResp struct {
	Channel           string                    `json:"channel"`
	Active            bool                      `json:"active"`
	HLSURL            string                    `json:"hlsUrl,omitempty"`
	Quality           string                    `json:"quality,omitempty"`
	Listeners         int64                     `json:"listeners,omitempty"`
	LastSeen          int64                     `json:"lastSeen,omitempty"`
	StartedAt         int64                     `json:"startedAt,omitempty"`
	UptimeMs          int64                     `json:"uptimeMs,omitempty"`
	WorkerStarted     int64                     `json:"workerStartedAt,omitempty"`
	WorkerUptimeMs    int64                     `json:"workerUptimeMs,omitempty"`
	LatencyMode       string                    `json:"latencyMode"`
	LiveEdge          int                       `json:"liveEdge,omitempty"`
	Restarts          int64                     `json:"restarts"`
	MaxRestarts       int                       `json:"maxRestarts"`
	LastRestartAt     int64                     `json:"lastRestartAt,omitempty"`
	LastWorkerErr     string                    `json:"lastWorkerError,omitempty"`
	LastStartError    string                    `json:"lastStartError,omitempty"`
	Stopped           bool                      `json:"stopped"`
	BackendVersion    string                    `json:"backendVersion"`
	RenderProtocol    string                    `json:"protocol"`
	Renditions        []usher.Rendition         `json:"renditions,omitempty"`
	SelectedRendition *usher.Rendition          `json:"selectedRendition,omitempty"`
	WorkerBackend     string                    `json:"workerBackend,omitempty"`
	StartupMs         int64                     `json:"startupMs,omitempty"`
	StartupBreakdown  registry.StartupBreakdown `json:"startupBreakdown,omitempty"`
	FallbackAttempts  int                       `json:"fallbackAttempts"`
	HLSProbe          hlsProbeResp              `json:"hlsProbe"`
	UpdatedAt         int64                     `json:"updatedAt"`
}

type hlsProbeResp struct {
	URL             string `json:"url,omitempty"`
	Ready           bool   `json:"ready"`
	StatusCode      int    `json:"statusCode,omitempty"`
	DurationMs      int64  `json:"durationMs,omitempty"`
	ContentType     string `json:"contentType,omitempty"`
	TargetDuration  string `json:"targetDuration,omitempty"`
	PartTarget      string `json:"partTarget,omitempty"`
	MediaSequence   string `json:"mediaSequence,omitempty"`
	Error           string `json:"error,omitempty"`
	PlaylistSummary string `json:"playlistSummary,omitempty"`
}

func toResp(s *registry.Session, sessionID string) statusResp {
	return statusResp{
		Channel:           s.Channel,
		HLSURL:            s.HLSURL,
		Quality:           s.Quality,
		Listeners:         s.Listeners(),
		LastSeen:          s.LastSeen().UnixMilli(),
		StartedAt:         s.StartedAt.UnixMilli(),
		Renditions:        s.Renditions,
		SelectedRendition: s.SelectedRendition,
		WorkerBackend:     s.WorkerBackend,
		StartupMs:         s.StartupMs,
		StartupBreakdown:  s.StartupBreakdown,
		FallbackAttempted: s.FallbackAttempted,
		QualityRestarted:  s.QualityRestarted,
		SessionID:         sessionID,
	}
}

func (h *Orchestrator) start(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req startReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		metrics.StreamStartFailures.WithLabelValues("bad_request").Inc()
		metrics.StreamStartDuration.WithLabelValues("bad_request", "none").Observe(time.Since(started).Seconds())
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !worker.ValidChannel(req.Channel) {
		metrics.StreamStartFailures.WithLabelValues("invalid_channel").Inc()
		metrics.StreamStartDuration.WithLabelValues("invalid_channel", "none").Observe(time.Since(started).Seconds())
		http.Error(w, "invalid channel", http.StatusBadRequest)
		return
	}
	req.Quality = effectiveQuality(req.Quality, h.o.DefaultQuality)
	latencyMode, liveEdge := parseLatencyMode(req.LatencyMode)
	qualityRestarted := false
	if s, ok := h.o.Registry.Get(req.Channel); ok {
		if s.Quality != req.Quality || s.LatencyMode != latencyMode {
			qualityRestarted = true
			h.stopExistingForQualityChange(s)
		} else {
			sessionID := newSessionID()
			s.AddListener(sessionID)
			metrics.StreamListeners.WithLabelValues(req.Channel).Set(float64(s.Listeners()))
			metrics.StreamStartDuration.WithLabelValues("ok", s.WorkerBackend).Observe(time.Since(started).Seconds())
			writeJSON(w, http.StatusOK, toResp(s, sessionID))
			return
		}
	}
	sfKey := req.Channel + ":" + req.Quality
	v, err, _ := h.sf.Do(sfKey, func() (any, error) {
		return h.create(r.Context(), req.Channel, req.Quality, latencyMode, liveEdge, qualityRestarted)
	})
	if err != nil {
		code := h.startErrorCode(err)
		metrics.StreamStartFailures.WithLabelValues(code).Inc()
		metrics.StreamStartDuration.WithLabelValues(code, "none").Observe(time.Since(started).Seconds())
		h.writeStartError(w, req.Channel, err)
		return
	}
	s := v.(*registry.Session)
	sessionID := newSessionID()
	s.AddListener(sessionID)
	metrics.StreamListeners.WithLabelValues(req.Channel).Set(float64(s.Listeners()))
	metrics.StreamStartDuration.WithLabelValues("ok", s.WorkerBackend).Observe(time.Since(started).Seconds())
	writeJSON(w, http.StatusOK, toResp(s, sessionID))
}

func (h *Orchestrator) stopExistingForQualityChange(s *registry.Session) {
	s.MarkStopped()
	h.o.Registry.Remove(s.Channel)
	metrics.StreamListeners.WithLabelValues(s.Channel).Set(0)
	if st := s.Stream(); st != nil {
		st.Kill()
	}
	h.release(s)
}

func (h *Orchestrator) startErrorCode(err error) string {
	switch {
	case errors.Is(err, usher.ErrChannelOffline):
		return "channel_offline"
	case errors.Is(err, errBusy):
		return "capacity_reached"
	case errors.Is(err, worker.ErrInvalidChannel):
		return "invalid_channel"
	case errors.Is(err, upstream.ErrPlaybackToken):
		return "upstream_token_failed"
	case errors.Is(err, errHLSNotReady):
		return "hls_not_ready"
	default:
		return "stream_start_failed"
	}
}

func (h *Orchestrator) writeStartError(w http.ResponseWriter, channel string, err error) {
	switch {
	case errors.Is(err, usher.ErrChannelOffline):
		writeAPIError(w, http.StatusNotFound, "channel_offline", "channel offline", false)
	case errors.Is(err, errBusy):
		writeAPIError(w, http.StatusServiceUnavailable, "capacity_reached", "stream capacity reached", true)
	case errors.Is(err, worker.ErrInvalidChannel):
		writeAPIError(w, http.StatusBadRequest, "invalid_channel", "invalid channel", false)
	case errors.Is(err, upstream.ErrPlaybackToken):
		h.o.Log.Error("playback token failed", "channel", channel, "err", err)
		writeAPIError(w, http.StatusBadGateway, "upstream_token_failed", err.Error(), true)
	case errors.Is(err, errHLSNotReady):
		h.o.Log.Error("hls readiness failed", "channel", channel, "err", err)
		writeAPIError(w, http.StatusGatewayTimeout, "hls_not_ready", "local HLS relay did not become ready", true)
	default:
		h.o.Log.Error("stream start failed", "channel", channel, "err", err)
		writeAPIError(w, http.StatusBadGateway, "stream_start_failed", "stream start failed: "+err.Error(), true)
	}
}

func (h *Orchestrator) create(ctx context.Context, channel, quality, latencyMode string, liveEdge int, qualityRestarted bool) (*registry.Session, error) {
	startupStartedAt := time.Now()
	select {
	case h.sem <- struct{}{}:
	default:
		return nil, errBusy
	}
	committed := false
	defer func() {
		if !committed {
			<-h.sem
		}
	}()

	upstreamStartedAt := time.Now()
	var tok token.Token
	tokenStartedAt := time.Now()
	terr := h.breaker.Do(func() error {
		return resilience.Retry(ctx, 2, 200*time.Millisecond, func() error {
			var e error
			tok, e = h.o.Token.Live(ctx, channel)
			return e
		})
	})
	if terr != nil {
		metrics.UpstreamRequests.WithLabelValues("token", "error").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues("token", "error").Observe(time.Since(tokenStartedAt).Seconds())
		return nil, terr
	}
	metrics.UpstreamRequests.WithLabelValues("token", "ok").Inc()
	metrics.UpstreamRequestDuration.WithLabelValues("token", "ok").Observe(time.Since(tokenStartedAt).Seconds())
	usherStartedAt := time.Now()
	rends, err := h.o.Usher.Discover(ctx, channel, tok.Value, tok.Signature)
	if err != nil {
		metrics.UpstreamRequests.WithLabelValues("usher", "error").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues("usher", "error").Observe(time.Since(usherStartedAt).Seconds())
		return nil, err
	}
	metrics.UpstreamRequests.WithLabelValues("usher", "ok").Inc()
	metrics.UpstreamRequestDuration.WithLabelValues("usher", "ok").Observe(time.Since(usherStartedAt).Seconds())
	selected := selectRendition(rends, quality)
	upstreamFetchMs := time.Since(upstreamStartedAt).Milliseconds()
	rtmp := "rtmp://" + h.o.RTMPBase + "/live/" + channel
	st, backend, fallbackAttempted, fallbackAttempts, lastStartErr, startupBreakdown, err := h.startWorker(ctx, channel, quality, rtmp, liveEdge, latencyMode, selected)
	if err != nil {
		return nil, err
	}
	startupMs := time.Since(startupStartedAt).Milliseconds()
	startupBreakdown.UpstreamFetchMs = upstreamFetchMs
	startupBreakdown.TotalMs = startupMs
	now := time.Now()
	s := &registry.Session{
		Channel:           channel,
		Quality:           quality,
		HLSURL:            h.o.HLSBase + "/live/" + channel + "/index.m3u8",
		Renditions:        rends,
		SelectedRendition: selected,
		WorkerBackend:     backend,
		StartupMs:         startupMs,
		StartupBreakdown:  startupBreakdown,
		FallbackAttempted: fallbackAttempted,
		FallbackAttempts:  fallbackAttempts,
		LastStartError:    lastStartErr,
		QualityRestarted:  qualityRestarted,
		LatencyMode:       latencyMode,
		LiveEdge:          liveEdge,
		StartedAt:         now,
	}
	s.MarkWorkerStart(now)
	s.SetStream(st)
	h.o.Registry.Add(s)
	committed = true
	metrics.StreamsActive.Inc()
	go h.supervise(s, channel, quality, rtmp)
	return s, nil
}

func (h *Orchestrator) startWorker(ctx context.Context, channel, quality, rtmp string, liveEdge int, latencyMode string, selected *usher.Rendition) (registry.Streamer, string, bool, int, string, registry.StartupBreakdown, error) {
	var lastErr error
	fallbackAttempts := 0
	liveEdgeStr := strconv.Itoa(liveEdge)
	backends := normalizeBackends(h.o.WorkerBackends)
	stabilityWindow, skipVariant := hlsProbeTuning(latencyMode)
	for i, backend := range backends {
		if i > 0 {
			fallbackAttempts++
		}
		spawnStartedAt := time.Now()
		st, err := h.spawnBackend(channel, quality, rtmp, backend, liveEdgeStr, selected)
		spawnMs := time.Since(spawnStartedAt).Milliseconds()
		if err != nil {
			if !errors.Is(err, errDirectHLSSourceUnavailable) || lastErr == nil {
				lastErr = err
			}
			continue
		}
		probeTimeout := backendProbeTimeout(h.o.HLSProbeTimeout, i, len(backends))
		hlsReadyStartedAt := time.Now()
		if err := waitForHLS(ctx, h.o.HLSProbeBase, channel, probeTimeout, stabilityWindow, skipVariant); err != nil {
			st.Kill()
			lastErr = err
			continue
		}
		hlsReadyMs := time.Since(hlsReadyStartedAt).Milliseconds()
		last := ""
		if lastErr != nil {
			last = lastErr.Error()
		}
		return st, backend, i > 0, fallbackAttempts, last, registry.StartupBreakdown{
			WorkerSpawnMs: spawnMs,
			HLSReadyMs:    hlsReadyMs,
		}, nil
	}
	if lastErr == nil {
		lastErr = errHLSNotReady
	}
	return nil, "", fallbackAttempts > 0, fallbackAttempts, lastErr.Error(), registry.StartupBreakdown{}, lastErr
}

func backendProbeTimeout(full time.Duration, backendIndex, backendCount int) time.Duration {
	if backendCount <= 1 || backendIndex >= backendCount-1 {
		return full
	}
	if full > backendProbeFastTimeout {
		return backendProbeFastTimeout
	}
	return full
}

func (h *Orchestrator) spawnBackend(channel, quality, rtmp, backend, liveEdge string, selected *usher.Rendition) (registry.Streamer, error) {
	switch backend {
	case "direct_hls":
		if selected == nil || selected.URL == "" {
			return nil, errDirectHLSSourceUnavailable
		}
		if _, err := allowedProxyURL(selected.URL); err != nil {
			return nil, fmt.Errorf("%w: %v", errDirectHLSSourceUnavailable, err)
		}
		localPort := "8080"
		if addr := os.Getenv("HTTP_ADDR"); addr != "" {
			if idx := strings.LastIndex(addr, ":"); idx >= 0 {
				localPort = addr[idx+1:]
			}
		}
		proxyURL := fmt.Sprintf("http://127.0.0.1:%s/v1/stream/proxy?url=%s", localPort, url.QueryEscape(selected.URL))
		h.o.Log.Info("spawning direct HLS worker with local manifest proxy", "channel", channel, "proxy_url", proxyURL)
		return h.o.DirectSpawn(channel, proxyURL, rtmp, logWriter{log: h.o.Log, ch: channel})
	default:
		return h.o.Spawn(channel, quality, rtmp, liveEdge, logWriter{log: h.o.Log, ch: channel})
	}
}

func normalizeBackends(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		if key != "streamlink" && key != "direct_hls" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return []string{"streamlink"}
	}
	return out
}

func effectiveQuality(requested, fallback string) string {
	q := strings.TrimSpace(requested)
	if q == "" {
		q = strings.TrimSpace(fallback)
	}
	if q == "" {
		return "best"
	}
	return q
}

func selectRendition(rends []usher.Rendition, quality string) *usher.Rendition {
	if len(rends) == 0 {
		return nil
	}
	tokens := strings.Split(quality, ",")
	for _, raw := range tokens {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if token == "best" || token == "source" || token == "chunked" {
			if r := firstSourceRendition(rends); r != nil {
				return r
			}
			return bestRendition(rends)
		}
		for _, r := range rends {
			name := strings.ToLower(r.Name)
			group := strings.ToLower(r.Group)
			if name == token || group == token || strings.Contains(name, token) || strings.Contains(group, token) {
				return copyRendition(r)
			}
			if r.Height > 0 && (token == strconv.Itoa(r.Height)+"p" || token == strconv.Itoa(r.Height)+"p"+frameRateSuffix(r.FrameRate)) {
				return copyRendition(r)
			}
		}
	}
	return bestRendition(rends)
}

func firstSourceRendition(rends []usher.Rendition) *usher.Rendition {
	for _, r := range rends {
		name := strings.ToLower(r.Name)
		group := strings.ToLower(r.Group)
		if strings.Contains(name, "source") || group == "chunked" {
			return copyRendition(r)
		}
	}
	return nil
}

func bestRendition(rends []usher.Rendition) *usher.Rendition {
	if len(rends) == 0 {
		return nil
	}
	best := rends[0]
	bestScore := renditionScore(best)
	for _, r := range rends[1:] {
		score := renditionScore(r)
		if score > bestScore {
			best = r
			bestScore = score
		}
	}
	return copyRendition(best)
}

func renditionScore(r usher.Rendition) int64 {
	fps := int64(r.FrameRate * 10)
	return int64(r.Height)*1_000_000_000 + int64(r.Width)*1_000_000 + fps*10_000 + int64(r.Bandwidth)
}

func frameRateSuffix(fps float64) string {
	if fps >= 59.5 {
		return "60"
	}
	return ""
}

func copyRendition(r usher.Rendition) *usher.Rendition {
	cp := r
	return &cp
}

func (h *Orchestrator) supervise(s *registry.Session, channel, quality, rtmp string) {
	restarts := 0
	backends := normalizeBackends(h.o.WorkerBackends)
	backendIdx := 0
	for i, backend := range backends {
		if backend == s.WorkerBackend {
			backendIdx = i
			break
		}
	}
	liveEdgeStr := strconv.Itoa(s.LiveEdge)
	if s.LiveEdge <= 0 {
		_, edge := parseLatencyMode(s.LatencyMode)
		liveEdgeStr = strconv.Itoa(edge)
	}
	stabilityWindow, skipVariant := hlsProbeTuning(s.LatencyMode)

	for {
		err := s.Stream().Wait()
		s.RecordWorkerError(err)
		if s.Stopped() {
			return
		}
		if restarts >= h.o.MaxRestarts {
			h.o.Log.Error("stream exceeded max restarts", "channel", channel, "err", err)
			break
		}
		restarts++
		s.RecordRestart(time.Now(), err)
		metrics.StreamRestarts.WithLabelValues(channel).Inc()
		h.o.Log.Warn("worker exited; restarting", "channel", channel, "attempt", restarts, "err", err)

		backoff := time.Duration(restarts) * 2 * time.Second
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		time.Sleep(backoff)

		var recovered bool
		for attempt := 0; attempt < len(backends); attempt++ {
			backendIdx = (backendIdx + 1) % len(backends)
			backend := backends[backendIdx]
			h.o.Log.Info("supervise restart attempt", "channel", channel, "backend", backend, "attempt", attempt+1)
			nst, serr := h.spawnBackend(channel, quality, rtmp, backend, liveEdgeStr, s.SelectedRendition)
			if serr != nil {
				h.o.Log.Error("restart spawn failed", "channel", channel, "backend", backend, "err", serr)
				s.RecordWorkerError(serr)
				continue
			}
			probeTimeout := backendProbeTimeout(h.o.HLSProbeTimeout, backendIdx, len(backends))
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			probeErr := waitForHLS(ctx, h.o.HLSProbeBase, channel, probeTimeout, stabilityWindow, skipVariant)
			cancel()
			if probeErr != nil {
				h.o.Log.Error("restart hls probe failed", "channel", channel, "backend", backend, "err", probeErr)
				nst.Kill()
				s.RecordWorkerError(probeErr)
				continue
			}
			s.SetStream(nst)
			s.WorkerBackend = backend
			s.MarkWorkerStart(time.Now())
			recovered = true
			break
		}
		if !recovered {
			h.o.Log.Error("restart failed after backend rotation", "channel", channel)
			break
		}
	}
	s.MarkStopped()
	h.o.Registry.Remove(channel)
	if st := s.Stream(); st != nil {
		st.Kill()
	}
	h.release(s)
}

func (h *Orchestrator) RunReaper(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			for _, s := range h.o.Registry.Reap(now, h.o.IdleTimeout) {
				if st := s.Stream(); st != nil {
					st.Kill()
				}
				h.release(s)
				metrics.StreamsReaped.Inc()
				metrics.StreamListeners.WithLabelValues(s.Channel).Set(0)
				h.o.Log.Info("reaped idle stream", "channel", s.Channel)
			}
		}
	}
}

func (h *Orchestrator) release(s *registry.Session) {
	if s.MarkReleased() {
		<-h.sem
		metrics.StreamsActive.Dec()
		metrics.StreamListeners.WithLabelValues(s.Channel).Set(0)
	}
}

func (h *Orchestrator) keepalive(w http.ResponseWriter, r *http.Request) {
	s, _, ok := h.lookup(r)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.Touch()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Orchestrator) stop(w http.ResponseWriter, r *http.Request) {
	s, sessionID, ok := h.lookup(r)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.Leave(sessionID)
	metrics.StreamListeners.WithLabelValues(s.Channel).Set(float64(s.Listeners()))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Orchestrator) status(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	if channel != "" {
		s, ok := h.o.Registry.Get(channel)
		if !ok {
			http.Error(w, "no active session", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, toResp(s, ""))
		return
	}
	sessions := h.o.Registry.Snapshot()
	out := make([]statusResp, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, toResp(s, ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *Orchestrator) diagnostics(w http.ResponseWriter, r *http.Request) {
	channel := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("channel")))
	if channel == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "missing_channel", Error: "channel is required", Retryable: false})
		return
	}
	if !worker.ValidChannel(channel) {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "invalid_channel", Error: "invalid channel", Retryable: false})
		return
	}
	now := time.Now()
	s, ok := h.o.Registry.Get(channel)
	if !ok {
		writeJSON(w, http.StatusOK, diagnosticsResp{
			Channel:        channel,
			Active:         false,
			MaxRestarts:    h.o.MaxRestarts,
			BackendVersion: h.o.BackendVersion,
			LatencyMode:    "stable",
			RenderProtocol: "HLS",
			HLSProbe:       probeHLS(r.Context(), h.o.HLSProbeBase, channel),
			UpdatedAt:      now.UnixMilli(),
		})
		return
	}
	workerStarted := s.WorkerStartedAt()
	lastRestart := s.LastRestartAt()
	resp := diagnosticsResp{
		Channel:           channel,
		Active:            true,
		HLSURL:            s.HLSURL,
		Quality:           s.Quality,
		Listeners:         s.Listeners(),
		LastSeen:          s.LastSeen().UnixMilli(),
		StartedAt:         s.StartedAt.UnixMilli(),
		UptimeMs:          now.Sub(s.StartedAt).Milliseconds(),
		Restarts:          s.Restarts(),
		MaxRestarts:       h.o.MaxRestarts,
		LastWorkerErr:     s.LastWorkerError(),
		LastStartError:    s.LastStartError,
		Stopped:           s.Stopped(),
		BackendVersion:    h.o.BackendVersion,
		LatencyMode:       latencyModeLabel(s.LatencyMode),
		LiveEdge:          s.LiveEdge,
		RenderProtocol:    "HLS",
		Renditions:        s.Renditions,
		SelectedRendition: s.SelectedRendition,
		WorkerBackend:     s.WorkerBackend,
		StartupMs:         s.StartupMs,
		StartupBreakdown:  s.StartupBreakdown,
		FallbackAttempts:  s.FallbackAttempts,
		HLSProbe:          probeHLS(r.Context(), h.o.HLSProbeBase, channel),
		UpdatedAt:         now.UnixMilli(),
	}
	if !workerStarted.IsZero() {
		resp.WorkerStarted = workerStarted.UnixMilli()
		resp.WorkerUptimeMs = now.Sub(workerStarted).Milliseconds()
	}
	if !lastRestart.IsZero() {
		resp.LastRestartAt = lastRestart.UnixMilli()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Orchestrator) lookup(r *http.Request) (*registry.Session, string, bool) {
	channel := r.URL.Query().Get("channel")
	sessionID := r.URL.Query().Get("session_id")
	if channel == "" {
		var req sessionReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		channel = req.Channel
		sessionID = req.SessionID
	}
	s, ok := h.o.Registry.Get(channel)
	if !ok {
		return nil, "", false
	}
	return s, sessionID, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Code      string `json:"code"`
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeJSON(w, status, apiError{Code: code, Error: message, Retryable: retryable})
}

func waitForHLS(ctx context.Context, base, channel string, timeout time.Duration, stabilityWindow time.Duration, skipVariant bool) error {
	if base == "" {
		return nil
	}
	started := time.Now()
	result := "timeout"
	defer func() {
		if result != "ok" {
			metrics.HLSReadinessFailures.Inc()
		}
		metrics.HLSProbeDuration.WithLabelValues(result).Observe(time.Since(started).Seconds())
	}()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := hlsProbeClient(3 * time.Second)
	playlist := strings.TrimRight(base, "/") + "/live/" + url.PathEscape(channel) + "/index.m3u8"
	ticker := time.NewTicker(hlsProbeInterval)
	defer ticker.Stop()
	stableSince := time.Time{}

	for {
		ready, err := probePlaylistGraph(ctx, client, playlist, skipVariant)
		if err == nil && ready {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stabilityWindow {
				result = "ok"
				return nil
			}
		} else {
			stableSince = time.Time{}
		}
		select {
		case <-ctx.Done():
			result = "timeout"
			return errHLSNotReady
		case <-ticker.C:
		}
	}
}

func probePlaylistGraph(ctx context.Context, client *http.Client, playlist string, skipVariant bool) (bool, error) {
	body, err := fetchPlaylistBody(ctx, client, playlist)
	if err != nil {
		return false, err
	}
	child := firstVariantPlaylist(body)
	if child == "" || skipVariant {
		return true, nil
	}
	childURL, err := resolvePlaylistReference(playlist, child)
	if err != nil {
		return false, err
	}
	if _, err := fetchPlaylistBody(ctx, client, childURL); err != nil {
		return false, err
	}
	return true, nil
}

func fetchPlaylistBody(ctx context.Context, client *http.Client, playlist string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlist, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playlist status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func hlsProbeClient(timeout time.Duration) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Timeout: timeout, Jar: jar}
}

func firstVariantPlaylist(body string) string {
	if !strings.Contains(body, "#EXT-X-STREAM-INF") {
		return ""
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func resolvePlaylistReference(baseURL, ref string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	child, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(child).String(), nil
}

func probeHLS(ctx context.Context, base, channel string) hlsProbeResp {
	if base == "" {
		return hlsProbeResp{Ready: false, Error: "hls probe base not configured"}
	}
	playlist := strings.TrimRight(base, "/") + "/live/" + url.PathEscape(channel) + "/index.m3u8"
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlist, nil)
	if err != nil {
		return hlsProbeResp{URL: playlist, Ready: false, Error: err.Error()}
	}
	client := hlsProbeClient(3 * time.Second)
	resp, err := client.Do(req)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		return hlsProbeResp{URL: playlist, Ready: false, DurationMs: duration, Error: err.Error()}
	}
	defer resp.Body.Close()
	out := hlsProbeResp{
		URL:         playlist,
		Ready:       resp.StatusCode == http.StatusOK,
		StatusCode:  resp.StatusCode,
		DurationMs:  duration,
		ContentType: resp.Header.Get("Content-Type"),
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	out.TargetDuration, out.PartTarget, out.MediaSequence, out.PlaylistSummary = summarizePlaylist(string(body))
	return out
}

func summarizePlaylist(body string) (targetDuration, partTarget, mediaSequence, summary string) {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, 5)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			targetDuration = strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:")
		}
		if strings.HasPrefix(line, "#EXT-X-PART-INF:PART-TARGET=") {
			partTarget = strings.TrimPrefix(line, "#EXT-X-PART-INF:PART-TARGET=")
		}
		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			mediaSequence = strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")
		}
		if len(kept) < 5 && strings.HasPrefix(line, "#EXT") {
			kept = append(kept, line)
		}
	}
	return targetDuration, partTarget, mediaSequence, strings.Join(kept, " ")
}

type logWriter struct {
	log *slog.Logger
	ch  string
}

func (l logWriter) Write(p []byte) (int, error) {
	l.log.Debug("subprocess", "channel", l.ch, "out", strings.TrimSpace(string(p)))
	return len(p), nil
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func (h *Orchestrator) proxyPlaylist(w http.ResponseWriter, r *http.Request) {
	sourceURL := r.URL.Query().Get("url")
	if sourceURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}
	if _, err := allowedProxyURL(sourceURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("upstream status %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cleanedPlaylist := filterTwitchAdSegments(string(bodyBytes), sourceURL)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(cleanedPlaylist))
}

type adRange struct {
	start time.Time
	end   time.Time
}

func filterTwitchAdSegments(body string, sourceURL string) string {
	lines := strings.Split(body, "\n")
	var adRanges []adRange

	// First pass: extract all active ad date ranges
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-DATERANGE:") {
			attrs := usher.ParseAttrs(strings.TrimPrefix(line, "#EXT-X-DATERANGE:"))
			if attrs["CLASS"] == "twitch-stitched-ad" {
				startStr := attrs["START-DATE"]
				durStr := attrs["DURATION"]
				if startStr != "" && durStr != "" {
					start, err := time.Parse(time.RFC3339Nano, startStr)
					if err == nil {
						dur, err := strconv.ParseFloat(durStr, 64)
						if err == nil {
							adRanges = append(adRanges, adRange{
								start: start,
								end:   start.Add(time.Duration(dur * float64(time.Second))),
							})
						}
					}
				}
			}
		}
	}

	// Second pass: output clean manifest
	var outLines []string
	var currentProgTime time.Time
	var pendingProgTimeLine string

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-DATERANGE:") {
			attrs := usher.ParseAttrs(strings.TrimPrefix(line, "#EXT-X-DATERANGE:"))
			if attrs["CLASS"] == "twitch-stitched-ad" {
				i++
				continue
			}
			outLines = append(outLines, line)
			i++
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:") {
			val := strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")
			t, err := time.Parse(time.RFC3339Nano, val)
			if err == nil {
				currentProgTime = t
				pendingProgTimeLine = line
			} else {
				pendingProgTimeLine = line
			}
			i++
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			infLine := line
			durVal := strings.TrimPrefix(infLine, "#EXTINF:")
			durVal = strings.Split(durVal, ",")[0]
			segmentDuration, _ := strconv.ParseFloat(durVal, 64)

			var segmentURL string
			j := i + 1
			for j < len(lines) {
				nextLine := strings.TrimSpace(lines[j])
				if nextLine != "" {
					segmentURL = nextLine
					break
				}
				j++
			}

			if segmentURL != "" {
				isAd := false

				if !currentProgTime.IsZero() {
					for _, r := range adRanges {
						// Add a 100ms grace threshold
						if (currentProgTime.After(r.start) || currentProgTime.Equal(r.start)) && currentProgTime.Before(r.end.Add(-100*time.Millisecond)) {
							isAd = true
							break
						}
					}
				}

				if strings.Contains(strings.ToLower(infLine), "amazon") {
					isAd = true
				}

				if isAd {
					if !currentProgTime.IsZero() {
						currentProgTime = currentProgTime.Add(time.Duration(segmentDuration * float64(time.Second)))
					}
					i = j + 1
					pendingProgTimeLine = ""
					continue
				}

				if pendingProgTimeLine != "" {
					outLines = append(outLines, pendingProgTimeLine)
					pendingProgTimeLine = ""
				} else if !currentProgTime.IsZero() {
					outLines = append(outLines, fmt.Sprintf("#EXT-X-PROGRAM-DATE-TIME:%s", currentProgTime.Format(time.RFC3339Nano)))
				}

				outLines = append(outLines, infLine)

				if !strings.HasPrefix(segmentURL, "http://") && !strings.HasPrefix(segmentURL, "https://") {
					parsedSource, errS := url.Parse(sourceURL)
					parsedSegment, errSeg := url.Parse(segmentURL)
					if errS == nil && errSeg == nil {
						segmentURL = parsedSource.ResolveReference(parsedSegment).String()
					}
				}

				outLines = append(outLines, segmentURL)

				if !currentProgTime.IsZero() {
					currentProgTime = currentProgTime.Add(time.Duration(segmentDuration * float64(time.Second)))
				}

				i = j + 1
				continue
			}
		}

		if line == "#EXT-X-DISCONTINUITY" {
			if len(outLines) > 0 && outLines[len(outLines)-1] == "#EXT-X-DISCONTINUITY" {
				i++
				continue
			}
		}

		if pendingProgTimeLine != "" {
			outLines = append(outLines, pendingProgTimeLine)
			pendingProgTimeLine = ""
		}
		outLines = append(outLines, line)
		i++
	}

	return strings.Join(outLines, "\n")
}
