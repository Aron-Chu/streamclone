package analytics

import (
	"context"

	"streamclone/internal/analytics/ingestcore"
)

// IngestStoreAdapter implements ingestcore.BatchWriter using analytics Store.
type IngestStoreAdapter struct {
	Store *Store
}

func (a IngestStoreAdapter) WriteRollupBatch(ctx context.Context, open, closed []ingestcore.RollupSnapshot) error {
	if a.Store == nil {
		return nil
	}
	if len(open) > 0 {
		if err := a.Store.BulkUpsertLiveMinuteRollupsBatch(ctx, snapshotsToByStream(open), LiveRollupWriteOptions{Mode: LiveRollupWriteOpenMinute}); err != nil {
			return err
		}
	}
	if len(closed) > 0 {
		if err := a.Store.BulkUpsertLiveMinuteRollupsBatch(ctx, snapshotsToByStream(closed), LiveRollupWriteOptions{Mode: LiveRollupWriteCompletedMinute}); err != nil {
			return err
		}
	}
	return nil
}

func snapshotsToByStream(snaps []ingestcore.RollupSnapshot) map[string][]MinuteRollup {
	out := make(map[string][]MinuteRollup)
	for _, snap := range snaps {
		out[snap.StreamID] = append(out[snap.StreamID], MinuteRollup{
			MinuteTS:          snap.Minute,
			ViewerAvg:         snap.ViewerAvg,
			ViewerMax:         snap.ViewerMax,
			ViewerLatest:      snap.ViewerLatest,
			ViewerSamples:     snap.ViewerSamples,
			ChatCount:         snap.ChatCount,
			TotalEmoteCount:   snap.TotalEmoteCount,
			SevenTVEmoteCount: snap.SevenTVEmoteCount,
			Emotes:            snap.Emotes,
			ChatSource:        "irc",
			SourceConfidence:  "live",
		})
	}
	return out
}
