package analytics

import (
	"streamclone/internal/analytics/ingestcore"
)

// HubIngest is an additive public hub block for ingest-core observability.
type HubIngest struct {
	TieringEnabled    bool    `json:"tieringEnabled"`
	CoreEnabled       bool    `json:"coreEnabled"`
	DualReadMode      bool    `json:"dualReadMode"`
	ShadowMode        bool    `json:"shadowMode"`
	ActiveCollectors  int     `json:"activeCollectors"`
	DesiredCollectors int     `json:"desiredCollectors"`
	AdmitLagSeconds   float64 `json:"admitLagSeconds"`
	JoinRate1m        float64 `json:"joinRate1m"`
	PartRate1m        float64 `json:"partRate1m"`
	State             string  `json:"state"`
}

func buildHubIngest(engine *ingestcore.Engine) HubIngest {
	if engine == nil {
		return HubIngest{State: "legacy"}
	}
	cfg := engine.Config()
	snap := engine.ManagerSnapshot()
	state := "operational"
	if snap.DesiredCollectors > snap.ActiveCollectors {
		state = "admit_lag"
	}
	if snap.ActiveCollectors >= cfg.MaxActiveIRC && snap.DesiredCollectors > snap.ActiveCollectors {
		state = "saturated"
	}
	return HubIngest{
		TieringEnabled:    cfg.TieringEnabled,
		CoreEnabled:       cfg.CoreEnabled,
		DualReadMode:      cfg.DualReadMode,
		ShadowMode:        cfg.ShadowMode,
		ActiveCollectors:  snap.ActiveCollectors,
		DesiredCollectors: snap.DesiredCollectors,
		AdmitLagSeconds:   snap.AdmitLagSeconds,
		JoinRate1m:        snap.JoinRate1m,
		PartRate1m:        snap.PartRate1m,
		State:             state,
	}
}
