package analytics

import (
	"context"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

// TestTop500SilverGateLocalDryRunWindow exercises one LOAD-003c local dry-run window with fixtures.
func TestTop500SilverGateLocalDryRunWindow(t *testing.T) {
	ctx := context.Background()
	cfg := SilverGateConfig{
		Enabled:          true,
		DryRun:           true,
		WriteEnabled:     false,
		MaxCandidates:    5,
		MaxEnqueuePerRun: 1,
	}
	gate := NewTop500SilverGate(
		cfg,
		slog.Default(),
		NewFixtureSilverCandidateReader(),
		NewFixtureSilverBudgetCounterReader(),
		RefusingSilverEnqueueAdapter{WriteEnabled: false},
	)

	decisionsBefore := testutil.ToFloat64(metrics.Top500SilverGateDecisionsTotal.WithLabelValues("allow", string(SilverGateAllowEnqueue), SilverGateLaneTop500Selective, "evaluate"))
	candidatesBefore := testutil.ToFloat64(metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "evaluate"))
	attemptsBefore := testutil.ToFloat64(metrics.Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("dry_run", SilverGateLaneTop500Selective, "enqueue"))

	summary, err := RunTop500SilverGateOnce(ctx, gate)
	if err != nil {
		t.Fatalf("RunTop500SilverGateOnce err = %v", err)
	}
	if summary.CandidatesListed != 5 {
		t.Fatalf("CandidatesListed = %d, want 5 fixture candidates", summary.CandidatesListed)
	}
	if summary.CandidatesEvaluated != 5 {
		t.Fatalf("CandidatesEvaluated = %d, want 5", summary.CandidatesEvaluated)
	}
	if summary.EnqueueWrites != 0 {
		t.Fatalf("EnqueueWrites = %d, want 0", summary.EnqueueWrites)
	}
	if summary.DryRunAllows != 1 {
		t.Fatalf("DryRunAllows = %d, want 1 (max_enqueue_per_run=1)", summary.DryRunAllows)
	}
	if summary.Decisions[SilverGateAllowEnqueue] != 1 {
		t.Fatalf("allow_enqueue decisions = %d, want 1", summary.Decisions[SilverGateAllowEnqueue])
	}
	for _, reason := range []SilverGateDecisionReason{
		SilverGateSkipMetadataStale,
		SilverGateSkipMissingStreamID,
		SilverGateSkipDuplicateJob,
		SilverGateSkipDailyBudget,
	} {
		if summary.Decisions[reason] != 1 {
			t.Fatalf("decision %q count = %d, want 1", reason, summary.Decisions[reason])
		}
	}

	if delta := testutil.ToFloat64(metrics.Top500SilverGateDecisionsTotal.WithLabelValues("allow", string(SilverGateAllowEnqueue), SilverGateLaneTop500Selective, "evaluate")) - decisionsBefore; delta != 1 {
		t.Fatalf("allow decision metric delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "evaluate")) - candidatesBefore; delta != 5 {
		t.Fatalf("candidate evaluate metric delta = %v, want 5", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("dry_run", SilverGateLaneTop500Selective, "enqueue")) - attemptsBefore; delta != 1 {
		t.Fatalf("dry_run enqueue attempt delta = %v, want 1", delta)
	}
	if got := testutil.ToFloat64(metrics.Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("attempt", SilverGateLaneTop500Selective, "enqueue")); got > 0 {
		t.Fatalf("real enqueue attempts = %v, want 0 in dry-run", got)
	}
}

func TestTop500SilverGateLocalDryRunDisabledGateIsNoOp(t *testing.T) {
	gate := NewTop500SilverGate(SilverGateConfig{Enabled: false, DryRun: true}, nil, NewFixtureSilverCandidateReader(), NewFixtureSilverBudgetCounterReader(), RefusingSilverEnqueueAdapter{})
	summary, err := RunTop500SilverGateOnce(context.Background(), gate)
	if err != nil {
		t.Fatalf("RunTop500SilverGateOnce err = %v", err)
	}
	if summary.CandidatesEvaluated != 0 {
		t.Fatalf("CandidatesEvaluated = %d, want 0 when disabled", summary.CandidatesEvaluated)
	}
}

func TestNewTop500SilverGateFromAppUsesFixtureWhenConfigured(t *testing.T) {
	cfg := config.Config{
		Top500SilverGateEnabled:           true,
		Top500SilverGateDryRun:            true,
		Top500SilverGateWriteEnabled:      false,
		Top500SilverGateFixtureCandidates: true,
	}
	gate := NewTop500SilverGateFromApp(cfg, nil, nil, nil)
	if gate == nil {
		t.Fatal("gate is nil")
	}
	if !gate.cfg.Enabled || !gate.cfg.DryRun || gate.cfg.WriteEnabled {
		t.Fatalf("unexpected gate cfg: %+v", gate.cfg)
	}
	summary, err := gate.RunTick(context.Background())
	if err != nil {
		t.Fatalf("RunTick err = %v", err)
	}
	if summary.Decisions[SilverGateAllowEnqueue] != 1 {
		t.Fatalf("allow decisions = %d, want 1", summary.Decisions[SilverGateAllowEnqueue])
	}
}

func TestFixtureSilverCandidateReaderReturnsFiveLocalCandidates(t *testing.T) {
	reader := NewFixtureSilverCandidateReader()
	out, err := reader.ListCandidates(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListCandidates err = %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("len = %d, want 5", len(out))
	}
}

func TestFixtureSilverBudgetCounterReaderFailClosedOverrides(t *testing.T) {
	reader := NewFixtureSilverBudgetCounterReader()
	dup, err := reader.ReadSnapshot(context.Background(), fixtureSilverDuplicateLogin, "fixture-stream-dup")
	if err != nil || !dup.DuplicateQueuedOrRunning {
		t.Fatalf("dup snapshot = %+v err=%v", dup, err)
	}
	budget, err := reader.ReadSnapshot(context.Background(), fixtureSilverBudgetLogin, "fixture-stream-budget")
	if err != nil || budget.SilverEnqueuedToday != SilverGateGlobalMaxEnqueuePerDay {
		t.Fatalf("budget snapshot = %+v err=%v", budget, err)
	}
}
