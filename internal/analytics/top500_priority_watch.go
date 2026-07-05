package analytics

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

// Top500PriorityWatchPoller admits live Top-N channels into the IRC collector at
// TrackPriorityTopRoster when explicitly enabled. Candidates come from a
// LiveAdmissionSource (Helix top-live by default, roster metadata as legacy).
type Top500PriorityWatchPoller struct {
	source    LiveAdmissionSource
	collector *Collector
	cfg       config.Config
	log       *slog.Logger
	interval  time.Duration
}

type admissionCycleState struct {
	trackedLogins map[string]struct{}
	active        int
	max           int
}

func NewTop500PriorityWatchPoller(source LiveAdmissionSource, collector *Collector, cfg config.Config, log *slog.Logger) *Top500PriorityWatchPoller {
	if log == nil {
		log = slog.Default()
	}
	interval := cfg.PulseTop500AdmissionInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Top500PriorityWatchPoller{
		source:    source,
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
		"source", normalizePulseTop500AdmissionSource(poller.cfg.PulseTop500AdmissionSource),
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
	if p == nil || p.source == nil || p.collector == nil {
		return
	}
	mode := TopRosterAdmissionModeDefault
	metrics.TopRosterAdmissionEnabled.Set(boolFloat(p.Enabled()))
	topN := p.cfg.PulseTop500AdmissionTopN
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	live, err := p.source.ListLiveCandidates(ctx, topN)
	if err != nil {
		p.log.Warn("top500 priority watch list failed", "err", err)
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
		p.log.Info("top500 priority watch admission cycle", "considered", 0, "admitted", 0, "skipped_by_reason", skippedByReason)
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
		p.collector.NoteGoLiveDetected(streamID, normalizeLogin(row.Login), "top_roster", TrackPriorityTopRoster, false)
		outcome = classifyTopRosterWatchResponse(resp)
		message = resp.Message
		if resp.Tracking {
			state.noteAdmission(normalizeLogin(row.Login), resp.Active, resp.Max)
		}
		recordTopRosterAdmissionMetrics(mode, outcome)
		recordTopRosterAdmissionAttempt(buildTopRosterAdmissionAttempt(row, streamID, now, outcome, message, state))
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

func newAdmissionCycleState(snap TrackingSnapshot) admissionCycleState {
	tracked := make(map[string]struct{}, len(snap.TrackedChannels))
	for _, login := range snap.TrackedChannels {
		if login = normalizeLogin(login); login != "" {
			tracked[login] = struct{}{}
		}
	}
	return admissionCycleState{
		trackedLogins: tracked,
		active:        snap.Active,
		max:           snap.Max,
	}
}

func (s *admissionCycleState) noteAdmission(login string, active, max int) {
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	if s.trackedLogins == nil {
		s.trackedLogins = map[string]struct{}{}
	}
	s.trackedLogins[login] = struct{}{}
	s.active = active
	s.max = max
}

func classifyTopRosterCandidate(c *Collector, tracked map[string]struct{}, row Top500Current) (outcome, message, streamID string) {
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
	case c != nil && c.TrackedStreamID(login) == streamID:
		return TopRosterAdmissionDuplicateStream, "duplicate stream id", streamID
	}
	if tracked != nil {
		if _, ok := tracked[login]; ok {
			return TopRosterAdmissionAlreadyTracking, "already tracking", streamID
		}
	}
	return "", "", streamID
}

func buildTopRosterAdmissionAttempt(row Top500Current, streamID string, attemptedAt time.Time, outcome, message string, state admissionCycleState) TopRosterAdmissionAttempt {
	login := normalizeLogin(row.Login)
	_, tracking := state.trackedLogins[login]
	return TopRosterAdmissionAttempt{
		Login:             login,
		Rank:              row.Rank,
		StreamID:          streamID,
		SampledAt:         row.SampledAt,
		AttemptedAt:       attemptedAt,
		Outcome:           outcome,
		Message:           message,
		CollectorTracking: tracking,
		ActiveCollectors:  state.active,
		MaxCollectors:     state.max,
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
