package config

import (
	"os"
	"testing"
)

func TestLoadPulseTopRosterAdmissionAlias(t *testing.T) {
	t.Setenv("PULSE_TOP500_ADMISSION_ENABLED", "")
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_ENABLED", "true")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_TOP_N", "200")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PulseTop500AdmissionEnabled {
		t.Fatal("expected admission enabled via roster alias")
	}
	if cfg.PulseTop500AdmissionTopN != 200 {
		t.Fatalf("topN = %d", cfg.PulseTop500AdmissionTopN)
	}
}

func TestLoadPrefersCanonicalPulseTop500Env(t *testing.T) {
	t.Setenv("PULSE_TOP500_ADMISSION_ENABLED", "false")
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "150")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_ENABLED", "true")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_TOP_N", "200")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseTop500AdmissionEnabled {
		t.Fatal("canonical env should win")
	}
	if cfg.PulseTop500AdmissionTopN != 150 {
		t.Fatalf("topN = %d", cfg.PulseTop500AdmissionTopN)
	}
}

func TestLoadPrefersExplicitPulseTop500AdmissionTopNOverLiveAlias(t *testing.T) {
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "500")
	t.Setenv("LIVE_ADMISSION_TOP_N", "1000")
	t.Setenv("CORPUS_TARGET_TOP_N", "1000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseTop500AdmissionTopN != 500 {
		t.Fatalf("PulseTop500AdmissionTopN = %d, want explicit 500", cfg.PulseTop500AdmissionTopN)
	}
}

func TestLoadLiveAdmissionTopN5000NotResetTo100(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "5000")
	t.Setenv("PULSE_MAX_ACTIVE_CHANNELS", "5000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseTop500AdmissionTopN != 5000 {
		t.Fatalf("PulseTop500AdmissionTopN = %d, want 5000 (not corpus reset to 100)", cfg.PulseTop500AdmissionTopN)
	}
	if cfg.PulseMaxActiveChannels != 5000 {
		t.Fatalf("PulseMaxActiveChannels = %d, want 5000", cfg.PulseMaxActiveChannels)
	}
}

func TestLoadLiveAdmissionTopNDefaultsWhenUnset(t *testing.T) {
	clearTop500Env(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseTop500AdmissionTopN != DefaultLiveAdmissionTopN {
		t.Fatalf("PulseTop500AdmissionTopN = %d, want default %d", cfg.PulseTop500AdmissionTopN, DefaultLiveAdmissionTopN)
	}
}

func TestLoadLiveAdmissionTopNClampedToIRCMax(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "5000")
	t.Setenv("PULSE_MAX_ACTIVE_CHANNELS", "750")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseTop500AdmissionTopN != 750 {
		t.Fatalf("PulseTop500AdmissionTopN = %d, want clamped to IRC max 750", cfg.PulseTop500AdmissionTopN)
	}
}

func TestClampLiveAdmissionTopN(t *testing.T) {
	if got := ClampLiveAdmissionTopN(0, 5000); got != DefaultLiveAdmissionTopN {
		t.Fatalf("zero = %d, want default %d", got, DefaultLiveAdmissionTopN)
	}
	if got := ClampLiveAdmissionTopN(5000, 5000); got != 5000 {
		t.Fatalf("5000/5000 = %d", got)
	}
	if got := ClampLiveAdmissionTopN(5000, 750); got != 750 {
		t.Fatalf("5000/750 = %d, want 750", got)
	}
	if got := ClampLiveAdmissionTopN(6000, 0); got != MaxLiveAdmissionTopN {
		t.Fatalf("6000 = %d, want max %d", got, MaxLiveAdmissionTopN)
	}
}

func init() {
	_ = os.Unsetenv("PULSE_TOP500_ADMISSION_ENABLED")
}
