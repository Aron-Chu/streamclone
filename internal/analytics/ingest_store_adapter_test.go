package analytics

import (
	"testing"
	"time"

	"streamclone/internal/analytics/ingestcore"
)

func TestIngestStoreAdapterSnapshotsToRollups(t *testing.T) {
	minute, err := time.Parse(time.RFC3339, "2026-07-08T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	snaps := []ingestcore.RollupSnapshot{
		{
			StreamID:        "s1",
			Minute:          minute,
			ChatCount:       10,
			TotalEmoteCount: 3,
			Emotes:          map[string]int{"7tv:e:K": 3},
			Closed:          true,
		},
	}
	byStream := snapshotsToByStream(snaps)
	rollups := byStream["s1"]
	if len(rollups) != 1 {
		t.Fatalf("rollups = %d", len(rollups))
	}
	if rollups[0].ChatCount != 10 {
		t.Fatalf("chat = %d", rollups[0].ChatCount)
	}
}
