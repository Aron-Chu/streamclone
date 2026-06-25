package analytics

import (
	"log/slog"

	"streamclone/internal/config"
)

// NewTop500SilverGateFromApp builds a gate with fixture or read-only store candidates.
func NewTop500SilverGateFromApp(cfg config.Config, log *slog.Logger, store *Store) *Top500SilverGate {
	gateCfg := SilverGateConfigFromApp(cfg)
	var candidates SilverCandidateReader = NoopSilverCandidateReader{}
	var counters SilverBudgetCounterReader = NoopSilverBudgetCounterReader{}
	if cfg.Top500SilverGateFixtureCandidates {
		candidates = NewFixtureSilverCandidateReader()
		counters = NewFixtureSilverBudgetCounterReader()
	} else if store != nil {
		candidates = StoreSilverCandidateReader{Store: store}
		counters = HealthySilverBudgetCounterReader{}
	}
	enqueue := RefusingSilverEnqueueAdapter{WriteEnabled: gateCfg.WriteEnabled}
	return NewTop500SilverGate(gateCfg, log, candidates, counters, enqueue)
}
