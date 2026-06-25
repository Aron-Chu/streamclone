package analytics

import (
	"context"
	"log/slog"
	"time"

	"streamclone/internal/metrics"
)

// Top500SilverGate orchestrates one selective silver gate evaluation window.
type Top500SilverGate struct {
	cfg        SilverGateConfig
	candidates SilverCandidateReader
	counters   SilverBudgetCounterReader
	enqueue    SilverEnqueueAdapter
	log        *slog.Logger
}

// SilverGateTickSummary aggregates one dry-run or live gate tick.
type SilverGateTickSummary struct {
	CandidatesListed    int
	CandidatesEvaluated int
	Decisions           map[SilverGateDecisionReason]int
	DryRunAllows        int
	EnqueueAttempts     int
	EnqueueWrites       int
}

// NewTop500SilverGate wires readers and a refusing enqueue adapter for dry-run.
func NewTop500SilverGate(cfg SilverGateConfig, log *slog.Logger, candidates SilverCandidateReader, counters SilverBudgetCounterReader, enqueue SilverEnqueueAdapter) *Top500SilverGate {
	cfg = normalizeSilverGateConfig(cfg)
	if candidates == nil {
		candidates = NoopSilverCandidateReader{}
	}
	if counters == nil {
		counters = NoopSilverBudgetCounterReader{}
	}
	if enqueue == nil {
		enqueue = RefusingSilverEnqueueAdapter{WriteEnabled: cfg.WriteEnabled}
	}
	return &Top500SilverGate{
		cfg:        cfg,
		candidates: candidates,
		counters:   counters,
		enqueue:    enqueue,
		log:        log,
	}
}

// RunTick evaluates up to MaxCandidates and emits decisions/metrics without queue writes in dry-run.
func (g *Top500SilverGate) RunTick(ctx context.Context) (SilverGateTickSummary, error) {
	summary := SilverGateTickSummary{Decisions: make(map[SilverGateDecisionReason]int)}
	if g == nil || !g.cfg.Enabled {
		return summary, nil
	}

	start := time.Now()
	defer func() {
		metrics.Top500SilverGateDurationSeconds.WithLabelValues("tick", SilverGateLaneTop500Selective).
			Observe(time.Since(start).Seconds())
	}()

	limit := g.cfg.MaxCandidates
	candidates, err := g.candidates.ListCandidates(ctx, limit)
	if err != nil {
		return summary, err
	}
	summary.CandidatesListed = len(candidates)

	enqueueRemaining := g.cfg.MaxEnqueuePerRun
	for _, candidate := range candidates {
		summary.CandidatesEvaluated++
		metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "evaluate").Inc()

		budget, err := g.counters.ReadSnapshot(ctx, candidate.Login)
		if err != nil {
			return summary, err
		}
		result := EvaluateSilverGate(candidate, budget, g.cfg)
		RecordSilverGateDecision(result, SilverGateLaneTop500Selective, "evaluate")
		summary.Decisions[result.Decision]++

		if g.log != nil {
			g.log.Info("top500 silver gate candidate evaluated",
				"decision", result.Decision,
				"allow_enqueue", result.AllowEnqueue,
				"dry_run", g.cfg.DryRun,
				"write_enabled", g.cfg.WriteEnabled,
			)
		}

		if !result.AllowEnqueue {
			continue
		}
		if enqueueRemaining <= 0 {
			continue
		}
		enqueueRemaining--
		summary.EnqueueAttempts++

		if g.cfg.DryRun || !g.cfg.WriteEnabled {
			metrics.Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("dry_run", SilverGateLaneTop500Selective, "enqueue").Inc()
			summary.DryRunAllows++
			if g.log != nil {
				g.log.Info("top500 silver gate dry-run allow",
					"decision", result.Decision,
					"dry_run", g.cfg.DryRun,
					"write_enabled", g.cfg.WriteEnabled,
				)
			}
			continue
		}

		req := SilverEnqueueRequest{
			Tier:           "silver",
			Login:          candidate.Login,
			ChannelID:      candidate.ChannelID,
			StreamID:       candidate.StreamID,
			Source:         "top500_budget_gate",
			Priority:       candidate.PriorityScore,
			IdempotencyKey: "silver:" + normalizeLogin(candidate.Login) + ":" + candidate.StreamID,
			CreatedBy:      "top500_silver_gate",
		}
		metrics.Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("attempt", SilverGateLaneTop500Selective, "enqueue").Inc()
		inserted, err := g.enqueue.EnqueueSilver(ctx, req)
		if err != nil {
			metrics.Top500SilverGateEnqueueErrorsTotal.WithLabelValues("enqueue_error", SilverGateLaneTop500Selective, "enqueue").Inc()
			if g.log != nil {
				g.log.Warn("top500 silver gate enqueue refused", "err", err)
			}
			continue
		}
		if inserted {
			summary.EnqueueWrites++
		}
	}

	return summary, nil
}
