package config

import (
	"os"
	"testing"
)

func TestLoadPulseTopRosterAdmissionAlias(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("PULSE_TOP500_ADMISSION_ENABLED", "")
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_ENABLED", "true")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_TOP_N", "200")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PulseLiveAdmissionEnabled {
		t.Fatal("expected admission enabled via roster alias")
	}
	if cfg.PulseLiveAdmissionTopN != 200 {
		t.Fatalf("topN = %d", cfg.PulseLiveAdmissionTopN)
	}
}

func TestLoadPrefersCanonicalPulseTop500Env(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("PULSE_TOP500_ADMISSION_ENABLED", "false")
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "150")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_ENABLED", "true")
	t.Setenv("PULSE_TOP_ROSTER_ADMISSION_TOP_N", "200")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseLiveAdmissionEnabled {
		t.Fatal("canonical env should win")
	}
	if cfg.PulseLiveAdmissionTopN != 150 {
		t.Fatalf("topN = %d", cfg.PulseLiveAdmissionTopN)
	}
}

func TestLoadPrefersExplicitPulseLiveAdmissionTopNOverLiveAlias(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("PULSE_TOP500_ADMISSION_TOP_N", "500")
	t.Setenv("LIVE_ADMISSION_TOP_N", "1000")
	t.Setenv("CORPUS_TARGET_TOP_N", "1000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseLiveAdmissionTopN != 500 {
		t.Fatalf("PulseLiveAdmissionTopN = %d, want explicit 500", cfg.PulseLiveAdmissionTopN)
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
	if cfg.PulseLiveAdmissionTopN != 5000 {
		t.Fatalf("PulseLiveAdmissionTopN = %d, want 5000 (not corpus reset to 100)", cfg.PulseLiveAdmissionTopN)
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
	if cfg.PulseLiveAdmissionTopN != DefaultLiveAdmissionTopN {
		t.Fatalf("PulseLiveAdmissionTopN = %d, want default %d", cfg.PulseLiveAdmissionTopN, DefaultLiveAdmissionTopN)
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
	if cfg.PulseLiveAdmissionTopN != 750 {
		t.Fatalf("PulseLiveAdmissionTopN = %d, want clamped to IRC max 750", cfg.PulseLiveAdmissionTopN)
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
