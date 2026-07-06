package analytics

import (
	"testing"
	"time"
)

func TestViewersAtOffsetUsesMinuteRollup(t *testing.T) {
	streamStart := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmap(streamStart, 5, 2, 90)
	rollups[2].ViewerLatest = 1450
	if got := viewersAtOffset(rollups, points, 2*60); got != 1450 {
		t.Fatalf("viewersAtOffset(2m) = %d, want 1450", got)
	}
	if got := viewersAtOffset(rollups, points, 999*60); got != 0 {
		t.Fatalf("viewersAtOffset(missing) = %d, want 0", got)
	}
}

func TestViewersAtOffsetFallsBackToViewerAvg(t *testing.T) {
	streamStart := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmap(streamStart, 5, 2, 90)
	rollups[2].ViewerLatest = 0
	rollups[2].ViewerAvg = 980
	if got := viewersAtOffset(rollups, points, 2*60); got != 980 {
		t.Fatalf("viewersAtOffset(avg fallback) = %d, want 980", got)
	}
}
