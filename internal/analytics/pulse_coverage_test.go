package analytics

import (
	"testing"
	"time"

	"streamclone/internal/analytics/heatmap"
)

func TestComputePulseCoverageFullStream(t *testing.T) {
	streamStart := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	var rollups []heatmap.MinuteRollup
	for i := 0; i < 30; i++ {
		rollups = append(rollups, heatmap.MinuteRollup{
			MinuteTS:  streamStart.Add(time.Duration(i) * time.Minute),
			ChatCount: 5,
		})
	}
	cov := computePulseCoverage(rollups, streamStart, 30*60, true, "123", false, false)
	if !cov.HasFullStreamCoverage {
		t.Fatalf("expected full coverage, got %+v", cov)
	}
	if cov.State != CoverageStateFullStreamTracked {
		t.Fatalf("state = %q, want %q", cov.State, CoverageStateFullStreamTracked)
	}
}

func TestComputePulseCoverageMissingPrefix(t *testing.T) {
	streamStart := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	rollups := []heatmap.MinuteRollup{
		{MinuteTS: streamStart.Add(178 * time.Minute), ChatCount: 12},
		{MinuteTS: streamStart.Add(179 * time.Minute), ChatCount: 8},
	}
	cov := computePulseCoverage(rollups, streamStart, 180*60, true, "999", false, false)
	if cov.HasFullStreamCoverage {
		t.Fatal("expected partial coverage")
	}
	if len(cov.MissingRanges) == 0 {
		t.Fatal("expected missing prefix range")
	}
	if cov.MissingRanges[0].FromOffsetSeconds != 0 {
		t.Fatalf("missing from = %d, want 0", cov.MissingRanges[0].FromOffsetSeconds)
	}
	if !cov.CanBackfill {
		t.Fatal("expected canBackfill with vod")
	}
	if cov.BackfillReason != "vod_available" {
		t.Fatalf("backfillReason = %q", cov.BackfillReason)
	}
}

func TestComputePulseCoverageInternalGap(t *testing.T) {
	streamStart := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	rollups := []heatmap.MinuteRollup{
		{MinuteTS: streamStart, ChatCount: 5},
		{MinuteTS: streamStart.Add(10 * time.Minute), ChatCount: 5},
	}
	cov := computePulseCoverage(rollups, streamStart, 600, false, "123", false, false)
	if !cov.HasGaps {
		t.Fatalf("expected gaps, got %+v", cov)
	}
	if len(cov.MissingRanges) != 1 {
		t.Fatalf("expected one internal gap, got %+v", cov.MissingRanges)
	}
}

func TestComputePulseCoverageWaitingForVOD(t *testing.T) {
	streamStart := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	rollups := []heatmap.MinuteRollup{
		{MinuteTS: streamStart.Add(120 * time.Minute), ChatCount: 3},
	}
	cov := computePulseCoverage(rollups, streamStart, 7200, true, "", false, false)
	if cov.State != CoverageStateWaitingForVOD {
		t.Fatalf("state = %q, want waiting_for_vod", cov.State)
	}
	if cov.CanBackfill {
		t.Fatal("expected canBackfill=false while live without VOD")
	}
}

func TestRangeFullyCovered(t *testing.T) {
	offsets := []int{0, 60, 120, 180}
	if !rangeFullyCovered(offsets, 0, 180) {
		t.Fatal("expected range covered")
	}
	if rangeFullyCovered(offsets, 0, 240) {
		t.Fatal("expected range not covered through 240")
	}
}

func TestDetectMissingRangesPrefixOnly(t *testing.T) {
	offsets := []int{7200, 7260}
	ranges := detectMissingRanges(offsets, 7200)
	if len(ranges) != 1 || ranges[0].FromOffsetSeconds != 0 {
		t.Fatalf("ranges = %+v", ranges)
	}
}
