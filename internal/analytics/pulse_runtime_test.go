package analytics

import "testing"

func TestPulseHelixParentFlagDisablesAllSplitFlags(t *testing.T) {
	t.Setenv("PULSE_HELIX_ENABLED", "false")
	t.Setenv("PULSE_HELIX_VOD_ENABLED", "true")

	cfg := PulseRuntimeConfigFromEnv()
	if cfg.HelixLiveEnabled || cfg.HelixVodEnabled || cfg.HelixMetadataEnabled || cfg.HelixGoLiveEnabled {
		t.Fatalf("parent flag should disable all split flags: %+v", cfg)
	}
}

func TestPulseHelixVodDisabledDoesNotDisableLive(t *testing.T) {
	t.Setenv("PULSE_HELIX_VOD_ENABLED", "false")

	cfg := PulseRuntimeConfigFromEnv()
	if cfg.HelixVodEnabled {
		t.Fatalf("expected vod flag disabled: %+v", cfg)
	}
	if !cfg.HelixLiveEnabled {
		t.Fatalf("vod flag should not disable live flag: %+v", cfg)
	}
}
