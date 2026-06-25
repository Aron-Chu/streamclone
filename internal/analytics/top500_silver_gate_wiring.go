package analytics

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
)

// NewTop500SilverGateFromApp builds a gate with fixture or read-only store candidates.
func NewTop500SilverGateFromApp(cfg config.Config, log *slog.Logger, store *Store, rdb *redis.Client) *Top500SilverGate {
	gateCfg := SilverGateConfigFromApp(cfg)
	var candidates SilverCandidateReader = NoopSilverCandidateReader{}
	var counters SilverBudgetCounterReader = NoopSilverBudgetCounterReader{}
	realCounterReader := false
	if cfg.Top500SilverGateFixtureCandidates {
		candidates = NewFixtureSilverCandidateReader()
		counters = NewFixtureSilverBudgetCounterReader()
	} else if store != nil && store.db != nil {
		candidates = StoreSilverCandidateReader{Store: store}
		counters = NewPostgresSilverBudgetCounterReader(store.db, rdb, cfg.AnalyticsTTScrapeBackoffEnabled)
		realCounterReader = true
	} else if store != nil {
		candidates = StoreSilverCandidateReader{Store: store}
	}
	enqueue := RefusingSilverEnqueueAdapter{WriteEnabled: gateCfg.WriteEnabled}
	gate := NewTop500SilverGate(gateCfg, log, candidates, counters, enqueue)
	gate.realCounterReader = realCounterReader
	return gate
}
