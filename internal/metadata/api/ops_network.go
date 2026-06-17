package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	videoStatusURL       = "http://video:8080/v1/stream/status"
	videoDiagnosticsBase = "http://video:8080/v1/stream/diagnostics?channel="
	prometheusQueryURL   = "http://prometheus:9090/api/v1/query"
	analyticsSyncActiveURL       = "http://analytics:8080/v1/analytics/sync/active"
	analyticsTrackingSnapshotURL = "http://analytics:8080/v1/analytics/tracking/snapshot"
)

type opsNetworkPromSeries struct {
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

type opsNetworkPromMetric struct {
	Query  string                 `json:"query"`
	Value  *float64               `json:"value,omitempty"`
	Series []opsNetworkPromSeries `json:"series,omitempty"`
}

type opsNetworkPrometheus struct {
	HTTPRequestsPerSec         *opsNetworkPromMetric `json:"httpRequestsPerSec,omitempty"`
	ChatConnections            *opsNetworkPromMetric `json:"chatConnections,omitempty"`
	StreamListeners            *opsNetworkPromMetric `json:"streamListeners,omitempty"`
	StreamListenersByChannel   *opsNetworkPromMetric `json:"streamListenersByChannel,omitempty"`
	ChatMessagesOutPerSec      *opsNetworkPromMetric `json:"chatMessagesOutPerSec,omitempty"`
	UpstreamP95Sec             *opsNetworkPromMetric `json:"upstreamP95Sec,omitempty"`
	AnalyticsBytesByChannelOp  *opsNetworkPromMetric `json:"analyticsBytesByChannelOp,omitempty"`
	AnalyticsSyncActive        *opsNetworkPromMetric `json:"analyticsSyncActive,omitempty"`
}

type opsActiveStream struct {
	Channel            string  `json:"channel"`
	Listeners          int64   `json:"listeners"`
	Quality            string  `json:"quality,omitempty"`
	LiveEdge           int     `json:"liveEdge,omitempty"`
	WorkerBackend      string  `json:"workerBackend,omitempty"`
	HLSProbeDurationMs int64   `json:"hlsProbeDurationMs,omitempty"`
	TargetDuration     string  `json:"targetDuration,omitempty"`
	Bandwidth          int64   `json:"bandwidth,omitempty"`
}

type opsActiveAnalyticsSyncJobNetwork struct {
	TrackerScrapeBytes int64   `json:"trackerScrapeBytes,omitempty"`
	GQLFetchBytes      int64   `json:"gqlFetchBytes,omitempty"`
	EmotePreloadBytes  int64   `json:"emotePreloadBytes,omitempty"`
	HelixBytes         int64   `json:"helixBytes,omitempty"`
	TotalBytes         int64   `json:"totalBytes,omitempty"`
	LastRateBps        float64 `json:"lastRateBps,omitempty"`
}

type opsActiveAnalyticsSyncJobChat struct {
	GQLPages      int `json:"gqlPages,omitempty"`
	SegmentsDone  int `json:"segmentsDone,omitempty"`
	SegmentsTotal int `json:"segmentsTotal,omitempty"`
}

type opsActiveAnalyticsSyncJobTracker struct {
	Active bool `json:"active,omitempty"`
}

type opsActiveAnalyticsSyncJob struct {
	StreamID string                          `json:"streamId"`
	Channel  string                          `json:"channel"`
	Phase    string                          `json:"phase,omitempty"`
	Network  *opsActiveAnalyticsSyncJobNetwork `json:"network,omitempty"`
	Chat     *opsActiveAnalyticsSyncJobChat  `json:"chat,omitempty"`
	Tracker  *opsActiveAnalyticsSyncJobTracker `json:"tracker,omitempty"`
}

type opsActiveAnalyticsSyncsResponse struct {
	Jobs      []opsActiveAnalyticsSyncJob `json:"jobs"`
	UpdatedAt int64                       `json:"updatedAt"`
}

type opsTrackingSnapshot struct {
	Tracked         []string `json:"tracked"`
	AlwaysTracked   []string `json:"alwaysTracked,omitempty"`
	Active          int      `json:"active,omitempty"`
	Max             int      `json:"max,omitempty"`
	TrackedChannels []string `json:"trackedChannels,omitempty"`
}

type opsNetworkResponse struct {
	Services             setupDiagnosticsServices `json:"services"`
	PulseReady           bool                     `json:"pulseReady"`
	Prometheus           *opsNetworkPrometheus    `json:"prometheus,omitempty"`
	ActiveStreams        []opsActiveStream        `json:"activeStreams"`
	ActiveAnalyticsSyncs *opsActiveAnalyticsSyncsResponse `json:"activeAnalyticsSyncs,omitempty"`
	TrackingSnapshot     *opsTrackingSnapshot     `json:"trackingSnapshot,omitempty"`
	UpdatedAt            int64                    `json:"updatedAt"`
}

type videoStatusSession struct {
	Channel           string `json:"channel"`
	Listeners         int64  `json:"listeners"`
	Quality           string `json:"quality"`
	WorkerBackend     string `json:"workerBackend"`
	SelectedRendition *struct {
		Bandwidth int64 `json:"bandwidth"`
	} `json:"selectedRendition"`
}

type videoStatusResponse struct {
	Sessions []videoStatusSession `json:"sessions"`
}

type videoDiagnosticsSnapshot struct {
	LiveEdge int `json:"liveEdge"`
	HLSProbe struct {
		DurationMs     int64  `json:"durationMs"`
		TargetDuration string `json:"targetDuration"`
	} `json:"hlsProbe"`
}

const (
	opsNetworkPromCacheTTL   = 2 * time.Second
	opsNetworkFetchBudget    = 2200 * time.Millisecond
	videoDiagnosticsCacheTTL = 5 * time.Second
)

type videoDiagnosticsCacheEntry struct {
	snapshot  videoDiagnosticsSnapshot
	expiresAt time.Time
}

var (
	videoDiagnosticsCacheMu sync.RWMutex
	videoDiagnosticsCache   = map[string]videoDiagnosticsCacheEntry{}
)

type opsNetworkPromCacheEntry struct {
	pulseReady bool
	snapshot   *opsNetworkPrometheus
	expiresAt  time.Time
}

var (
	opsNetworkPromCacheMu sync.Mutex
	opsNetworkPromCache   opsNetworkPromCacheEntry
)

var opsNetworkPromQueries = []struct {
	key   string
	query string
}{
	{
		key:   "httpRequestsPerSec",
		query: `sum(rate(http_requests_total[1m])) by (service)`,
	},
	{
		key:   "chatConnections",
		query: `sum(chat_connections)`,
	},
	{
		key:   "streamListeners",
		query: `sum(stream_listeners)`,
	},
	{
		key:   "chatMessagesOutPerSec",
		query: `sum(rate(chat_messages_out_total[1m]))`,
	},
	{
		key:   "upstreamP95Sec",
		query: `histogram_quantile(0.95, sum(rate(upstream_request_duration_seconds_bucket[5m])) by (le))`,
	},
	{
		key:   "streamListenersByChannel",
		query: `sum(stream_listeners) by (channel)`,
	},
	{
		key:   "analyticsBytesByChannelOp",
		query: `sum(rate(analytics_sync_bytes_total[1m])) by (channel, op)`,
	},
	{
		key:   "analyticsSyncActive",
		query: `sum(analytics_sync_active) by (channel, phase)`,
	},
}

func (h *Handler) opsNetwork(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), setupProbeBudget)
	defer cancel()

	var statuses map[string]string
	var pulseStatus string
	var probeWg sync.WaitGroup
	probeWg.Add(2)
	go func() {
		defer probeWg.Done()
		statuses = h.probeSetupServices(ctx, map[string]string{
			"chat":      "http://chat:8080/healthz",
			"video":     "http://video:8080/healthz",
			"emote":     "http://emote:8080/healthz",
			"analytics": "http://analytics:8080/healthz",
			"scraper":   scraperHealthURL(h.scraperAPIURL),
			"clipper":   h.clipperServiceURL + "/v1/twitch/status",
		})
	}()
	go func() {
		defer probeWg.Done()
		pulseStatus = h.pulseServiceReady(ctx)
	}()
	probeWg.Wait()

	services := setupDiagnosticsServices{
		Metadata:  "ready",
		Chat:      statuses["chat"],
		Video:     statuses["video"],
		Emote:     statuses["emote"],
		Analytics: statuses["analytics"],
		Scraper:   statuses["scraper"],
		Clipper:   statuses["clipper"],
		Pulse:     pulseStatus,
	}
	pulseReady := services.Pulse == "ready"

	fetchCtx, fetchCancel := context.WithTimeout(context.WithoutCancel(r.Context()), opsNetworkFetchBudget)
	defer fetchCancel()

	resp := opsNetworkResponse{
		Services:             services,
		PulseReady:           pulseReady,
		ActiveStreams:        h.fetchActiveStreams(fetchCtx),
		ActiveAnalyticsSyncs: h.fetchActiveAnalyticsSyncs(fetchCtx),
		TrackingSnapshot:     h.fetchTrackingSnapshot(fetchCtx),
		UpdatedAt:            time.Now().Unix(),
	}
	if pulseReady {
		resp.Prometheus = h.fetchPrometheusSnapshotCached(fetchCtx, pulseReady)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) fetchActiveStreams(ctx context.Context) []opsActiveStream {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoStatusURL, nil)
	if err != nil {
		return nil
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var payload videoStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	if len(payload.Sessions) == 0 {
		return nil
	}

	out := make([]opsActiveStream, len(payload.Sessions))
	var wg sync.WaitGroup
	for i, session := range payload.Sessions {
		out[i] = opsActiveStream{
			Channel:       session.Channel,
			Listeners:     session.Listeners,
			Quality:       session.Quality,
			WorkerBackend: session.WorkerBackend,
		}
		if session.SelectedRendition != nil {
			out[i].Bandwidth = session.SelectedRendition.Bandwidth
		}
		wg.Add(1)
		go func(index int, channel string) {
			defer wg.Done()
			diag := h.fetchVideoDiagnostics(ctx, channel)
			if diag == nil {
				return
			}
			out[index].LiveEdge = diag.LiveEdge
			out[index].HLSProbeDurationMs = diag.HLSProbe.DurationMs
			out[index].TargetDuration = diag.HLSProbe.TargetDuration
		}(i, session.Channel)
	}
	wg.Wait()
	return out
}

func (h *Handler) fetchVideoDiagnostics(ctx context.Context, channel string) *videoDiagnosticsSnapshot {
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		return nil
	}
	videoDiagnosticsCacheMu.RLock()
	if cached, ok := videoDiagnosticsCache[channel]; ok && time.Now().Before(cached.expiresAt) {
		snap := cached.snapshot
		videoDiagnosticsCacheMu.RUnlock()
		return &snap
	}
	videoDiagnosticsCacheMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoDiagnosticsBase+url.QueryEscape(channel), nil)
	if err != nil {
		return nil
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var snapshot videoDiagnosticsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil
	}
	videoDiagnosticsCacheMu.Lock()
	videoDiagnosticsCache[channel] = videoDiagnosticsCacheEntry{
		snapshot:  snapshot,
		expiresAt: time.Now().Add(videoDiagnosticsCacheTTL),
	}
	videoDiagnosticsCacheMu.Unlock()
	return &snapshot
}

func (h *Handler) fetchPrometheusSnapshotCached(ctx context.Context, pulseReady bool) *opsNetworkPrometheus {
	now := time.Now()
	opsNetworkPromCacheMu.Lock()
	if opsNetworkPromCache.snapshot != nil && now.Before(opsNetworkPromCache.expiresAt) && opsNetworkPromCache.pulseReady == pulseReady {
		snapshot := opsNetworkPromCache.snapshot
		opsNetworkPromCacheMu.Unlock()
		return snapshot
	}
	opsNetworkPromCacheMu.Unlock()

	snapshot := h.fetchPrometheusSnapshot(ctx)
	if snapshot == nil {
		return nil
	}
	opsNetworkPromCacheMu.Lock()
	opsNetworkPromCache = opsNetworkPromCacheEntry{
		pulseReady: pulseReady,
		snapshot:   snapshot,
		expiresAt:  now.Add(opsNetworkPromCacheTTL),
	}
	opsNetworkPromCacheMu.Unlock()
	return snapshot
}

func (h *Handler) fetchPrometheusSnapshot(ctx context.Context) *opsNetworkPrometheus {
	snapshot := &opsNetworkPrometheus{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, item := range opsNetworkPromQueries {
		wg.Add(1)
		go func(key, query string) {
			defer wg.Done()
			metric := h.queryPrometheusInstant(ctx, query)
			if metric == nil {
				return
			}
			mu.Lock()
			switch key {
			case "httpRequestsPerSec":
				snapshot.HTTPRequestsPerSec = metric
			case "chatConnections":
				snapshot.ChatConnections = metric
			case "streamListeners":
				snapshot.StreamListeners = metric
			case "chatMessagesOutPerSec":
				snapshot.ChatMessagesOutPerSec = metric
			case "upstreamP95Sec":
				snapshot.UpstreamP95Sec = metric
			case "streamListenersByChannel":
				snapshot.StreamListenersByChannel = metric
			case "analyticsBytesByChannelOp":
				snapshot.AnalyticsBytesByChannelOp = metric
			case "analyticsSyncActive":
				snapshot.AnalyticsSyncActive = metric
			}
			mu.Unlock()
		}(item.key, item.query)
	}
	wg.Wait()
	if snapshot.HTTPRequestsPerSec == nil &&
		snapshot.ChatConnections == nil &&
		snapshot.StreamListeners == nil &&
		snapshot.StreamListenersByChannel == nil &&
		snapshot.ChatMessagesOutPerSec == nil &&
		snapshot.UpstreamP95Sec == nil &&
		snapshot.AnalyticsBytesByChannelOp == nil &&
		snapshot.AnalyticsSyncActive == nil {
		return nil
	}
	return snapshot
}

func (h *Handler) fetchActiveAnalyticsSyncs(ctx context.Context) *opsActiveAnalyticsSyncsResponse {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, analyticsSyncActiveURL, nil)
	if err != nil {
		return nil
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var payload struct {
		Syncs []struct {
			StreamID string `json:"streamId"`
			Channel  string `json:"channel,omitempty"`
			Phase    string `json:"phase,omitempty"`
			Status   *struct {
				Channel string `json:"channel,omitempty"`
				Phase   string `json:"phase,omitempty"`
				Network *opsActiveAnalyticsSyncJobNetwork `json:"network,omitempty"`
				Chat    *struct {
					GQLPages      int `json:"gqlPages,omitempty"`
					SegmentsDone  int `json:"segmentsDone,omitempty"`
					SegmentsTotal int `json:"segmentsTotal,omitempty"`
				} `json:"chat,omitempty"`
				Tracker *opsActiveAnalyticsSyncJobTracker `json:"tracker,omitempty"`
			} `json:"status,omitempty"`
		} `json:"syncs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	jobs := make([]opsActiveAnalyticsSyncJob, 0, len(payload.Syncs))
	for _, sync := range payload.Syncs {
		job := opsActiveAnalyticsSyncJob{
			StreamID: sync.StreamID,
			Channel:  sync.Channel,
			Phase:    sync.Phase,
		}
		if sync.Status != nil {
			if job.Channel == "" {
				job.Channel = sync.Status.Channel
			}
			if job.Phase == "" {
				job.Phase = sync.Status.Phase
			}
			job.Network = sync.Status.Network
			if sync.Status.Chat != nil {
				job.Chat = &opsActiveAnalyticsSyncJobChat{
					GQLPages:      sync.Status.Chat.GQLPages,
					SegmentsDone:  sync.Status.Chat.SegmentsDone,
					SegmentsTotal: sync.Status.Chat.SegmentsTotal,
				}
			}
			if sync.Status.Tracker != nil {
				job.Tracker = &opsActiveAnalyticsSyncJobTracker{Active: sync.Status.Tracker.Active}
			}
		}
		if job.Channel == "" {
			job.Channel = "unknown"
		}
		jobs = append(jobs, job)
	}
	return &opsActiveAnalyticsSyncsResponse{
		Jobs:      jobs,
		UpdatedAt: time.Now().Unix(),
	}
}

func (h *Handler) fetchTrackingSnapshot(ctx context.Context) *opsTrackingSnapshot {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, analyticsTrackingSnapshotURL, nil)
	if err != nil {
		return nil
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var snapshot struct {
		TrackedChannels []string `json:"trackedChannels"`
		AlwaysTracked   []string `json:"alwaysTracked"`
		Active          int      `json:"active"`
		Max             int      `json:"max"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil
	}
	return &opsTrackingSnapshot{
		Tracked:         snapshot.TrackedChannels,
		TrackedChannels: snapshot.TrackedChannels,
		AlwaysTracked:   snapshot.AlwaysTracked,
		Active:          snapshot.Active,
		Max:             snapshot.Max,
	}
}

func (h *Handler) queryPrometheusInstant(ctx context.Context, query string) *opsNetworkPromMetric {
	for _, base := range prometheusQueryURLs() {
		if metric := h.queryPrometheusInstantAt(ctx, base, query); metric != nil {
			return metric
		}
	}
	return nil
}

func prometheusQueryURLs() []string {
	base := strings.TrimSuffix(prometheusQueryURL, "/api/v1/query")
	base = strings.TrimRight(base, "/")
	return []string{
		base,
		"http://host.docker.internal:9090",
	}
}

func (h *Handler) queryPrometheusInstantAt(ctx context.Context, baseURL, query string) *opsNetworkPromMetric {
	base := strings.TrimSuffix(baseURL, "/api/v1/query")
	base = strings.TrimRight(base, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/query?"+url.Values{"query": {query}}.Encode(), nil)
	if err != nil {
		return nil
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Status != "success" {
		return nil
	}

	metric := &opsNetworkPromMetric{Query: query}
	series := make([]opsNetworkPromSeries, 0, len(payload.Data.Result))
	var scalarSum float64
	var scalarCount int
	for _, row := range payload.Data.Result {
		if len(row.Value) < 2 {
			continue
		}
		value, ok := promSampleValue(row.Value[1])
		if !ok {
			continue
		}
		labels := map[string]string{}
		for key, label := range row.Metric {
			if key != "__name__" {
				labels[key] = label
			}
		}
		if len(labels) > 0 {
			series = append(series, opsNetworkPromSeries{Labels: labels, Value: value})
		} else {
			scalarSum += value
			scalarCount++
		}
	}
	if len(series) > 0 {
		metric.Series = series
		return metric
	}
	if scalarCount > 0 {
		sum := scalarSum
		metric.Value = &sum
	}
	return metric
}

func promSampleValue(raw any) (float64, bool) {
	switch value := raw.(type) {
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	case float64:
		return value, true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
