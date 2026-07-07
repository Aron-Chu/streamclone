package analytics

import (
	"context"
	"log/slog"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

// LiveAdmissionPoller admits live Top-N channels into the IRC collector at
// TrackPriorityTopRoster when explicitly enabled. Candidates come from a
// LiveAdmissionSource (Helix top-live by default, roster metadata as legacy).
type LiveAdmissionPoller struct {
	source    LiveAdmissionSource
	collector *Collector
	cfg       config.Config
	log       *slog.Logger
	interval  time.Duration
}

func NewLiveAdmissionPoller(source LiveAdmissionSource, collector *Collector, cfg config.Config, log *slog.Logger) *LiveAdmissionPoller {
	if log == nil {
		log = slog.Default()
	}
	interval := cfg.PulseLiveAdmissionInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &LiveAdmissionPoller{
		source:    source,
		collector: collector,
		cfg:       cfg,
		log:       log,
		interval:  interval,
	}
}

func (p *LiveAdmissionPoller) Enabled() bool {
	return p != nil && p.cfg.PulseLiveAdmissionEnabled
}

func StartLiveAdmissionPoller(ctx context.Context, poller *LiveAdmissionPoller, log *slog.Logger) {
	if poller == nil || !poller.Enabled() {
		metrics.TopRosterAdmissionEnabled.Set(0)
		recordTopRosterAdmissionSkip(TopRosterAdmissionModeDefault, TopRosterAdmissionSkipDisabled)
		if log != nil {
			log.Info("live admission poller disabled", "reason", TopRosterAdmissionSkipDisabled)
		}
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Info("live admission poller started",
		"interval", poller.interval.String(),
		"top_n", poller.cfg.PulseLiveAdmissionTopN,
		"source", normalizeLiveAdmissionSource(poller.cfg.PulseLiveAdmissionSource),
	)
	go func() {
		ticker := time.NewTicker(poller.interval)
		defer ticker.Stop()
		poller.runOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poller.runOnce(ctx)
			}
		}
	}()
}

func (p *LiveAdmissionPoller) runOnce(ctx context.Context) {
	if p == nil || p.source == nil || p.collector == nil {
		return
	}
	mode := TopRosterAdmissionModeDefault
	metrics.TopRosterAdmissionEnabled.Set(boolFloat(p.Enabled()))
	topN := p.cfg.PulseLiveAdmissionTopN
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	live, err := p.source.ListLiveCandidates(ctx, topN)
	if err != nil {
		p.log.Warn("live admission list failed", "err", err)
		return
	}
	metrics.TopRosterAdmissionLiveConsidered.Set(float64(len(live)))
	snap := p.collector.TrackingSnapshot()
	state := newAdmissionCycleState(snap)
	metrics.TopRosterAdmissionActiveCollectors.Set(float64(state.active))
	skippedByReason := map[string]int{}
	if state.max <= 0 {
		recordTopRosterAdmissionSkip(TopRosterAdmissionModeDefault, TopRosterAdmissionSkipEnvMismatch)
		skippedByReason[TopRosterAdmissionSkipEnvMismatch]++
	}
	if len(live) == 0 {
		metrics.TopRosterAdmissionZeroChatLiveRows.Set(0)
		p.log.Info("live admission cycle", "considered", 0, "admitted", 0, "skipped_by_reason", skippedByReason)
		return
	}
	now := time.Now().UTC()
	capacityBlocked := false
	admitted := 0
	for _, row := range live {
		outcome, message, streamID := classifyTopRosterCandidate(p.collector, state.trackedLogins, row)
		if outcome == TopRosterAdmissionEmptyLogin || outcome == TopRosterAdmissionEmptyStreamID || outcome == TopRosterAdmissionNotLive {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, message, skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, message, state))
			continue
		}
		if outcome == TopRosterAdmissionDuplicateStream || outcome == TopRosterAdmissionAlreadyTracking {
			p.collector.TouchAdmissionObservation(normalizeLogin(row.Login))
		}
		if outcome == TopRosterAdmissionDuplicateStream {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, "duplicate stream id already tracked", skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, "duplicate stream id already tracked", state))
			continue
		}
		if outcome == TopRosterAdmissionAlreadyTracking {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, "already tracking", skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, "already tracking", state))
			continue
		}
		resp := p.collector.WatchWithPriority(ctx, normalizeLogin(row.Login), "", TrackPriorityTopRoster)
		p.collector.NoteGoLiveDetected(streamID, normalizeLogin(row.Login), "top_roster", TrackPriorityTopRoster, false, liveAdmissionStreamStartedAt(row))
		outcome = classifyTopRosterWatchResponse(resp)
		message = resp.Message
		if resp.Tracking {
			state.noteAdmission(normalizeLogin(row.Login), resp.Active, resp.Max)
		}
		recordTopRosterAdmissionMetrics(mode, outcome)
		recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, message, state))
		if resp.Tracking {
			admitted++
			p.log.Info("live admission admitted", "login", normalizeLogin(row.Login), "stream_id", streamID, "active", resp.Active, "max", resp.Max)
			continue
		}
		recordTopRosterAdmissionSkipForOutcome(mode, outcome, message, skippedByReason)
		p.log.Debug("live admission skipped", "login", normalizeLogin(row.Login), "message", resp.Message, "active", resp.Active, "max", resp.Max)
		if outcome == TopRosterAdmissionCapacityFull {
			capacityBlocked = true
			break
		}
	}
	if capacityBlocked {
		metrics.TopRosterAdmissionCapacityBlockedTotal.Inc()
	}
	desired := make(map[string]struct{}, len(live))
	for _, row := range live {
		login := normalizeLogin(row.Login)
		if login != "" && row.IsLive {
			desired[login] = struct{}{}
		}
	}
	grace := p.cfg.PulseLiveAdmissionMissGraceCycles
	if grace <= 0 {
		grace = 3
	}
	if evicted := p.collector.ReconcileTopRosterAdmissionMisses(desired, grace); evicted > 0 {
		p.log.Info("live admission evicted stale roster channels", "evicted", evicted, "grace_cycles", grace)
	}
	p.log.Info("live admission cycle", "considered", len(live), "admitted", admitted, "skipped_by_reason", skippedByReason)
}

func liveAdmissionStreamStartedAt(row Top500Current) time.Time {
	if row.StartedAt == nil || row.StartedAt.IsZero() {
		return time.Time{}
	}
	return row.StartedAt.UTC()
}

// Deprecated: use LiveAdmissionPoller.
type Top500PriorityWatchPoller = LiveAdmissionPoller

// Deprecated: use NewLiveAdmissionPoller.
func NewTop500PriorityWatchPoller(source LiveAdmissionSource, collector *Collector, cfg config.Config, log *slog.Logger) *LiveAdmissionPoller {
	return NewLiveAdmissionPoller(source, collector, cfg, log)
}

// Deprecated: use StartLiveAdmissionPoller.
func StartTop500PriorityWatchPoller(ctx context.Context, poller *LiveAdmissionPoller, log *slog.Logger) {
	StartLiveAdmissionPoller(ctx, poller, log)
}
