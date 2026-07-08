package analytics

import (
	"streamclone/internal/analytics/ingestcore"
)

func minuteRollupToIngestSnapshot(streamID string, rollup MinuteRollup, closed bool) ingestcore.RollupSnapshot {
	return ingestcore.RollupSnapshot{
		StreamID:          streamID,
		Minute:            rollup.MinuteTS,
		ViewerAvg:         rollup.ViewerAvg,
		ViewerMax:         rollup.ViewerMax,
		ViewerLatest:      rollup.ViewerLatest,
		ViewerSamples:     rollup.ViewerSamples,
		ChatCount:         rollup.ChatCount,
		TotalEmoteCount:   rollup.TotalEmoteCount,
		SevenTVEmoteCount: rollup.SevenTVEmoteCount,
		Emotes:            rollup.Emotes,
		Closed:            closed,
	}
}

// RecordLegacyShadowMinute forwards a legacy collector rollup into ingest-core shadow compare.
func RecordLegacyShadowMinute(engine *ingestcore.Engine, streamID, login string, rollup MinuteRollup, closed bool) {
	if engine == nil {
		return
	}
	if streamID != "" && login != "" {
		engine.BindStream(login, streamID)
	}
	engine.RecordLegacySnapshot(login, minuteRollupToIngestSnapshot(streamID, rollup, closed))
}
