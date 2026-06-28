package analytics

import (
	"encoding/json"
	"testing"
)

func TestExtensionHealthPublicShape(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())

	payload := h.extensionHealthPayload()
	if !payload.OK {
		t.Fatal("expected ok=true")
	}
	if payload.Version == "" {
		t.Fatal("expected version")
	}
	if payload.Time <= 0 {
		t.Fatal("expected time")
	}
	if !payload.Routes.PulseChannel || !payload.Routes.VodHint || !payload.Routes.Backfill {
		t.Fatalf("expected public route capabilities, got %+v", payload.Routes)
	}
	if !payload.Routes.PulseCoverage {
		t.Fatal("expected pulseCoverage route flag for read-only coverage endpoint")
	}
	if !payload.Routes.Jobs {
		t.Fatal("expected jobs route flag for backfill status endpoint")
	}
	if payload.Capabilities.Backfill || payload.Capabilities.MissedMomentsBackfill {
		t.Fatalf("expected backfill capability unavailable without manager, got %+v", payload.Capabilities)
	}
	if !payload.Degraded.Backfill {
		t.Fatalf("expected backfill degraded without manager, got %+v", payload.Degraded)
	}
}

func TestExtensionHealthDoesNotExposeCapsOrRateLimits(t *testing.T) {
	h := (&Handler{}).WithPulseRuntime(DefaultPulseRuntimeConfig())
	raw, err := json.Marshal(h.extensionHealthPayload())
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"caps", "rates", "killSwitches", "queues", "config"} {
		if _, ok := body[key]; ok {
			t.Fatalf("public health exposed operator key %q in %s", key, string(raw))
		}
	}
}

func TestAdminPulseHealthIncludesCapsSwitchesAndQueues(t *testing.T) {
	h := (&Handler{}).
		WithPulseHosted(PulseHostedConfig{
			Hosted:                  true,
			MaxActiveChannels:       10,
			MaxChannelsPerPrincipal: 3,
			WatchRatePerMin:         6,
			BackfillRatePerHour:     5,
		}).
		WithPulseRuntime(PulseRuntimeConfig{
			Configured:           true,
			HelixLiveEnabled:     true,
			HelixVodEnabled:      false,
			HelixMetadataEnabled: true,
			HelixGoLiveEnabled:   true,
			GQLCommentsEnabled:   true,
			BackfillEnabled:      true,
			BFFCacheEnabled:      true,
			RosterSize:           500,
			ProtectedGlobalLimit: 500,
		})

	payload := h.adminPulseHealthPayload()
	if payload.Caps.ActiveChannels != 10 || payload.Caps.ChannelsPerPrincipal != 3 {
		t.Fatalf("unexpected caps: %+v", payload.Caps)
	}
	if payload.Rates.WatchPerMin != 6 || payload.Rates.BackfillPerHour != 5 {
		t.Fatalf("unexpected rates: %+v", payload.Rates)
	}
	if payload.KillSwitches["PULSE_HELIX_VOD_ENABLED"] {
		t.Fatalf("expected vod split flag false in kill switches: %+v", payload.KillSwitches)
	}
	if payload.Config.HelixVodEnabled {
		t.Fatalf("expected admin config helixVodEnabled=false: %+v", payload.Config)
	}
}
