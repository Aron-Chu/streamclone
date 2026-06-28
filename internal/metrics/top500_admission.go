package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TopRosterAdmissionEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top_roster_admission_enabled",
		Help: "Whether Top Roster priority IRC admission is enabled in the current process.",
	})
	TopRosterAdmissionLiveConsidered = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top_roster_admission_live_considered",
		Help: "Live Top Roster metadata rows considered on the latest admission poll.",
	})
	TopRosterAdmissionActiveCollectors = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top_roster_admission_active_collectors",
		Help: "Active IRC collector channels after the latest Top Roster admission poll.",
	})
	TopRosterAdmissionZeroChatLiveRows = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamclone_top_roster_admission_zero_chat_live_rows",
		Help: "Live Top Roster rows with fresh metadata but zero chat/emote rollups after the age threshold.",
	})
	TopRosterAdmissionAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamclone_top_roster_admission_attempts_total",
		Help: "Top Roster admission poll outcomes by result and mode.",
	}, []string{"outcome", "mode"})
	TopRosterAdmissionCapacityBlockedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "streamclone_top_roster_admission_capacity_blocked_total",
		Help: "Top Roster admission polls stopped because the IRC collector pool was full.",
	})
)
