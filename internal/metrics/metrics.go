package metrics

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	StreamsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streams_active", Help: "Currently active stream workers.",
	})
	StreamsReaped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "streams_reaped_total", Help: "Total reaped idle streams.",
	})
	StreamListeners = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stream_listeners", Help: "Current stream listeners by channel.",
	}, []string{"channel"})
	StreamRestarts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stream_restart_total", Help: "Total stream worker restarts by channel.",
	}, []string{"channel"})
	StreamStartDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stream_start_duration_seconds",
		Help:    "Time spent starting or attaching to a stream.",
		Buckets: prometheus.DefBuckets,
	}, []string{"result", "backend"})
	StreamStartFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stream_start_failures_total", Help: "Total stream start failures by error code.",
	}, []string{"code"})
	HLSProbeDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hls_probe_duration_seconds",
		Help:    "Time spent probing local HLS readiness.",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})
	HLSReadinessFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hls_readiness_failures_total", Help: "Total failed HLS readiness waits.",
	})
	ChatMessagesIn = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_in_total", Help: "Total chat messages ingested from upstream.",
	})
	ChatMessagesOut = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_out_total", Help: "Total chat messages delivered to clients.",
	})
	ChatConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "chat_connections", Help: "Current websocket chat clients.",
	})
	ChatChannelSubscribers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "chat_channel_subscribers", Help: "Current websocket subscribers by channel.",
	}, []string{"channel"})
	ChatQueueDrops = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_queue_drops_total", Help: "Total chat frames dropped from full client queues.",
	})
	ChatSendAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_send_attempts_total", Help: "Authenticated chat send attempts by result.",
	}, []string{"result"})
	TokenizeSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tokenize_seconds",
		Help:    "Time spent tokenizing chat messages.",
		Buckets: []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05},
	})
	CacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_requests_total", Help: "Cache lookups by result.",
	}, []string{"result"})
	UpstreamRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "upstream_requests_total", Help: "Upstream calls by operation and result.",
	}, []string{"op", "result"})
	UpstreamRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "upstream_request_duration_seconds",
		Help:    "Time spent on upstream calls by operation and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"op", "result"})
	AssetJobs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "asset_jobs_total", Help: "Asset processing jobs by terminal state.",
	}, []string{"state"})
	AssetProcessSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "asset_process_seconds",
		Help:    "Time spent processing emote asset jobs by result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})
	EmoteDictionaryQueueDrops = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emote_dictionary_queue_drops_total", Help: "Total dropped emote dictionary rebuild requests.",
	})
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total", Help: "HTTP requests by service, method, route, and status.",
	}, []string{"service", "method", "route", "status"})
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration by service, method, route, and status.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "route", "status"})
	ReadinessFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "readiness_failures_total", Help: "Readiness probe failures by service.",
	}, []string{"service"})
	TimeseriesWriteAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "timeseries_write_attempts_total", Help: "Best-effort time-series write attempts by backend and result.",
	}, []string{"backend", "result"})
	TimeseriesWriteBatchSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "timeseries_write_batch_size",
		Help:    "Number of rollups included in each time-series write attempt.",
		Buckets: []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000},
	}, []string{"backend"})
	TimeseriesWriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "timeseries_write_duration_seconds",
		Help:    "Time spent writing best-effort time-series batches by backend and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"backend", "result"})
	TimeseriesQueueDrops = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "timeseries_queue_drops_total", Help: "Rollup items dropped because the time-series writer queue was full.",
	}, []string{"backend"})
	AnalyticsRollupWriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "analytics_rollup_write_duration_seconds",
		Help:    "Time spent writing analytics minute rollups by write kind and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"kind", "result"})
	AnalyticsVODGQLPagesFetched = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_vod_gql_pages_fetched_total", Help: "Twitch GQL VOD comment pages fetched by result.",
	}, []string{"result"})
	AnalyticsVODGQLSegments = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_vod_gql_segments_total", Help: "Twitch GQL VOD comment segment transitions by state.",
	}, []string{"state"})
	AnalyticsHeatmapCacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_heatmap_cache_requests_total", Help: "Replay heatmap cache lookups by result.",
	}, []string{"result"})
	AnalyticsScraperRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_scraper_requests_total", Help: "Analytics scraper requests by source and result.",
	}, []string{"source", "result"})
	AnalyticsSyncBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_sync_bytes_total", Help: "Historical analytics sync response bytes by channel and operation.",
	}, []string{"channel", "op"})
	AnalyticsSyncActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "analytics_sync_active", Help: "Active historical analytics sync jobs by channel and phase.",
	}, []string{"channel", "phase"})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			started := time.Now()
			next.ServeHTTP(rec, r)
			route := "unknown"
			if rc := chi.RouteContext(r.Context()); rc != nil {
				if pattern := rc.RoutePattern(); pattern != "" {
					route = pattern
				}
			}
			status := strconv.Itoa(rec.status)
			HTTPRequests.WithLabelValues(service, r.Method, route, status).Inc()
			HTTPRequestDuration.WithLabelValues(service, r.Method, route, status).Observe(time.Since(started).Seconds())
		})
	}
}
