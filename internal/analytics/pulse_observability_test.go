package analytics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/metrics"
)

func TestNoteGoLiveDuplicateDoesNotDoubleCountGoLive(t *testing.T) {
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, time.Hour, 200)
	before := testutil.ToFloat64(metrics.PulseGoLiveDetectedTotal.WithLabelValues("always_track"))
	dupBefore := testutil.ToFloat64(metrics.PulseGoLiveDuplicateObservationTotal)
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, false, time.Time{})
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, true, time.Time{})
	after := testutil.ToFloat64(metrics.PulseGoLiveDetectedTotal.WithLabelValues("always_track"))
	if after-before != 1 {
		t.Fatalf("expected single go-live increment, got delta %v", after-before)
	}
	if testutil.ToFloat64(metrics.PulseGoLiveDuplicateObservationTotal)-dupBefore != 1 {
		t.Fatal("expected duplicate observation counter increment")
	}
}

func TestFirstRollupAfterGoLiveRecordsTiming(t *testing.T) {
	streamStart := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	detectedAt := streamStart.Add(20 * time.Second)
	now := detectedAt.Add(30 * time.Second)
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, time.Hour, 200)
	c.WithNowClock(func() time.Time { return now })
	trackedBefore := testutil.ToFloat64(metrics.PulseTrackedFromStartTotal)
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, false, streamStart)
	c.recordFirstRollupMetrics("stream-1", streamStart.Add(30*time.Second))
	c.recordFirstRollupMetrics("stream-1", streamStart.Add(60*time.Second))
	if testutil.ToFloat64(metrics.PulseTrackedFromStartTotal)-trackedBefore != 1 {
		t.Fatal("expected tracked_from_start counter increment once")
	}
}

func TestFirstRollupLateCapStartUsesStreamStartOffset(t *testing.T) {
	streamStart := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	detectedAt := streamStart.Add(20 * time.Second)
	now := detectedAt.Add(3 * time.Minute)
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, time.Hour, 200)
	c.WithNowClock(func() time.Time { return now })
	lateBefore := testutil.ToFloat64(metrics.PulseLateCapStartTotal.WithLabelValues("top_roster"))
	c.NoteGoLiveDetected("stream-late", "chan1", "top_roster", TrackPriorityTopRoster, false, streamStart)
	c.recordFirstRollupMetrics("stream-late", streamStart.Add(3*time.Minute))
	if testutil.ToFloat64(metrics.PulseLateCapStartTotal.WithLabelValues("top_roster"))-lateBefore != 1 {
		t.Fatal("expected late cap start counter increment for top_roster")
	}
}

func TestCoverageStartOffsetSecondsForRollupPrefersStreamStart(t *testing.T) {
	streamStart := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	obs := pulseGoLiveObservation{
		detectedAt:      streamStart.Add(90 * time.Second),
		streamStartedAt: streamStart,
	}
	got := coverageStartOffsetSecondsForRollup(obs, streamStart.Add(2*time.Minute))
	if got != 120 {
		t.Fatalf("coverageStartOffsetSecondsForRollup = %d, want 120 from stream start", got)
	}
}

func TestNormalizeGoLiveSourceClass(t *testing.T) {
	if normalizeGoLiveSourceClass("global_protected") != "always_track" {
		t.Fatal("expected always_track class")
	}
	if normalizeGoLiveSourceClass("top_roster") != "top_roster" {
		t.Fatal("expected top_roster class")
	}
}
