package ingestcore

import (
	"context"
	"log/slog"
	"time"
)

// CandidateSource lists live channels for tier scheduling.
type CandidateSource interface {
	ListLiveCandidates(ctx context.Context, topN int) ([]SchedulerCandidate, error)
}

// SchedulerCandidate is a minimal live row for tier assignment.
type SchedulerCandidate struct {
	Login     string
	StreamID  string
	IsLive    bool
	HelixRank int
	Priority  int
}

// TierScheduler periodically reconciles desired IRC collectors.
type TierScheduler struct {
	cfg          Config
	manager      *CollectorManager
	source       CandidateSource
	log          *slog.Logger
	extraDesired func() []DesiredChannel
}

// NewTierScheduler wires scheduler to manager and candidate source.
func NewTierScheduler(cfg Config, manager *CollectorManager, source CandidateSource, log *slog.Logger) *TierScheduler {
	if log == nil {
		log = slog.Default()
	}
	return &TierScheduler{cfg: cfg, manager: manager, source: source, log: log}
}

// RunOnce executes one reconcile cycle.
func (s *TierScheduler) RunOnce(ctx context.Context) ReconcileResult {
	if s == nil || s.manager == nil || s.source == nil {
		return ReconcileResult{}
	}
	topN := s.cfg.HubRosterLimit
	if !s.cfg.TieringEnabled {
		if s.cfg.MaxActiveIRC > 0 {
			topN = s.cfg.MaxActiveIRC
		}
	}
	rows, err := s.source.ListLiveCandidates(ctx, topN)
	if err != nil {
		s.log.Warn("ingest scheduler list failed", "err", err)
		return ReconcileResult{}
	}
	desired := make([]DesiredChannel, 0, len(rows))
	for i, row := range rows {
		if !row.IsLive {
			continue
		}
		rank := row.HelixRank
		if rank <= 0 {
			rank = i + 1
		}
		priority := row.Priority
		if priority <= 0 {
			priority = 10
		}
		tier := AssignTier(s.cfg, priority, rank, s.cfg.TieringEnabled)
		if !s.cfg.TieringEnabled {
			tier = TierP1Hot
		}
		desired = append(desired, DesiredChannel{
			Login:         normalizeLogin(row.Login),
			StreamID:      row.StreamID,
			Tier:          tier,
			TrackPriority: priority,
			HelixRank:     rank,
		})
	}
	if s.extraDesired != nil {
		desired = mergeDesiredChannels(desired, s.extraDesired())
	}
	return s.manager.Reconcile(desired)
}

func mergeDesiredChannels(base, extra []DesiredChannel) []DesiredChannel {
	if len(extra) == 0 {
		return base
	}
	byLogin := make(map[string]DesiredChannel, len(base)+len(extra))
	for _, d := range base {
		byLogin[normalizeLogin(d.Login)] = d
	}
	for _, d := range extra {
		login := normalizeLogin(d.Login)
		if login == "" {
			continue
		}
		d.Login = login
		existing, ok := byLogin[login]
		if !ok || d.Tier < existing.Tier || (d.Tier == existing.Tier && d.TrackPriority > existing.TrackPriority) {
			byLogin[login] = d
		}
	}
	out := make([]DesiredChannel, 0, len(byLogin))
	for _, d := range byLogin {
		out = append(out, d)
	}
	return out
}

// Start runs the scheduler loop until ctx is cancelled.
func (s *TierScheduler) Start(ctx context.Context, interval time.Duration) {
	if s == nil || interval <= 0 {
		return
	}
	if s.manager != nil {
		s.manager.SetRunContext(ctx)
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.RunOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunOnce(ctx)
			}
		}
	}()
}
