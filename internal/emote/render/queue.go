package render

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/store"
	"streamclone/internal/metrics"
)

const sourceDownloadTimeout = 6 * time.Second

const maxSourceDownloadBytes = 5 << 20

// Queue enqueues demand-driven emote render jobs with backpressure and metrics.
type Queue struct {
	st   *store.Store
	obj  *objstore.Client
	cfg  Config
	log  *slog.Logger
	hc   *http.Client
	mu   sync.Mutex
	rate map[Reason]*rateBucket

	asyncSem      chan struct{}
	asyncInFlight map[string]struct{}
	asyncDropped  int64
}

const maxAsyncEnqueue = 16

type rateBucket struct {
	limit    int
	window   time.Duration
	count    int
	windowAt time.Time
}

type Request struct {
	EmoteID         string
	Provider        string
	ProviderEmoteID string
	ChannelLogin    string
	Reason          Reason
	Scale           string
	ChannelPriority string
	SourceURL       string
	SourceHash      string
	AlreadyRendered bool
}

func NewQueue(st *store.Store, obj *objstore.Client, cfg Config, log *slog.Logger) *Queue {
	q := &Queue{
		st:            st,
		obj:           obj,
		cfg:           cfg,
		log:           log,
		hc:            &http.Client{Timeout: 20 * time.Second},
		rate:          make(map[Reason]*rateBucket),
		asyncSem:      make(chan struct{}, maxAsyncEnqueue),
		asyncInFlight: make(map[string]struct{}),
	}
	if cfg.ChatObservedRateLimitPerMin > 0 {
		q.rate[ReasonChatObserved] = &rateBucket{limit: cfg.ChatObservedRateLimitPerMin, window: time.Minute}
	}
	if cfg.UIRequestRateLimitPerMin > 0 {
		q.rate[ReasonUIRequest] = &rateBucket{limit: cfg.UIRequestRateLimitPerMin, window: time.Minute}
	}
	return q
}

func (q *Queue) Config() Config {
	return q.cfg
}

func (q *Queue) ShouldEagerRender(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "twitch":
		return q.cfg.TwitchEager
	case "seventv", "7tv", "ffz", "frankerfacez", "bttv", "betterttv":
		return q.cfg.ThirdpartyEager
	case "custom", "":
		return true
	default:
		return q.cfg.ThirdpartyEager
	}
}

func (q *Queue) ShouldObserveInChat(provider string) bool {
	if !q.cfg.OnChatObserved {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "seventv", "7tv", "ffz", "frankerfacez", "bttv", "betterttv":
		return true
	default:
		return false
	}
}

func (q *Queue) ShouldRenderOnUIRequest() bool {
	return q.cfg.OnUIRequest
}

func (q *Queue) DefaultScales(requestedScale string) []string {
	scale := strings.TrimSpace(requestedScale)
	if scale != "" {
		return ResolveScales([]string{scale}, q.cfg.DefaultScales, q.cfg.AllowedScales)
	}
	return ResolveScales(nil, q.cfg.DefaultScales, q.cfg.AllowedScales)
}

func (q *Queue) Enqueue(ctx context.Context, req Request) (bool, error) {
	if q == nil || q.st == nil {
		return false, nil
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Reason = Reason(strings.TrimSpace(string(req.Reason)))
	if req.Reason == "" {
		req.Reason = ReasonEnsure
	}
	if req.EmoteID == "" {
		return false, nil
	}

	if !q.reasonEnabled(req.Reason) {
		q.logSkip(req, "reason_disabled")
		return false, nil
	}
	if !q.allowByBackpressure(req.Reason) {
		q.logSkip(req, "queue_backpressure")
		return false, nil
	}
	if !q.allowByRate(req.Reason) {
		q.logSkip(req, "rate_limited")
		return false, nil
	}

	emote, err := q.st.GetEmote(ctx, req.EmoteID)
	if err != nil {
		return false, err
	}
	if req.Provider == "" {
		req.Provider = strings.ToLower(strings.TrimSpace(emote.Provider))
	}
	if req.ProviderEmoteID == "" {
		req.ProviderEmoteID = emote.ProviderEmoteID
	}
	if req.SourceURL == "" {
		req.SourceURL = emote.SourceURL
	}
	if req.SourceHash == "" {
		req.SourceHash = emote.SourceHash
	}

	if q.obj != nil {
		scaleToCheck := strings.TrimSpace(req.Scale)
		if scaleToCheck == "" {
			scaleToCheck = "1x"
		}
		if _, _, err := q.obj.Get(ctx, req.EmoteID, scaleToCheck); err == nil {
			req.AlreadyRendered = true
			if req.Reason != ReasonRetry {
				q.logSkip(req, "already_rendered")
				return false, nil
			}
		}
	}

	scales := q.DefaultScales(req.Scale)
	sourceHash, err := q.ensureSource(ctx, emote, req)
	if err != nil {
		q.logSkip(req, "source_unavailable:"+err.Error())
		metrics.EmoteRenderFailed.WithLabelValues(req.Provider, firstScale(scales), string(req.Reason)).Inc()
		return false, err
	}

	sourceKey := JobSourceKey(sourceHash, scales)
	exists, err := q.st.JobExists(ctx, req.EmoteID, sourceKey)
	if err != nil {
		return false, err
	}
	if exists {
		q.logSkip(req, "duplicate_job")
		return false, nil
	}

	if _, err := q.st.InsertOrRequeueJob(ctx, req.EmoteID, sourceKey); err != nil {
		return false, err
	}

	channelPriority := req.ChannelPriority
	if channelPriority == "" {
		channelPriority = "normal"
	}
	metrics.EmoteRenderEnqueued.WithLabelValues(string(req.Reason), req.Provider, channelPriority).Inc()
	q.updateQueueDepthMetrics(ctx)

	if q.log != nil {
		q.log.Info("emote render enqueued",
			"emote_id", req.EmoteID,
			"provider", req.Provider,
			"provider_emote_id", req.ProviderEmoteID,
			"channel_login", req.ChannelLogin,
			"reason", req.Reason,
			"scale", req.Scale,
			"channel_priority", channelPriority,
			"source_url_present", req.SourceURL != "",
			"already_rendered", req.AlreadyRendered,
			"scales", strings.Join(scales, ","),
		)
	}
	return true, nil
}

func (q *Queue) EnqueueAsync(ctx context.Context, req Request) {
	key := strings.TrimSpace(req.EmoteID) + "|" + strings.TrimSpace(req.Scale)
	q.mu.Lock()
	if q.asyncSem == nil {
		q.asyncSem = make(chan struct{}, maxAsyncEnqueue)
	}
	if q.asyncInFlight == nil {
		q.asyncInFlight = make(map[string]struct{})
	}
	if _, dup := q.asyncInFlight[key]; dup {
		q.mu.Unlock()
		return
	}
	select {
	case q.asyncSem <- struct{}{}:
		q.asyncInFlight[key] = struct{}{}
		q.mu.Unlock()
	default:
		q.asyncDropped++
		q.mu.Unlock()
		if q.log != nil {
			q.log.Warn("async emote render enqueue dropped (semaphore full)",
				"emote_id", req.EmoteID,
				"provider", req.Provider,
				"reason", req.Reason,
			)
		}
		return
	}
	go func() {
		defer func() {
			<-q.asyncSem
			q.mu.Lock()
			delete(q.asyncInFlight, key)
			q.mu.Unlock()
		}()
		bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := q.Enqueue(bg, req); err != nil && q.log != nil {
			q.log.Warn("async emote render enqueue failed",
				"emote_id", req.EmoteID,
				"provider", req.Provider,
				"reason", req.Reason,
				"err", err,
			)
		}
	}()
}

func (q *Queue) HandleObservePayload(ctx context.Context, payload ObservePayload) error {
	if err := payload.validate(); err != nil {
		return err
	}
	if !q.ShouldObserveInChat(payload.Provider) {
		return nil
	}
	metrics.EmoteObservedInChat.WithLabelValues(strings.ToLower(payload.Provider)).Inc()
	_, err := q.Enqueue(ctx, Request{
		EmoteID:         payload.EmoteID,
		Provider:        payload.Provider,
		ProviderEmoteID: payload.ProviderEmoteID,
		ChannelLogin:    payload.ChannelLogin,
		Reason:          ReasonChatObserved,
		Scale:           payload.Scale,
	})
	return err
}

func (q *Queue) reasonEnabled(reason Reason) bool {
	switch reason {
	case ReasonManualBackfill, ReasonLegacyBackfill:
		return q.cfg.BackfillEnabled
	case ReasonChatObserved:
		return q.cfg.OnChatObserved
	case ReasonUIRequest:
		return q.cfg.OnUIRequest
	case ReasonCustomUpload, ReasonRetry:
		return true
	case ReasonEnsure:
		return true
	default:
		return true
	}
}

func (q *Queue) allowByBackpressure(reason Reason) bool {
	if reason.priority() >= 3 {
		return true
	}
	if q.st == nil || q.cfg.QueueMaxDepth <= 0 {
		return true
	}
	depth, err := q.st.CountPendingJobs(context.Background())
	if err != nil {
		return true
	}
	if depth < q.cfg.QueueMaxDepth {
		return true
	}
	return reason.priority() >= 2
}

func (q *Queue) allowByRate(reason Reason) bool {
	bucket, ok := q.rate[reason]
	if !ok || bucket == nil || bucket.limit <= 0 {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	if bucket.windowAt.IsZero() || now.Sub(bucket.windowAt) >= bucket.window {
		bucket.windowAt = now
		bucket.count = 0
	}
	if bucket.count >= bucket.limit {
		return false
	}
	bucket.count++
	return true
}

func (q *Queue) ensureSource(ctx context.Context, emote *store.Emote, req Request) (string, error) {
	if emote.SourceHash != "" {
		if q.obj != nil {
			if _, err := q.obj.GetSrc(ctx, emote.ID); err == nil {
				return emote.SourceHash, nil
			}
		}
	}
	sourceURL := strings.TrimSpace(req.SourceURL)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(emote.SourceURL)
	}
	if sourceURL == "" {
		return "", errNoSourceURL
	}
	if q.obj == nil {
		return "", errNoObjectStore
	}

	downloadCtx, cancel := context.WithTimeout(ctx, sourceDownloadTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := q.hc.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errDownloadStatus(resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxSourceDownloadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxSourceDownloadBytes {
		return "", errSourceTooLarge
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !isAllowedSourceMIME(mimeType) {
		return "", errBadSourceMIME
	}
	hash := hashBytes(data)
	if err := q.obj.PutSrc(ctx, emote.ID, data, mimeType); err != nil {
		return "", err
	}
	if emote.SourceHash != hash {
		_ = q.st.UpdateEmoteSourceHash(ctx, emote.ID, hash)
	}
	return hash, nil
}

func (q *Queue) logSkip(req Request, cause string) {
	if q.log == nil {
		return
	}
	q.log.Debug("emote render enqueue skipped",
		"emote_id", req.EmoteID,
		"provider", req.Provider,
		"provider_emote_id", req.ProviderEmoteID,
		"channel_login", req.ChannelLogin,
		"reason", req.Reason,
		"scale", req.Scale,
		"source_url_present", req.SourceURL != "",
		"already_rendered", req.AlreadyRendered,
		"cause", cause,
	)
}

func (q *Queue) updateQueueDepthMetrics(ctx context.Context) {
	SyncQueueDepthMetric(ctx, q.st)
}

// SyncQueueDepthMetric refreshes the global pending render queue gauge from Postgres.
func SyncQueueDepthMetric(ctx context.Context, st *store.Store) {
	if st == nil {
		return
	}
	depth, err := st.CountPendingJobs(ctx)
	if err != nil {
		return
	}
	metrics.EmoteRenderQueueDepth.Set(float64(depth))
}

func isAllowedSourceMIME(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch mimeType {
	case "image/webp", "image/gif", "image/png", "image/jpeg", "image/apng":
		return true
	default:
		return strings.HasPrefix(mimeType, "image/")
	}
}

func firstScale(scales []string) string {
	if len(scales) == 0 {
		return "1x"
	}
	return scales[0]
}

var (
	errNoSourceURL    = errString("provider source url unavailable")
	errNoObjectStore  = errString("object store unavailable")
	errSourceTooLarge = errString("provider source exceeds download cap")
	errBadSourceMIME  = errString("provider source content type unsupported")
)

type errString string

func (e errString) Error() string { return string(e) }

type errDownloadStatus int

func (e errDownloadStatus) Error() string {
	return "cdn returned " + strings.TrimSpace(strings.ReplaceAll(http.StatusText(int(e)), " ", "_"))
}
