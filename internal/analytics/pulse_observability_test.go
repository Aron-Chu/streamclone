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
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, false)
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, true)
	after := testutil.ToFloat64(metrics.PulseGoLiveDetectedTotal.WithLabelValues("always_track"))
	if after-before != 1 {
		t.Fatalf("expected single go-live increment, got delta %v", after-before)
	}
	if testutil.ToFloat64(metrics.PulseGoLiveDuplicateObservationTotal)-dupBefore != 1 {
		t.Fatal("expected duplicate observation counter increment")
	}
}

func TestFirstRollupAfterGoLiveRecordsTiming(t *testing.T) {
	detectedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := detectedAt.Add(30 * time.Second)
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, time.Hour, 200)
	c.WithNowClock(func() time.Time { return now })
	trackedBefore := testutil.ToFloat64(metrics.PulseTrackedFromStartTotal)
	c.NoteGoLiveDetected("stream-1", "chan1", "global_protected", 60, false)
	c.recordFirstRollupMetrics("stream-1", detectedAt.Add(30*time.Second))
	c.recordFirstRollupMetrics("stream-1", detectedAt.Add(60*time.Second))
	if testutil.ToFloat64(metrics.PulseTrackedFromStartTotal)-trackedBefore != 1 {
		t.Fatal("expected tracked_from_start counter increment once")
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
