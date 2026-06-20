package ingest

import (
	"context"
	"log/slog"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/social"
	"streamclone/internal/social/reddit"
	"streamclone/internal/social/streamerbans"
	"streamclone/internal/social/youtube"
	"streamclone/internal/storygraph/preview"
	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/score"
	"streamclone/internal/storygraph/store"
)

// BackfillOptions configures historical import sweeps.
type BackfillOptions struct {
	Store       *store.Store
	Reliability *reliability.Registry
	Logger      *slog.Logger
	Config      config.Config
	Preview     *preview.Hydrator
	Since       time.Time
	Budget      social.Budget
	Persist     func(ctx context.Context, sourceName string, item social.Item)
}

// BackfillRunner imports historical social items with strict budgets.
// It does not synthesize historical trend snapshots — live trend remains forward-only.
type BackfillRunner struct {
	opts    BackfillOptions
	sources map[string]social.SocialSource
	score   *score.Engine
}

// NewBackfillRunner wires sources that can honestly provide historical data.
func NewBackfillRunner(opts BackfillOptions) *BackfillRunner {
	if opts.Preview == nil {
		opts.Preview = preview.NewHydrator(opts.Logger)
	}
	if opts.Budget.MaxItems <= 0 {
		opts.Budget.MaxItems = 32
	}
	cfg := opts.Config
	return &BackfillRunner{
		opts: opts,
		sources: map[string]social.SocialSource{
			"reddit":       reddit.NewSource(cfg),
			"youtube":      youtube.NewSource(cfg),
			"streamerbans": streamerbans.NewSource(cfg),
		},
		score: score.New(opts.Reliability),
	}
}

// Run executes backfill for named sources that advertise Backfill capability.
func (b *BackfillRunner) Run(ctx context.Context, sourceNames ...string) (map[string]int, error) {
	if b.opts.Store == nil {
		return nil, nil
	}
	if len(sourceNames) == 0 {
		sourceNames = []string{"reddit", "youtube", "streamerbans"}
	}
	since := b.opts.Since
	if since.IsZero() {
		since = time.Now().Add(-7 * 24 * time.Hour)
	}
	q := social.Query{
		Since:    since,
		Budget:   b.opts.Budget,
		Keywords: append([]string(nil), b.opts.Config.StorygraphYTKeywords...),
	}
	counts := map[string]int{}
	for _, name := range sourceNames {
		src, ok := b.sources[name]
		if !ok {
			continue
		}
		if !src.Capabilities().Backfill {
			if b.opts.Logger != nil {
				b.opts.Logger.Info("backfill skipped: source not marked for backfill", "source", name)
			}
			continue
		}
		backfiller, ok := src.(social.Backfiller)
		if !ok {
			if b.opts.Logger != nil {
				b.opts.Logger.Info("backfill skipped: Backfill not implemented", "source", name)
			}
			continue
		}
		page, err := backfiller.Backfill(ctx, q)
		if err != nil {
			if b.opts.Logger != nil {
				b.opts.Logger.Warn("backfill source failed", "source", name, "err", err)
			}
			continue
		}
		counts[name] = len(page.Items)
		if b.opts.Logger != nil {
			b.opts.Logger.Info("backfill imported items", "source", name, "count", len(page.Items))
		}
		if b.opts.Persist != nil {
			for _, item := range page.Items {
				b.opts.Persist(ctx, name, item)
			}
		}
	}
	return counts, nil
}

// SetPersist wires item persistence for imported backfill rows.
func (b *BackfillRunner) SetPersist(fn func(ctx context.Context, sourceName string, item social.Item)) {
	b.opts.Persist = fn
}
func HistoricalTrendLabel(imported bool) string {
	if imported {
		return "Historical evidence only — live trend unavailable for imported window"
	}
	return ""
}
