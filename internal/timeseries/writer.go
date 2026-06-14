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
	DefaultBackend      = "influxdb"
	DefaultBucket       = "streamclone"
	DefaultWriteTimeout = time.Second
	DefaultQueueSize    = 1024
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
	Close(context.Context) error
}

type sink interface {
	WriteRollups(context.Context, []Rollup) error
}

type NoopWriter struct{}

func (NoopWriter) EnqueueRollups([]Rollup) {}
func (NoopWriter) Close(context.Context) error {
	return nil
}

func NewAsyncWriter(cfg Config, logger *slog.Logger) Writer {
	backend := normalizeBackend(cfg.Backend)
	if !cfg.Enabled {
		return NoopWriter{}
	}
	if backend != DefaultBackend {
		if logger != nil {
			logger.Warn("time-series writer disabled; unsupported backend", "backend", cfg.Backend)
		}
		return NoopWriter{}
	}
	if strings.TrimSpace(cfg.URL) == "" || strings.TrimSpace(cfg.Org) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		if logger != nil {
			logger.Warn("time-series writer disabled; missing InfluxDB URL, org, or bucket")
		}
		return NoopWriter{}
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
		metrics.TimeseriesQueueDrops.WithLabelValues(w.backend).Add(float64(len(batch)))
		if w.logger != nil {
			w.logger.Warn("time-series rollup batch dropped; queue full", "backend", w.backend, "rollups", len(batch))
		}
	}
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
		w.write(batch)
	}
}

func (w *AsyncWriter) write(batch []Rollup) {
	started := time.Now()
	result := "success"
	ctx, cancel := context.WithTimeout(context.Background(), w.writeTimeout)
	defer cancel()
	metrics.TimeseriesWriteBatchSize.WithLabelValues(w.backend).Observe(float64(len(batch)))
	if err := w.sink.WriteRollups(ctx, batch); err != nil {
		result = "error"
		if w.logger != nil {
			w.logger.Warn("time-series write failed", "backend", w.backend, "rollups", len(batch), "err", err)
		}
	}
	metrics.TimeseriesWriteAttempts.WithLabelValues(w.backend, result).Inc()
	metrics.TimeseriesWriteDuration.WithLabelValues(w.backend, result).Observe(time.Since(started).Seconds())
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
			writeTag(&b, "provider", provider)
			writeTag(&b, "emote_id", id)
			b.WriteByte(' ')
			b.WriteString("emote_name=")
			b.WriteString(quoteStringField(name))
			b.WriteString(",count=")
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
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(ts, 10))
	b.WriteByte('\n')
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
