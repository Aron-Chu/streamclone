package analytics

import (
	"context"
	"sort"
	"time"

	"streamclone/internal/analytics/heatmap"
)

func (h *Handler) consolidateForHeatmap(ctx context.Context, streamID string) ([]heatmap.MinuteRollup, time.Time, error) {
	stream, err := h.store.StreamByID(ctx, streamID)
	if err != nil {
		return nil, time.Time{}, err
	}

	rawRollups, err := h.store.RollupsByStream(ctx, streamID)
	if err != nil {
		return nil, time.Time{}, err
	}
	rawRollups = filterTimelineRollups(rawRollups)

	consolidated := consolidateRollupsByMinute(rawRollups)

	out := make([]heatmap.MinuteRollup, 0, len(consolidated))
	for _, r := range consolidated {
		out = append(out, heatmap.MinuteRollup{
			MinuteTS:          r.MinuteTS,
			ViewerAvg:         r.ViewerAvg,
			ViewerMax:         r.ViewerMax,
			ViewerLatest:      r.ViewerLatest,
			ViewerSamples:     r.ViewerSamples,
			ChatCount:         r.ChatCount,
			TotalEmoteCount:   r.TotalEmoteCount,
			SevenTVEmoteCount: r.SevenTVEmoteCount,
			Emotes:            r.Emotes,
			Missing:           r.Missing,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].MinuteTS.Before(out[j].MinuteTS)
	})

	return out, stream.StartedAt, nil
}
