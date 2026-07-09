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
	HTTPRequestsPerSec       *opsNetworkPromMetric `json:"httpRequestsPerSec,omitempty"`
	ChatConnections          *opsNetworkPromMetric `json:"chatConnections,omitempty"`
	StreamListeners          *opsNetworkPromMetric `json:"streamListeners,omitempty"`
	StreamListenersByChannel *opsNetworkPromMetric `json:"streamListenersByChannel,omitempty"`
	ChatMessagesOutPerSec    *opsNetworkPromMetric `json:"chatMessagesOutPerSec,omitempty"`
	UpstreamP95Sec           *opsNetworkPromMetric `json:"upstreamP95Sec,omitempty"`
}

type opsActiveStream struct {
	Channel            string `json:"channel"`
	Listeners          int64  `json:"listeners"`
	Quality            string `json:"quality,omitempty"`
	LiveEdge           int    `json:"liveEdge,omitempty"`
	WorkerBackend      string `json:"workerBackend,omitempty"`
	HLSProbeDurationMs int64  `json:"hlsProbeDurationMs,omitempty"`
	TargetDuration     string `json:"targetDuration,omitempty"`
	Bandwidth          int64  `json:"bandwidth,omitempty"`
}

type opsNetworkResponse struct {
	Services      setupDiagnosticsServices `json:"services"`
	ActiveStreams []opsActiveStream        `json:"activeStreams"`
	UpdatedAt     int64                    `json:"updatedAt"`
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
}

func (h *Handler) opsNetwork(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), setupProbeBudget)
	defer cancel()

	statuses := h.probeSetupServices(ctx, map[string]string{
		"chat":    "http://chat:8080/healthz",
		"video":   "http://video:8080/healthz",
		"emote":   "http://emote:8080/healthz",
		"scraper": scraperHealthURL(h.scraperAPIURL),
		"clipper": h.clipperServiceURL + "/v1/twitch/status",
	})

	services := setupDiagnosticsServices{
		Metadata: "ready",
		Chat:     statuses["chat"],
		Video:    statuses["video"],
		Emote:    statuses["emote"],
		Scraper:  statuses["scraper"],
		Clipper:  statuses["clipper"],
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.WithoutCancel(r.Context()), opsNetworkFetchBudget)
	defer fetchCancel()

	writeJSON(w, http.StatusOK, opsNetworkResponse{
		Services:      services,
		ActiveStreams: h.fetchActiveStreams(fetchCtx),
		UpdatedAt:     time.Now().Unix(),
	})
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
