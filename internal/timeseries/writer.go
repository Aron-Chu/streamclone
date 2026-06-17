package timeseries

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

const (
	DefaultBackend       = "influxdb"
	DefaultBucket        = "streamclone"
	DefaultWriteTimeout  = time.Second
	DefaultQueueSize     = 1024
	maxStreamTitleTagLen = 80
)

type Config struct {
	Enabled      bool
	Backend      string
	URL          string
	Token        string
	Org          string
	Bucket       string
	WriteTimeout time.Duration
	QueueSize    int
}

type Rollup struct {
	ChannelLogin      string
	StreamID          string
	StreamTitle       string
	StreamCategory    string
	StreamStartedAt   time.Time
	MinuteTS          time.Time
	ViewerAvg         int
	ViewerMax         int
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
}

type Writer interface {
	EnqueueRollups([]Rollup)
	WriteRollups(context.Context, []Rollup) error
	Status() Status
	Close(context.Context) error
}

type BackfillReporter interface {
	StartBackfill(streams, rollups uint64)
	AddBackfillProgress(exported uint64)
	FinishBackfill(error)
}

type sink interface {
	WriteRollups(context.Context, []Rollup) error
}

type Status struct {
	Enabled     bool       `json:"enabled"`
	Configured  bool       `json:"configured"`
	Backend     string     `json:"backend"`
	Org         string     `json:"org,omitempty"`
	Bucket      string     `json:"bucket,omitempty"`
	State       string     `json:"state"`
	Attempts    uint64     `json:"attempts"`
	Failures    uint64     `json:"failures"`
	Drops       uint64     `json:"drops"`
	LastWriteAt *time.Time `json:"lastWriteAt,omitempty"`
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`

	BackfillState       string     `json:"backfillState,omitempty"`
	BackfillStreams     uint64     `json:"backfillStreams,omitempty"`
	BackfillRollups     uint64     `json:"backfillRollups,omitempty"`
	BackfillExported    uint64     `json:"backfillExported,omitempty"`
	BackfillStartedAt   *time.Time `json:"backfillStartedAt,omitempty"`
	BackfillCompletedAt *time.Time `json:"backfillCompletedAt,omitempty"`
	BackfillLastError   string     `json:"backfillLastError,omitempty"`
}

type NoopWriter struct {
	status Status
}

func (NoopWriter) EnqueueRollups([]Rollup)                      {}
func (NoopWriter) WriteRollups(context.Context, []Rollup) error { return nil }
func (w NoopWriter) Status() Status {
	status := w.status
	if status.Backend == "" {
		status.Backend = DefaultBackend
	}
	if status.State == "" {
		if status.Enabled {
			status.State = "misconfigured"
		} else {
			status.State = "disabled"
		}
	}
	return status
}
func (NoopWriter) Close(context.Context) error {
	return nil
}
func (NoopWriter) StartBackfill(uint64, uint64) {}
func (NoopWriter) AddBackfillProgress(uint64)   {}
func (NoopWriter) FinishBackfill(error)         {}

func NewAsyncWriter(cfg Config, logger *slog.Logger) Writer {
	backend := normalizeBackend(cfg.Backend)
	if !cfg.Enabled {
		return NoopWriter{status: Status{Enabled: false, Configured: false, Backend: backend, Org: cfg.Org, Bucket: cfg.Bucket, State: "disabled"}}
	}
	if backend != DefaultBackend {
		if logger != nil {
			logger.Warn("time-series writer disabled; unsupported backend", "backend", cfg.Backend)
		}
		return NoopWriter{status: Status{Enabled: true, Configured: false, Backend: backend, Org: cfg.Org, Bucket: cfg.Bucket, State: "misconfigured", LastError: "unsupported backend"}}
	}
	if strings.TrimSpace(cfg.URL) == "" || strings.TrimSpace(cfg.Org) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		if logger != nil {
			logger.Warn("time-series writer disabled; missing InfluxDB URL, org, or bucket")
		}
		return NoopWriter{status: Status{Enabled: true, Configured: false, Backend: backend, Org: cfg.Org, Bucket: cfg.Bucket, State: "misconfigured", LastError: "missing InfluxDB URL, org, or bucket"}}
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = DefaultWriteTimeout
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	w := &AsyncWriter{
		backend:      backend,
		queue:        make(chan []Rollup, cfg.QueueSize),
		writeTimeout: cfg.WriteTimeout,
		sink: NewInfluxSink(InfluxConfig{
			URL:    cfg.URL,
			Token:  cfg.Token,
			Org:    cfg.Org,
			Bucket: cfg.Bucket,
		}),
		logger: logger,
		done:   make(chan struct{}),
		status: Status{
			Enabled:    true,
			Configured: true,
			Backend:    backend,
			Org:        cfg.Org,
			Bucket:     cfg.Bucket,
			State:      "idle",
		},
	}
	w.wg.Add(1)
	go w.run()
	return w
}

type AsyncWriter struct {
	backend      string
	queue        chan []Rollup
	writeTimeout time.Duration
	sink         sink
	logger       *slog.Logger
	statusMu     sync.RWMutex
	status       Status
	wg           sync.WaitGroup
	closeOnce    sync.Once
	done         chan struct{}
}

func (w *AsyncWriter) EnqueueRollups(rollups []Rollup) {
	if w == nil || len(rollups) == 0 {
		return
	}
	batch := cloneRollups(rollups)
	if len(batch) == 0 {
		return
	}
	select {
	case w.queue <- batch:
	default:
		w.recordDrops(uint64(len(batch)))
		metrics.TimeseriesQueueDrops.WithLabelValues(w.backend).Add(float64(len(batch)))
		if w.logger != nil {
			w.logger.Warn("time-series rollup batch dropped; queue full", "backend", w.backend, "rollups", len(batch))
		}
	}
}

func (w *AsyncWriter) WriteRollups(ctx context.Context, rollups []Rollup) error {
	if w == nil || len(rollups) == 0 {
		return nil
	}
	batch := cloneRollups(rollups)
	if len(batch) == 0 {
		return nil
	}
	return w.writeBatch(ctx, batch)
}

func (w *AsyncWriter) Status() Status {
	if w == nil {
		return Status{Enabled: false, Configured: false, Backend: DefaultBackend, State: "disabled"}
	}
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	return w.status
}

func (w *AsyncWriter) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.closeOnce.Do(func() {
		close(w.queue)
		go func() {
			w.wg.Wait()
			close(w.done)
		}()
	})
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *AsyncWriter) run() {
	defer w.wg.Done()
	for batch := range w.queue {
		_ = w.writeBatch(context.Background(), batch)
	}
}

func (w *AsyncWriter) writeBatch(ctx context.Context, batch []Rollup) error {
	if len(batch) == 0 {
		return nil
	}
	started := time.Now()
	result := "success"
	if ctx == nil {
		ctx = context.Background()
	}
	writeCtx, cancel := context.WithTimeout(ctx, w.writeTimeout)
	defer cancel()
	metrics.TimeseriesWriteBatchSize.WithLabelValues(w.backend).Observe(float64(len(batch)))
	err := w.sink.WriteRollups(writeCtx, batch)
	if err != nil {
		result = "error"
		w.recordWriteError(err)
		if w.logger != nil {
			w.logger.Warn("time-series write failed", "backend", w.backend, "rollups", len(batch), "err", err)
		}
	} else {
		w.recordWriteSuccess()
	}
	metrics.TimeseriesWriteAttempts.WithLabelValues(w.backend, result).Inc()
	metrics.TimeseriesWriteDuration.WithLabelValues(w.backend, result).Observe(time.Since(started).Seconds())
	return err
}

func (w *AsyncWriter) recordDrops(count uint64) {
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.Drops += count
	if w.status.State == "idle" {
		w.status.State = "degraded"
	}
}

func (w *AsyncWriter) recordWriteSuccess() {
	now := time.Now().UTC()
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.Attempts++
	w.status.State = "ready"
	w.status.LastWriteAt = &now
	w.status.LastError = ""
}

func (w *AsyncWriter) recordWriteError(err error) {
	now := time.Now().UTC()
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.Attempts++
	w.status.Failures++
	w.status.State = "degraded"
	w.status.LastErrorAt = &now
	w.status.LastError = err.Error()
}

func (w *AsyncWriter) StartBackfill(streams, rollups uint64) {
	if w == nil {
		return
	}
	now := time.Now().UTC()
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.BackfillState = "running"
	w.status.BackfillStreams = streams
	w.status.BackfillRollups = rollups
	w.status.BackfillExported = 0
	w.status.BackfillStartedAt = &now
	w.status.BackfillCompletedAt = nil
	w.status.BackfillLastError = ""
}

func (w *AsyncWriter) AddBackfillProgress(exported uint64) {
	if w == nil || exported == 0 {
		return
	}
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.BackfillExported += exported
}

func (w *AsyncWriter) FinishBackfill(err error) {
	if w == nil {
		return
	}
	now := time.Now().UTC()
	w.statusMu.Lock()
	defer w.statusMu.Unlock()
	w.status.BackfillCompletedAt = &now
	if err != nil {
		w.status.BackfillState = "failed"
		w.status.BackfillLastError = err.Error()
		w.status.State = "degraded"
		w.status.LastErrorAt = &now
		w.status.LastError = err.Error()
		return
	}
	w.status.BackfillState = "completed"
	if w.status.State == "idle" {
		w.status.State = "ready"
	}
	w.status.BackfillLastError = ""
}

type InfluxConfig struct {
	URL    string
	Token  string
	Org    string
	Bucket string
	Client *http.Client
}

type InfluxSink struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewInfluxSink(cfg InfluxConfig) *InfluxSink {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	values := url.Values{}
	values.Set("org", cfg.Org)
	values.Set("bucket", cfg.Bucket)
	values.Set("precision", "s")
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &InfluxSink{
		endpoint: base + "/api/v2/write?" + values.Encode(),
		token:    cfg.Token,
		client:   client,
	}
}

func (s *InfluxSink) WriteRollups(ctx context.Context, rollups []Rollup) error {
	if s == nil || len(rollups) == 0 {
		return nil
	}
	body := BuildInfluxLineProtocol(rollups)
	if strings.TrimSpace(body) == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if s.token != "" {
		req.Header.Set("Authorization", "Token "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf bytes.Buffer
		_, _ = io.CopyN(&buf, resp.Body, 512)
		return fmt.Errorf("influxdb write failed: status=%d body=%q", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	return nil
}

func BuildInfluxLineProtocol(rollups []Rollup) string {
	var b strings.Builder
	for _, rollup := range rollups {
		if rollup.MinuteTS.IsZero() || strings.TrimSpace(rollup.StreamID) == "" || strings.TrimSpace(rollup.ChannelLogin) == "" {
			continue
		}
		ts := rollup.MinuteTS.UTC().Unix()
		writeStreamActivityLine(&b, rollup, ts)
		for key, count := range rollup.Emotes {
			if count <= 0 {
				continue
			}
			name, id, provider := splitEmoteKey(key)
			if provider == "" {
				provider = "unknown"
			}
			if id == "" {
				id = key
			}
			b.WriteString("emote_usage_1m")
			writeTag(&b, "channel_login", rollup.ChannelLogin)
			writeTag(&b, "stream_id", rollup.StreamID)
			writeStreamMetaTags(&b, rollup)
			writeTag(&b, "provider", provider)
			writeTag(&b, "emote_id", id)
			writeTag(&b, "emote_name", name)
			b.WriteByte(' ')
			b.WriteString("count=")
			b.WriteString(strconv.Itoa(count))
			b.WriteByte('i')
			b.WriteByte(' ')
			b.WriteString(strconv.FormatInt(ts, 10))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func writeStreamActivityLine(b *strings.Builder, rollup Rollup, ts int64) {
	b.WriteString("stream_activity_1m")
	writeTag(b, "channel_login", rollup.ChannelLogin)
	writeTag(b, "stream_id", rollup.StreamID)
	writeStreamMetaTags(b, rollup)
	b.WriteByte(' ')
	writeIntField(b, "viewer_avg", rollup.ViewerAvg)
	b.WriteByte(',')
	writeIntField(b, "viewer_max", rollup.ViewerMax)
	b.WriteByte(',')
	writeIntField(b, "chat_count", rollup.ChatCount)
	b.WriteByte(',')
	writeIntField(b, "total_emote_count", rollup.TotalEmoteCount)
	b.WriteByte(',')
	writeIntField(b, "seventv_emote_count", rollup.SevenTVEmoteCount)
	b.WriteByte(',')
	writeIntField(b, "unique_emote_count", countUniqueEmotes(rollup.Emotes))
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(ts, 10))
	b.WriteByte('\n')
}

func writeStreamMetaTags(b *strings.Builder, rollup Rollup) {
	if !rollup.StreamStartedAt.IsZero() {
		writeTag(b, "stream_started", strconv.FormatInt(rollup.StreamStartedAt.UTC().Unix(), 10))
	}
	title := sanitizeStreamTitle(rollup.StreamTitle)
	if title != "" {
		writeTag(b, "stream_title", title)
	}
	category := sanitizeStreamTitle(rollup.StreamCategory)
	if category != "" {
		writeTag(b, "stream_category", category)
	}
}

func countUniqueEmotes(emotes map[string]int) int {
	if len(emotes) == 0 {
		return 0
	}
	count := 0
	for _, uses := range emotes {
		if uses > 0 {
			count++
		}
	}
	return count
}

func sanitizeStreamTitle(title string) string {
	title = strings.TrimSpace(title)
	if len(title) > maxStreamTitleTagLen {
		title = title[:maxStreamTitleTagLen]
	}
	return title
}

func writeTag(b *strings.Builder, key, value string) {
	b.WriteByte(',')
	b.WriteString(escapeTag(key))
	b.WriteByte('=')
	b.WriteString(escapeTag(value))
}

func writeIntField(b *strings.Builder, key string, value int) {
	b.WriteString(escapeFieldKey(key))
	b.WriteByte('=')
	b.WriteString(strconv.Itoa(value))
	b.WriteByte('i')
}

func cloneRollups(rollups []Rollup) []Rollup {
	out := make([]Rollup, 0, len(rollups))
	for _, r := range rollups {
		if r.MinuteTS.IsZero() || strings.TrimSpace(r.StreamID) == "" || strings.TrimSpace(r.ChannelLogin) == "" {
			continue
		}
		cp := r
		if len(r.Emotes) > 0 {
			cp.Emotes = make(map[string]int, len(r.Emotes))
			for k, v := range r.Emotes {
				if v > 0 {
					cp.Emotes[k] = v
				}
			}
		} else {
			cp.Emotes = nil
		}
		out = append(out, cp)
	}
	return out
}

func normalizeBackend(backend string) string {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		return DefaultBackend
	}
	return backend
}

func splitEmoteKey(key string) (name, id, provider string) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) == 3 {
		return parts[2], parts[1], parts[0]
	}
	if len(parts) == 2 {
		return parts[1], parts[1], parts[0]
	}
	return key, "", ""
}

func escapeTag(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, " ", `\ `)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "=", `\=`)
	return value
}

func escapeFieldKey(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, " ", `\ `)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "=", `\=`)
	return value
}

func quoteStringField(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
