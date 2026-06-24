package analytics

import (
	"net/http"
)

type AdminPulseHealthResponse struct {
	ExtensionHealthResponse
	Caps         AdminPulseCaps   `json:"caps"`
	Rates        AdminPulseRates  `json:"rates"`
	KillSwitches map[string]bool  `json:"killSwitches"`
	Queues       AdminPulseQueues `json:"queues"`
	Config       AdminPulseConfig `json:"config"`
}

type AdminPulseCaps struct {
	ActiveChannels       int `json:"activeChannels"`
	Backfills            int `json:"backfills"`
	RosterSize           int `json:"rosterSize"`
	ProtectedGlobal      int `json:"protectedGlobal"`
	ChannelsPerPrincipal int `json:"channelsPerPrincipal"`
}

type AdminPulseRates struct {
	WatchPerMin     int `json:"watchPerMin"`
	BackfillPerHour int `json:"backfillPerHour"`
}

type AdminPulseQueues struct {
	ActiveBackfills       int `json:"activeBackfills"`
	BackfillCapacity      int `json:"backfillCapacity"`
	ActiveTrackedChannels int `json:"activeTrackedChannels"`
	TrackedCapacity       int `json:"trackedCapacity"`
}

type AdminPulseConfig struct {
	HostedMode             bool `json:"hostedMode"`
	HelixLiveEnabled       bool `json:"helixLiveEnabled"`
	HelixVodEnabled        bool `json:"helixVodEnabled"`
	HelixMetadataEnabled   bool `json:"helixMetadataEnabled"`
	HelixGoLiveEnabled     bool `json:"helixGoLiveEnabled"`
	GQLCommentsEnabled     bool `json:"gqlCommentsEnabled"`
	BackfillEnabled        bool `json:"backfillEnabled"`
	ProtectedGoLiveEnabled bool `json:"protectedGoLiveEnabled"`
	TopRosterPollEnabled   bool `json:"topRosterPollEnabled"`
}


func (h *Handler) adminPulseHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.adminPulseHealthPayload())
}

func (h *Handler) adminPulseHealthPayload() AdminPulseHealthResponse {
	RefreshPulseMetricGauges(h.collector, h.pulseBackfill)
	runtime := h.pulseRuntimeConfig()
	base := h.extensionHealthPayload()
	trackedActive, trackedMax := 0, 0
	if h.collector != nil {
		snap := h.collector.TrackingSnapshot()
		trackedActive = snap.Active
		trackedMax = snap.Max
	}
	backfillActive, backfillMax := 0, 0
	if h.pulseBackfill != nil {
		snap := h.pulseBackfill.Snapshot()
		backfillActive = snap.Active
		backfillMax = snap.Max
	}
	return AdminPulseHealthResponse{
		ExtensionHealthResponse: base,
		Caps: AdminPulseCaps{
			ActiveChannels:       h.pulseHosted.MaxActiveChannels,
			Backfills:            backfillMax,
			RosterSize:           runtime.RosterSize,
			ProtectedGlobal:      runtime.ProtectedGlobalLimit,
			ChannelsPerPrincipal: h.pulseHosted.MaxChannelsPerPrincipal,
		},
		Rates: AdminPulseRates{
			WatchPerMin:     h.pulseHosted.WatchRatePerMin,
			BackfillPerHour: h.pulseHosted.BackfillRatePerHour,
		},
		KillSwitches: map[string]bool{
			"PULSE_HELIX_LIVE_ENABLED":       runtime.HelixLiveEnabled,
			"PULSE_HELIX_VOD_ENABLED":        runtime.HelixVodEnabled,
			"PULSE_HELIX_METADATA_ENABLED":   runtime.HelixMetadataEnabled,
			"PULSE_HELIX_GOLIVE_ENABLED":     runtime.HelixGoLiveEnabled,
			"PULSE_GQL_COMMENTS_ENABLED":     runtime.GQLCommentsEnabled,
			"PULSE_BACKFILL_ENABLED":         runtime.BackfillEnabled,
			"PULSE_PROTECTED_GOLIVE_ENABLED": runtime.ProtectedGoLiveEnabled,
			"PULSE_TOP_ROSTER_POLL_ENABLED":  runtime.TopRosterPollEnabled,
			"PULSE_BFF_CACHE_ENABLED":        runtime.BFFCacheEnabled,
			"PULSE_READ_ONLY_MODE":           runtime.ReadOnlyMode,
		},
		Queues: AdminPulseQueues{
			ActiveBackfills:       backfillActive,
			BackfillCapacity:      backfillMax,
			ActiveTrackedChannels: trackedActive,
			TrackedCapacity:       trackedMax,
		},
		Config: AdminPulseConfig{
			HostedMode:             h.pulseHosted.Hosted,
			HelixLiveEnabled:       runtime.HelixLiveEnabled,
			HelixVodEnabled:        runtime.HelixVodEnabled,
			HelixMetadataEnabled:   runtime.HelixMetadataEnabled,
			HelixGoLiveEnabled:     runtime.HelixGoLiveEnabled,
			GQLCommentsEnabled:     runtime.GQLCommentsEnabled,
			BackfillEnabled:        runtime.BackfillEnabled,
			ProtectedGoLiveEnabled: runtime.ProtectedGoLiveEnabled,
			TopRosterPollEnabled:   runtime.TopRosterPollEnabled,
		},
	}
}
