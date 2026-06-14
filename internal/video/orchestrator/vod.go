package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/metrics"
	"streamclone/internal/resilience"
	"streamclone/internal/upstream"
	"streamclone/internal/video/registry"
	"streamclone/internal/video/token"
	"streamclone/internal/video/usher"
	"streamclone/internal/video/worker"
)

var errVodHelixMissing = errors.New("helix_missing")

type VodSpawnFunc func(vodID, quality string, offsetSeconds int, rtmp string, logw io.Writer, twitchOAuth string) (registry.Streamer, error)
type VodDirectSpawnFunc func(vodID, sourceURL string, offsetSeconds int, rtmp string, logw io.Writer) (registry.Streamer, error)

type vodStartReq struct {
	VodID         string `json:"vod_id"`
	OffsetSeconds int    `json:"offset_seconds"`
	Quality       string `json:"quality"`
	LatencyMode   string `json:"latency_mode"`
}

type vodStartResp struct {
	VodID             string                    `json:"vod_id"`
	HLSURL            string                    `json:"hlsUrl"`
	SessionID         string                    `json:"session_id"`
	Quality           string                    `json:"quality"`
	OffsetSeconds     int                       `json:"offset_seconds"`
	SeekSeconds       int                       `json:"seek_seconds"`
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
}

func toVodResp(s *registry.Session, sessionID string) vodStartResp {
	return vodStartResp{
		VodID:             s.VodID,
		HLSURL:            s.HLSURL,
		SessionID:         sessionID,
		Quality:           s.Quality,
		OffsetSeconds:     s.OffsetSeconds,
		SeekSeconds:       s.SeekSeconds,
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
	}
}

func (h *Orchestrator) startVod(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req vodStartReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		metrics.StreamStartFailures.WithLabelValues("bad_request").Inc()
		metrics.StreamStartDuration.WithLabelValues("bad_request", "none").Observe(time.Since(started).Seconds())
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.VodID = strings.TrimSpace(req.VodID)
	if !worker.ValidVodID(req.VodID) {
		metrics.StreamStartFailures.WithLabelValues("invalid_vod_id").Inc()
		metrics.StreamStartDuration.WithLabelValues("invalid_vod_id", "none").Observe(time.Since(started).Seconds())
		writeAPIError(w, http.StatusBadRequest, "invalid_vod_id", "invalid vod id", false)
		return
	}
	if req.OffsetSeconds < 0 {
		req.OffsetSeconds = 0
	}
	req.Quality = effectiveQuality(req.Quality, h.o.DefaultQuality)
	regKey := worker.VodRegistryKey(req.VodID)
	qualityRestarted := false
	if s, ok := h.o.Registry.Get(regKey); ok {
		if s.Quality != req.Quality || s.OffsetSeconds != req.OffsetSeconds {
			qualityRestarted = true
			h.stopExistingForQualityChange(s)
		} else {
			sessionID := newSessionID()
			s.AddListener(sessionID)
			metrics.StreamListeners.WithLabelValues(regKey).Set(float64(s.Listeners()))
			metrics.StreamStartDuration.WithLabelValues("ok", s.WorkerBackend).Observe(time.Since(started).Seconds())
			writeJSON(w, http.StatusOK, toVodResp(s, sessionID))
			return
		}
	}
	sfKey := regKey + ":" + req.Quality + ":" + strconv.Itoa(req.OffsetSeconds)
	v, err, _ := h.sf.Do(sfKey, func() (any, error) {
		viewerOAuth := ""
		if h.o.ViewerAuth != nil {
			if tok, ok := h.o.ViewerAuth.ViewerAccessToken(r.Context(), r); ok {
				viewerOAuth = tok
			}
		}
		return h.createVod(r.Context(), req.VodID, req.Quality, req.OffsetSeconds, qualityRestarted, viewerOAuth)
	})
	if err != nil {
		code := h.vodStartErrorCode(err)
		metrics.StreamStartFailures.WithLabelValues(code).Inc()
		metrics.StreamStartDuration.WithLabelValues(code, "none").Observe(time.Since(started).Seconds())
		h.writeVodStartError(w, req.VodID, err, h.vodFailReason(err))
		return
	}
	s := v.(*registry.Session)
	sessionID := newSessionID()
	s.AddListener(sessionID)
	metrics.StreamListeners.WithLabelValues(regKey).Set(float64(s.Listeners()))
	metrics.StreamStartDuration.WithLabelValues("ok", s.WorkerBackend).Observe(time.Since(started).Seconds())
	writeJSON(w, http.StatusOK, toVodResp(s, sessionID))
}

func (h *Orchestrator) vodStartErrorCode(err error) string {
	switch {
	case errors.Is(err, resilience.ErrOpen):
		return "upstream_token_failed"
	case errors.Is(err, usher.ErrVodUnavailable):
		return "vod_unavailable"
	case errors.Is(err, errBusy):
		return "capacity_reached"
	case errors.Is(err, worker.ErrInvalidVodID):
		return "invalid_vod_id"
	case errors.Is(err, upstream.ErrPlaybackToken):
		return "upstream_token_failed"
	case errors.Is(err, errHLSNotReady):
		return "hls_not_ready"
	default:
		return "vod_start_failed"
	}
}

func (h *Orchestrator) vodFailReason(err error) string {
	if errors.Is(err, errVodHelixMissing) {
		return "helix_missing"
	}
	if errors.Is(err, usher.ErrVodUnavailable) {
		return "usher_404"
	}
	return ""
}

func (h *Orchestrator) writeVodStartError(w http.ResponseWriter, vodID string, err error, reason string) {
	switch {
	case errors.Is(err, usher.ErrVodUnavailable):
		writeAPIErrorWithReason(w, http.StatusNotFound, "vod_unavailable", "vod unavailable", false, reason)
	case errors.Is(err, errBusy):
		writeAPIError(w, http.StatusServiceUnavailable, "capacity_reached", "stream capacity reached", true)
	case errors.Is(err, worker.ErrInvalidVodID):
		writeAPIError(w, http.StatusBadRequest, "invalid_vod_id", "invalid vod id", false)
	case errors.Is(err, upstream.ErrPlaybackToken):
		h.o.Log.Error("vod playback token failed", "vod_id", vodID, "err", err)
		writeAPIError(w, http.StatusBadGateway, "upstream_token_failed", err.Error(), true)
	case errors.Is(err, resilience.ErrOpen):
		h.o.Log.Warn("vod token circuit breaker open", "vod_id", vodID)
		writeAPIError(w, http.StatusBadGateway, "upstream_token_failed", "relay token service cooling down; try again shortly", true)
	case errors.Is(err, errHLSNotReady):
		h.o.Log.Error("vod hls readiness failed", "vod_id", vodID, "err", err)
		writeAPIError(w, http.StatusGatewayTimeout, "hls_not_ready", "local HLS relay did not become ready", true)
	default:
		h.o.Log.Error("vod start failed", "vod_id", vodID, "err", err)
		writeAPIError(w, http.StatusBadGateway, "vod_start_failed", "vod start failed: "+err.Error(), true)
	}
}

// fetchVodPlaybackToken tries a viewer OAuth token first (chat sign-in). Chat scopes
// do not include VOD playback, so GQL often returns 401 — fall back to anonymous embed
// token without counting that failure against the shared upstream circuit breaker.
// streamlinkOAuth is non-empty only when the viewer token worked for VOD playback;
// do not pass chat-only OAuth to streamlink (it can hang or never publish HLS).
func (h *Orchestrator) fetchVodPlaybackToken(ctx context.Context, vodID, viewerOAuth string) (token.Token, string, error) {
	if strings.TrimSpace(viewerOAuth) != "" {
		tok, err := h.o.Token.Vod(ctx, vodID, viewerOAuth)
		if err == nil {
			return tok, viewerOAuth, nil
		}
		h.o.Log.Warn("viewer oauth vod token failed; falling back to anonymous", "vod_id", vodID, "err", err)
	}

	var tok token.Token
	err := h.breaker.Do(func() error {
		return resilience.Retry(ctx, 2, 200*time.Millisecond, func() error {
			var e error
			tok, e = h.o.Token.Vod(ctx, vodID, "")
			return e
		})
	})
	return tok, "", err
}

func (h *Orchestrator) createVod(ctx context.Context, vodID, quality string, offsetSeconds int, qualityRestarted bool, viewerOAuth string) (*registry.Session, error) {
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

	mediaKey := worker.VodMediaKey(vodID)
	regKey := worker.VodRegistryKey(vodID)
	seekSeconds := worker.VodSeekSeconds(offsetSeconds)

	if h.o.VodHelix != nil {
		exists, herr := h.o.VodHelix.VideoExists(ctx, vodID)
		if herr != nil {
			h.o.Log.Warn("vod helix preflight failed; continuing", "vod_id", vodID, "err", herr)
		} else if !exists {
			h.o.Log.Warn("vod unavailable", "vod_id", vodID, "reason", "helix_missing")
			return nil, fmt.Errorf("%w", errors.Join(usher.ErrVodUnavailable, errVodHelixMissing))
		}
	}

	upstreamStartedAt := time.Now()
	var tok token.Token
	tokenStartedAt := time.Now()
	tok, streamlinkOAuth, terr := h.fetchVodPlaybackToken(ctx, vodID, viewerOAuth)
	if terr != nil {
		metrics.UpstreamRequests.WithLabelValues("vod_token", "error").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues("vod_token", "error").Observe(time.Since(tokenStartedAt).Seconds())
		return nil, terr
	}
	metrics.UpstreamRequests.WithLabelValues("vod_token", "ok").Inc()
	metrics.UpstreamRequestDuration.WithLabelValues("vod_token", "ok").Observe(time.Since(tokenStartedAt).Seconds())

	usherStartedAt := time.Now()
	rends, usherErr := h.o.Usher.DiscoverVod(ctx, vodID, tok.Value, tok.Signature)
	var selected *usher.Rendition
	if usherErr != nil {
		metrics.UpstreamRequests.WithLabelValues("vod_usher", "error").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues("vod_usher", "error").Observe(time.Since(usherStartedAt).Seconds())
		if errors.Is(usherErr, usher.ErrVodUnavailable) {
			h.o.Log.Warn("vod usher unavailable; continuing with streamlink relay", "vod_id", vodID, "has_viewer_oauth", viewerOAuth != "")
		} else {
			return nil, usherErr
		}
	} else {
		metrics.UpstreamRequests.WithLabelValues("vod_usher", "ok").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues("vod_usher", "ok").Observe(time.Since(usherStartedAt).Seconds())
		selected = selectRendition(rends, quality)
	}

	upstreamFetchMs := time.Since(upstreamStartedAt).Milliseconds()
	rtmp := "rtmp://" + h.o.RTMPBase + "/live/" + mediaKey
	st, backend, fallbackAttempted, fallbackAttempts, lastStartErr, startupBreakdown, err := h.startVodWorker(ctx, vodID, quality, offsetSeconds, rtmp, selected, streamlinkOAuth)
	if err != nil {
		return nil, err
	}
	startupMs := time.Since(startupStartedAt).Milliseconds()
	startupBreakdown.UpstreamFetchMs = upstreamFetchMs
	startupBreakdown.TotalMs = startupMs
	now := time.Now()
	s := &registry.Session{
		Channel:           regKey,
		VodID:             vodID,
		OffsetSeconds:     offsetSeconds,
		SeekSeconds:       seekSeconds,
		Quality:           quality,
		HLSURL:            h.o.HLSBase + "/live/" + mediaKey + "/index.m3u8",
		Renditions:        rends,
		SelectedRendition: selected,
		WorkerBackend:     backend,
		StartupMs:         startupMs,
		StartupBreakdown:  startupBreakdown,
		FallbackAttempted: fallbackAttempted,
		FallbackAttempts:  fallbackAttempts,
		LastStartError:    lastStartErr,
		QualityRestarted:  qualityRestarted,
		StartedAt:         now,
	}
	s.MarkWorkerStart(now)
	s.SetStream(st)
	h.o.Registry.Add(s)
	committed = true
	metrics.StreamsActive.Inc()
	go h.superviseVod(s, vodID, mediaKey)
	return s, nil
}

// VOD streamlink→ffmpeg→RTMP→MediaMTX can exceed live probe budgets on cold start.
const vodHLSProbeMinTimeout = 45 * time.Second

func (h *Orchestrator) startVodWorker(ctx context.Context, vodID, quality string, offsetSeconds int, rtmp string, selected *usher.Rendition, viewerOAuth string) (registry.Streamer, string, bool, int, string, registry.StartupBreakdown, error) {
	var lastErr error
	fallbackAttempts := 0
	backends := vodWorkerBackends(h.o.WorkerBackends)
	for i, backend := range backends {
		if i > 0 {
			fallbackAttempts++
		}
		spawnStartedAt := time.Now()
		st, err := h.spawnVodBackend(vodID, quality, offsetSeconds, rtmp, backend, selected, viewerOAuth)
		spawnMs := time.Since(spawnStartedAt).Milliseconds()
		if err != nil {
			if !errors.Is(err, errDirectHLSSourceUnavailable) || lastErr == nil {
				lastErr = err
			}
			continue
		}
		probeTimeout := h.o.HLSProbeTimeout
		if probeTimeout < vodHLSProbeMinTimeout {
			probeTimeout = vodHLSProbeMinTimeout
		}
		hlsReadyStartedAt := time.Now()
		mediaKey := worker.VodMediaKey(vodID)
		if err := waitForHLS(ctx, h.o.HLSProbeBase, mediaKey, probeTimeout, hlsStabilityWindow, true); err != nil {
			st.Kill()
			_ = st.Wait()
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

func vodWorkerBackends(backends []string) []string {
	normalized := normalizeBackends(backends)
	if len(normalized) == 0 {
		return normalized
	}
	out := make([]string, 0, len(normalized))
	for _, backend := range normalized {
		if backend == "streamlink" {
			out = append(out, backend)
		}
	}
	for _, backend := range normalized {
		if backend != "streamlink" {
			out = append(out, backend)
		}
	}
	return out
}

func (h *Orchestrator) spawnVodBackend(vodID, quality string, offsetSeconds int, rtmp, backend string, selected *usher.Rendition, viewerOAuth string) (registry.Streamer, error) {
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
		h.o.Log.Info("spawning vod direct HLS worker with local manifest proxy", "vod_id", vodID, "proxy_url", proxyURL)
		return h.o.VodDirectSpawn(vodID, proxyURL, offsetSeconds, rtmp, vodLogWriter{log: h.o.Log, vodID: vodID})
	default:
		return h.o.VodSpawn(vodID, quality, offsetSeconds, rtmp, vodLogWriter{log: h.o.Log, vodID: vodID}, viewerOAuth)
	}
}

func (h *Orchestrator) superviseVod(s *registry.Session, vodID, mediaKey string) {
	err := s.Stream().Wait()
	s.RecordWorkerError(err)
	s.MarkStopped()
	h.o.Registry.Remove(s.Channel)
	if st := s.Stream(); st != nil {
		st.Kill()
	}
	h.release(s)
	h.o.Log.Info("vod relay finished", "vod_id", vodID, "media_key", mediaKey, "err", err)
}

type vodLogWriter struct {
	log   *slog.Logger
	vodID string
}

func (l vodLogWriter) Write(p []byte) (int, error) {
	l.log.Debug("vod subprocess", "vod_id", l.vodID, "out", strings.TrimSpace(string(p)))
	return len(p), nil
}
