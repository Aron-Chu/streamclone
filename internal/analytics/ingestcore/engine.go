package ingestcore

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"streamclone/internal/chat/enrich"
	"streamclone/internal/metrics"
)

type ircQueued struct {
	line     string
	tier     IngestTier
	login    string
	enqueued time.Time
}

// Engine is the ingest-core facade wired from cmd/analytics.
type Engine struct {
	cfg       Config
	parser    *Parser
	agg       *Aggregator
	flusher   *BatchFlusher
	manager   *CollectorManager
	scheduler *TierScheduler
	shadow    *ShadowComparer

	ircQueue   chan ircQueued
	p0InFlight int
	mu         sync.Mutex
	stopCh     chan struct{}
	runCtx     context.Context

	protectedMu sync.Mutex
	protected   map[string]DesiredChannel
}

// EngineDeps bundles dependencies for NewEngine.
type EngineDeps struct {
	Config   Config
	IRC      IRCConn
	Writer   BatchWriter
	Enricher *enrich.Enricher
	Source   CandidateSource
	Log      *slog.Logger
}

// NewEngine constructs the ingest-core engine.
func NewEngine(deps EngineDeps) *Engine {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	cfg := deps.Config
	e := &Engine{
		cfg:      cfg,
		parser:   NewParser(deps.Enricher),
		agg:      NewAggregator(cfg),
		manager:  NewCollectorManager(cfg, deps.IRC, log),
		ircQueue: make(chan ircQueued, cfg.IRCQueueSize),
		stopCh:   make(chan struct{}),
	}
	if deps.Writer != nil && cfg.WritesProduction() {
		e.flusher = NewBatchFlusher(cfg, deps.Writer, log)
	}
	if cfg.ShadowMode || cfg.DualReadMode {
		e.shadow = NewShadowComparer(cfg)
	}
	if deps.Source != nil {
		e.scheduler = NewTierScheduler(cfg, e.manager, deps.Source, log)
		e.scheduler.extraDesired = e.protectedDesiredLocked
	}
	return e
}

// Start launches workers when ingest-core is active.
func (e *Engine) Start(ctx context.Context, admissionInterval time.Duration) {
	if e == nil || !e.cfg.Active() {
		return
	}
	e.runCtx = ctx
	e.manager.SetRunContext(ctx)
	e.agg.Start()
	if e.flusher != nil && e.cfg.WritesProduction() {
		e.flusher.Start(ctx)
	}
	go e.ircLoop()
	go e.tickLoop(ctx)
	if e.scheduler != nil && e.cfg.CoreEnabled {
		e.scheduler.Start(ctx, admissionInterval)
	}
}

// Stop shuts down background loops.
func (e *Engine) Stop(ctx context.Context) {
	if e == nil {
		return
	}
	close(e.stopCh)
	if e.flusher != nil {
		snaps := e.agg.DrainAll()
		if e.cfg.WritesProduction() {
			e.flusher.Enqueue(snaps)
			e.flusher.Stop(ctx)
		}
	}
}

// HandleIRCLine processes one IRC line through ingest-core.
func (e *Engine) HandleIRCLine(line string, login string, tier IngestTier) {
	if e == nil || !e.cfg.Active() {
		return
	}
	channel := normalizeLogin(login)
	if channel == "" {
		if parsed, ok := e.parser.ChannelFromLine(line); ok {
			channel = parsed
		}
	}
	if !e.allowChannel(channel) {
		return
	}
	item := ircQueued{line: line, tier: tier, login: channel, enqueued: time.Now().UTC()}
	select {
	case e.ircQueue <- item:
		metrics.IngestIRCQueueDepth.Set(float64(len(e.ircQueue)))
	default:
		if tier != TierP0Always {
			metrics.IngestMessagesDroppedTotal.WithLabelValues(tier.Label()).Inc()
		} else {
			metrics.IngestMessagesDroppedTotal.WithLabelValues("P0").Inc()
		}
	}
}

func (e *Engine) allowChannel(login string) bool {
	if len(e.cfg.ShadowAllowlist) == 0 || e.cfg.CoreEnabled {
		return true
	}
	_, ok := e.cfg.ShadowAllowlist[normalizeLogin(login)]
	return ok
}

func (e *Engine) ircLoop() {
	for {
		select {
		case <-e.stopCh:
			return
		case item := <-e.ircQueue:
			metrics.IngestIRCQueueAgeSeconds.Observe(time.Since(item.enqueued).Seconds())
			metrics.IngestIRCQueueDepth.Set(float64(len(e.ircQueue)))
			e.processLine(item)
		}
	}
}

func (e *Engine) processLine(item ircQueued) {
	login := item.login
	if login == "" {
		if channel, ok := e.parser.ChannelFromLine(item.line); ok {
			login = channel
		}
	}
	streamID := e.agg.StreamIDForLogin(login)
	msg, ok := e.parser.ParseIRCLine(item.line, streamID, item.tier)
	if !ok {
		return
	}
	if login == "" {
		login = msg.Channel
	}
	metrics.AnalyticsIRCLinesProcessedTotal.Inc()
	e.mu.Lock()
	p0Used := e.p0InFlight
	e.mu.Unlock()
	if !e.agg.Enqueue(msg, func(int) {}, p0Used, e.cfg.P0QueueReserve) {
		return
	}
	if e.shadow != nil {
		// Shadow path records in-memory snapshot after enqueue processing async; tick loop flushes compare.
	}
}

func (e *Engine) tickLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.FlushInterval)
	openTicker := time.NewTicker(e.cfg.OpenMinuteFlush)
	defer ticker.Stop()
	defer openTicker.Stop()
	lastOpen := map[string]time.Time{}
	for {
		select {
		case <-e.stopCh:
			return
		case <-ctx.Done():
			return
		case <-openTicker.C:
			e.flush(false, true, lastOpen)
		case <-ticker.C:
			e.flush(true, true, lastOpen)
		}
	}
}

func (e *Engine) flush(completed, open bool, lastOpen map[string]time.Time) {
	now := time.Now().UTC()
	var snaps []RollupSnapshot
	if completed {
		snaps = append(snaps, e.agg.CollectFlushCandidates(open, e.cfg.OpenMinuteFlush, lastOpen, now)...)
	} else if open {
		snaps = e.agg.CollectFlushCandidates(true, e.cfg.OpenMinuteFlush, lastOpen, now)
	}
	if len(snaps) == 0 {
		return
	}
	if e.cfg.WritesProduction() && e.flusher != nil {
		e.flusher.Enqueue(snaps)
	}
	if e.shadow != nil {
		for _, s := range snaps {
			login := e.agg.LoginForStreamID(s.StreamID)
			e.shadow.RecordShadow(login, s)
		}
	}
}

// ManagerSnapshot exposes hub ingest block fields.
func (e *Engine) ManagerSnapshot() ManagerSnapshot {
	if e == nil || e.manager == nil {
		return ManagerSnapshot{}
	}
	return e.manager.Snapshot()
}

// Config returns engine config.
func (e *Engine) Config() Config {
	if e == nil {
		return Config{}
	}
	return e.cfg
}

// BindStream binds login to stream for aggregation.
func (e *Engine) BindStream(login, streamID string) {
	if e == nil || e.agg == nil {
		return
	}
	e.agg.BindStream(login, streamID)
}

// OwnsIRCAdmission reports whether ingest-core is the sole IRC admission owner.
func (e *Engine) OwnsIRCAdmission() bool {
	return e != nil && e.cfg.CoreEnabled
}

// RegisterProtectedGoLive admits a protected P0 channel via the collector manager.
func (e *Engine) RegisterProtectedGoLive(login, streamID string, trackPriority int) {
	if e == nil || !e.cfg.CoreEnabled || e.manager == nil {
		return
	}
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	tier := AssignTier(e.cfg, trackPriority, 0, e.cfg.TieringEnabled)
	if trackPriority >= 60 {
		tier = TierP0Always
	}
	e.protectedMu.Lock()
	if e.protected == nil {
		e.protected = make(map[string]DesiredChannel)
	}
	e.protected[login] = DesiredChannel{
		Login:         login,
		StreamID:      streamID,
		Tier:          tier,
		TrackPriority: trackPriority,
	}
	e.protectedMu.Unlock()
	if streamID != "" {
		e.BindStream(login, streamID)
	}
	if e.scheduler != nil && e.runCtx != nil {
		e.scheduler.RunOnce(e.runCtx)
	}
}

func (e *Engine) protectedDesiredLocked() []DesiredChannel {
	e.protectedMu.Lock()
	defer e.protectedMu.Unlock()
	if len(e.protected) == 0 {
		return nil
	}
	out := make([]DesiredChannel, 0, len(e.protected))
	for _, d := range e.protected {
		out = append(out, d)
	}
	return out
}

// RecordLegacySnapshot stores a legacy-path rollup for dual-read shadow compare.
func (e *Engine) RecordLegacySnapshot(channel string, snap RollupSnapshot) {
	if e == nil || e.shadow == nil {
		return
	}
	e.shadow.RecordLegacy(channel, snap)
}

// TouchAdmission forwards anti-churn touch to manager.
func (e *Engine) TouchAdmission(login string) {
	if e == nil || e.manager == nil {
		return
	}
	e.manager.TouchAdmissionObservation(login)
}

// SchedulerAdapter wraps analytics LiveAdmissionSource for ingest scheduler.
type SchedulerAdapter struct {
	ListFn func(ctx context.Context, topN int) ([]SchedulerCandidate, error)
}

func (a SchedulerAdapter) ListLiveCandidates(ctx context.Context, topN int) ([]SchedulerCandidate, error) {
	if a.ListFn == nil {
		return nil, nil
	}
	return a.ListFn(ctx, topN)
}
