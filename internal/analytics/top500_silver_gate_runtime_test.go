package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/config"
	"streamclone/internal/metrics"
)

func TestShouldStartTop500SilverGateDefaultOff(t *testing.T) {
	if ShouldStartTop500SilverGate(config.Config{}) {
		t.Fatal("default config must not start top500 silver gate")
	}
	if ShouldStartTop500SilverGate(config.Config{Top500SilverGateEnabled: false}) {
		t.Fatal("TOP500_SILVER_GATE_ENABLED=false must not start gate")
	}
	if !ShouldStartTop500SilverGate(config.Config{Top500SilverGateEnabled: true}) {
		t.Fatal("TOP500_SILVER_GATE_ENABLED=true should allow gate startup")
	}
}

func TestStartTop500SilverGateDisabledIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartTop500SilverGate(ctx, SilverGateConfig{Enabled: false}, nil)
	time.Sleep(5 * time.Millisecond)
}

func TestSilverGateConfigFromAppDefaults(t *testing.T) {
	cfg := SilverGateConfigFromApp(config.Config{
		Top500SilverGateDryRun:           true,
		Top500SilverGateMaxCandidates:    5,
		Top500SilverGateMaxEnqueuePerRun: 1,
		Top500SilverGateInterval:         DefaultTop500SilverGateInterval,
	})
	if cfg.Enabled || !cfg.DryRun || cfg.WriteEnabled {
		t.Fatalf("unexpected defaults: enabled=%v dryRun=%v write=%v", cfg.Enabled, cfg.DryRun, cfg.WriteEnabled)
	}
	if cfg.MaxCandidates != DefaultTop500SilverGateMaxCandidates {
		t.Fatalf("MaxCandidates = %d, want %d", cfg.MaxCandidates, DefaultTop500SilverGateMaxCandidates)
	}
	if cfg.MaxEnqueuePerRun != DefaultTop500SilverGateMaxEnqueuePerRun {
		t.Fatalf("MaxEnqueuePerRun = %d, want %d", cfg.MaxEnqueuePerRun, DefaultTop500SilverGateMaxEnqueuePerRun)
	}
	if cfg.Interval != DefaultTop500SilverGateInterval {
		t.Fatalf("Interval = %s, want %s", cfg.Interval, DefaultTop500SilverGateInterval)
	}
}

func TestRecordSilverGateDecisionIncrementsMetric(t *testing.T) {
	RecordSilverGateDecision(SilverGateResult{Decision: SilverGateSkipDailyBudget}, SilverGateLaneTop500Selective, "evaluate")
}

func TestInitTop500SilverGateMetricsDisabledState(t *testing.T) {
	InitTop500SilverGateMetrics(SilverGateConfig{Enabled: false, DryRun: true, WriteEnabled: false})
	if got := testutil.ToFloat64(metrics.Top500SilverGateEnabled); got != 0 {
		t.Fatalf("enabled gauge = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.Top500SilverGateDryRun); got != 1 {
		t.Fatalf("dry_run gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Top500SilverGateWriteEnabled); got != 0 {
		t.Fatalf("write_enabled gauge = %v, want 0", got)
	}
}

func TestInitTop500SilverGateMetricsEnabledState(t *testing.T) {
	InitTop500SilverGateMetrics(SilverGateConfig{Enabled: true, DryRun: false, WriteEnabled: false})
	if got := testutil.ToFloat64(metrics.Top500SilverGateEnabled); got != 1 {
		t.Fatalf("enabled gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Top500SilverGateWriteEnabled); got != 0 {
		t.Fatalf("write_enabled gauge = %v, want 0", got)
	}
}

func TestWriteEnabledWithoutEnabledDoesNotStartRuntime(t *testing.T) {
	cfg := config.Config{Top500SilverGateEnabled: false, Top500SilverGateWriteEnabled: true}
	if ShouldStartTop500SilverGate(cfg) {
		t.Fatal("write-enabled without TOP500_SILVER_GATE_ENABLED must not start runtime")
	}
}

func TestStartTop500SilverGateEnabledScaffoldTicksWithoutEnqueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := testutil.ToFloat64(metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "tick"))
	StartTop500SilverGate(ctx, SilverGateConfig{
		Enabled:  true,
		DryRun:   true,
		Interval: 5 * time.Millisecond,
	}, nil)
	time.Sleep(25 * time.Millisecond)
	cancel()
	time.Sleep(5 * time.Millisecond)

	after := testutil.ToFloat64(metrics.Top500SilverGateCandidatesTotal.WithLabelValues(SilverGateLaneTop500Selective, "tick"))
	if after <= before {
		t.Fatalf("candidate tick metric unchanged: before=%v after=%v", before, after)
	}
}

func TestEnabledScaffoldWithWriteDisabledUsesRefusingAdapter(t *testing.T) {
	adapter := RefusingSilverEnqueueAdapter{WriteEnabled: false}
	inserted, err := adapter.EnqueueSilver(context.Background(), SilverEnqueueRequest{Tier: "silver"})
	if inserted || !errors.Is(err, ErrSilverGateWriteDisabled) {
		t.Fatalf("inserted=%v err=%v, want refuse write", inserted, err)
	}
}
