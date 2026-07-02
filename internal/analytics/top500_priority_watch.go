package analytics

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

// Top500PriorityWatchPoller admits live Top-N metadata channels into the IRC collector
// at TrackPriorityTopRoster when explicitly enabled. It reads top500_current only —
// no archive or corpus enqueue.
type Top500PriorityWatchPoller struct {
	store     top500PriorityWatchStore
	collector *Collector
	cfg       config.Config
	log       *slog.Logger
	interval  time.Duration
}

type top500PriorityWatchStore interface {
	ListTop500LiveForPriorityWatch(ctx context.Context, topN, limit int) ([]Top500Current, error)
}

func NewTop500PriorityWatchPoller(store top500PriorityWatchStore, collector *Collector, cfg config.Config, log *slog.Logger) *Top500PriorityWatchPoller {
	if log == nil {
		log = slog.Default()
	}
	interval := cfg.PulseTop500AdmissionInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Top500PriorityWatchPoller{
		store:     store,
		collector: collector,
		cfg:       cfg,
		log:       log,
		interval:  interval,
	}
}

func (p *Top500PriorityWatchPoller) Enabled() bool {
	return p != nil && p.cfg.PulseTop500AdmissionEnabled
}

func StartTop500PriorityWatchPoller(ctx context.Context, poller *Top500PriorityWatchPoller, log *slog.Logger) {
	if poller == nil || !poller.Enabled() {
		metrics.TopRosterAdmissionEnabled.Set(0)
		recordTopRosterAdmissionSkip(TopRosterAdmissionModeDefault, TopRosterAdmissionSkipDisabled)
		if log != nil {
			log.Info("top500 priority watch poller disabled", "reason", TopRosterAdmissionSkipDisabled)
		}
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Info("top500 priority watch poller started",
		"interval", poller.interval.String(),
		"top_n", poller.cfg.PulseTop500AdmissionTopN,
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

func (p *Top500PriorityWatchPoller) runOnce(ctx context.Context) {
	if p == nil || p.store == nil || p.collector == nil {
		return
	}
	mode := TopRosterAdmissionModeDefault
	metrics.TopRosterAdmissionEnabled.Set(boolFloat(p.Enabled()))
	topN := p.cfg.PulseTop500AdmissionTopN
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	live, err := p.store.ListTop500LiveForPriorityWatch(ctx, topN, topN)
	if err != nil {
		p.log.Warn("top500 priority watch list failed", "err", err)
		return
	}
	metrics.TopRosterAdmissionLiveConsidered.Set(float64(len(live)))
	snap := p.collector.TrackingSnapshot()
	metrics.TopRosterAdmissionActiveCollectors.Set(float64(snap.Active))
	skippedByReason := map[string]int{}
	if snap.Max <= 0 {
		recordTopRosterAdmissionSkip(TopRosterAdmissionModeDefault, TopRosterAdmissionSkipEnvMismatch)
		skippedByReason[TopRosterAdmissionSkipEnvMismatch]++
	}
	if len(live) == 0 {
		metrics.TopRosterAdmissionZeroChatLiveRows.Set(0)
		p.log.Info("top500 priority watch admission cycle", "considered", 0, "admitted", 0, "skipped_by_reason", skippedByReason)
		return
	}
	now := time.Now().UTC()
	capacityBlocked := false
	admitted := 0
	for _, row := range live {
		outcome, message, streamID := classifyTopRosterCandidate(p, row)
		if outcome == TopRosterAdmissionEmptyLogin || outcome == TopRosterAdmissionEmptyStreamID || outcome == TopRosterAdmissionNotLive {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, message, skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, message, snap))
			continue
		}
		if outcome == TopRosterAdmissionDuplicateStream {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, "duplicate stream id already tracked", skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, "duplicate stream id already tracked", snap))
			continue
		}
		if outcome == TopRosterAdmissionAlreadyTracking {
			recordTopRosterAdmissionMetrics(mode, outcome)
			recordTopRosterAdmissionSkipForOutcome(mode, outcome, "already tracking", skippedByReason)
			recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, "already tracking", snap))
			continue
		}
		resp := p.collector.WatchWithPriority(ctx, normalizeLogin(row.Login), "", TrackPriorityTopRoster)
		p.collector.NoteGoLiveDetected(streamID, normalizeLogin(row.Login), "top_roster", TrackPriorityTopRoster, false)
		outcome = classifyTopRosterWatchResponse(resp)
		message = resp.Message
		snap = p.collector.TrackingSnapshot()
		recordTopRosterAdmissionMetrics(mode, outcome)
		recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, message, snap))
		if resp.Tracking {
			admitted++
			p.log.Info("top500 priority watch admitted", "login", normalizeLogin(row.Login), "stream_id", streamID, "active", resp.Active, "max", resp.Max)
			continue
		}
		recordTopRosterAdmissionSkipForOutcome(mode, outcome, message, skippedByReason)
		p.log.Debug("top500 priority watch skipped", "login", normalizeLogin(row.Login), "message", resp.Message, "active", resp.Active, "max", resp.Max)
		if outcome == TopRosterAdmissionCapacityFull {
			capacityBlocked = true
			break
		}
	}
	if capacityBlocked {
		metrics.TopRosterAdmissionCapacityBlockedTotal.Inc()
	}
	p.log.Info("top500 priority watch admission cycle", "considered", len(live), "admitted", admitted, "skipped_by_reason", skippedByReason)
}

func classifyTopRosterCandidate(p *Top500PriorityWatchPoller, row Top500Current) (outcome, message, streamID string) {
	login := normalizeLogin(row.Login)
	streamID = ""
	if row.StreamID != nil {
		streamID = strings.TrimSpace(*row.StreamID)
	}
	switch {
	case login == "":
		return TopRosterAdmissionEmptyLogin, "login required", streamID
	case !row.IsLive:
		return TopRosterAdmissionNotLive, "not live", streamID
	case streamID == "":
		return TopRosterAdmissionEmptyStreamID, "stream id required", streamID
	case p.collector.TrackedStreamID(login) == streamID:
		return TopRosterAdmissionDuplicateStream, "duplicate stream id", streamID
	case p.collector.IsTracking(login):
		return TopRosterAdmissionAlreadyTracking, "already tracking", streamID
	default:
		return "", "", streamID
	}
}

func buildTopRosterAdmissionAttempt(row Top500Current, streamID string, attemptedAt time.Time, outcome, message string, snap TrackingSnapshot) TopRosterAdmissionAttempt {
	login := normalizeLogin(row.Login)
	return TopRosterAdmissionAttempt{
		Login:             login,
		Rank:              row.Rank,
		StreamID:          streamID,
		SampledAt:         row.SampledAt,
		AttemptedAt:       attemptedAt,
		Outcome:           outcome,
		Message:           message,
		CollectorTracking: snap.Active > 0 && containsString(snap.TrackedChannels, login),
		ActiveCollectors:  snap.Active,
		MaxCollectors:     snap.Max,
	}
}

func recordTopRosterAdmissionMetrics(mode, outcome string) {
	if outcome == "" {
		return
	}
	metrics.TopRosterAdmissionAttemptsTotal.WithLabelValues(outcome, mode).Inc()
}

func recordTopRosterAdmissionSkipForOutcome(mode, outcome, message string, skippedByReason map[string]int) {
	reason := topRosterAdmissionSkipReason(outcome, message)
	if reason == "" {
		return
	}
	recordTopRosterAdmissionSkip(mode, reason)
	if skippedByReason != nil {
		skippedByReason[reason]++
	}
}

func recordTopRosterAdmissionSkip(mode, reason string) {
	if reason == "" {
		return
	}
	metrics.TopRosterAdmissionSkippedTotal.WithLabelValues(reason, mode).Inc()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
