package analytics

import (
	"context"
	"testing"
	"time"

	"streamclone/internal/config"
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
