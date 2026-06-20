package analytics

import (
	"context"
	"log/slog"
	"time"
)

// ViewerSampler polls Helix viewer counts for tracked live streamers.
type ViewerSampler struct {
	store    *Store
	helix    *HelixClient
	roster   *RosterSyncer
	interval time.Duration
	log      *slog.Logger
}

func NewViewerSampler(store *Store, helix *HelixClient, roster *RosterSyncer, interval time.Duration, log *slog.Logger) *ViewerSampler {
	if interval <= 0 {
		interval = 45 * time.Second
	}
	return &ViewerSampler{
		store:    store,
		helix:    helix,
		roster:   roster,
		interval: interval,
		log:      log,
	}
}

func (v *ViewerSampler) SampleOnce(ctx context.Context) error {
	if v == nil || v.store == nil || v.helix == nil || !v.helix.Enabled() {
		return nil
	}
	tracked, err := v.roster.ListLiveTracked(ctx, 200)
	if err != nil {
		return err
	}
	if len(tracked) == 0 {
		return nil
	}
	logins := make([]string, 0, len(tracked))
	for _, row := range tracked {
		if login := normalizeLogin(row.Login); login != "" {
			logins = append(logins, login)
		}
	}
	streams, err := v.helix.StreamsByLogin(ctx, logins)
	if err != nil {
		return err
	}
	profiles, err := v.helix.UsersByLogin(ctx, logins)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	minute := now.Truncate(time.Minute)
	for login, stream := range streams {
		profile := profiles[login]
		if err := v.store.UpsertLiveStream(ctx, stream, profile, now); err != nil {
			if v.log != nil {
				v.log.Warn("tier-0 live stream upsert failed", "login", login, "err", err)
			}
			continue
		}
		rollup := MinuteRollup{
			MinuteTS:      minute,
			ViewerAvg:     stream.ViewerCount,
			ViewerMax:     stream.ViewerCount,
			ViewerLatest:  stream.ViewerCount,
			ViewerSamples: 1,
			Emotes:        map[string]int{},
		}
		if err := v.store.UpsertMinuteRollup(ctx, stream.ID, rollup); err != nil && v.log != nil {
			v.log.Warn("tier-0 minute rollup failed", "stream_id", stream.ID, "err", err)
		}
	}
	return nil
}

func StartViewerSampler(ctx context.Context, sampler *ViewerSampler) {
	if sampler == nil || sampler.interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(sampler.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sampler.SampleOnce(ctx); err != nil && sampler.log != nil {
					sampler.log.Warn("tier-0 viewer sample failed", "err", err)
				}
			}
		}
	}()
}

// Tier0CoveragePct returns live viewer minute coverage for shouldSkipTracker heuristics.
func Tier0CoveragePct(stream *StreamRecord, rollups []MinuteRollup) float64 {
	if stream == nil {
		return 0
	}
	if normalizeViewerSource(stream.ViewerSource) == ViewerSourceLive {
		return 100
	}
	duration := streamDurationSeconds(stream, rollups)
	if duration <= 0 {
		return 0
	}
	minutesWithData := 0
	for _, r := range rollups {
		if r.ViewerSamples > 0 || r.ViewerAvg > 0 {
			minutesWithData++
		}
	}
	expected := duration / 60
	if expected <= 0 {
		return 0
	}
	pct := float64(minutesWithData) / float64(expected) * 100
	if pct > 100 {
		return 100
	}
	return pct
}
