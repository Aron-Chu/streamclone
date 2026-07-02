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

func init() {
	_ = os.Unsetenv("PULSE_TOP500_ADMISSION_ENABLED")
}
