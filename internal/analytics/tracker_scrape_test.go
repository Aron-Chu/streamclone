package analytics

import (
	"testing"
	"time"
)

func TestDirectHTTPTelemetryAutoDisable(t *testing.T) {
	var tel directHTTPTelemetry
	for i := 0; i < directHTTPWindowSize; i++ {
		tel.record(false)
	}
	if tel.allowed() {
		t.Fatal("expected direct HTTP disabled after low success rate")
	}
}

func TestTrackerScrapeMaxAgeMS(t *testing.T) {
	ended := time.Now().Add(-72 * time.Hour)
	svc := &SyncService{passTTMaxAge: true}
	got := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &ended}, false)
	if got != ttMaxAgeStaleMS {
		t.Fatalf("stale stream maxAge=%d want %d", got, ttMaxAgeStaleMS)
	}

	if got := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &ended}, true); got != 0 {
		t.Fatalf("viewers-only maxAge=%d want 0", got)
	}

	svc.ttMaxAgeMSDefault = 12345
	if got := svc.trackerScrapeMaxAgeMS(nil, false); got != 12345 {
		t.Fatalf("override maxAge=%d want 12345", got)
	}
}

func TestParseTrackerDurationMinutesFromHTML(t *testing.T) {
	html := `<div class="g-x-s-value">4<small>h</small>39<small>m</small></div><div class="g-x-s-label color-streamed">Stream duration</div>`
	if got := parseTrackerDurationMinutesFromHTML(html); got != 279 {
		t.Fatalf("duration minutes=%d want 279", got)
	}
}

func TestHasGoodViewerCoverageFromRollups(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	rollups := make([]MinuteRollup, 20)
	for i := range rollups {
		val := 100 + i*10
		if i >= 15 {
			val = 250
		}
		rollups[i] = MinuteRollup{
			MinuteTS:      now.Add(time.Duration(i) * time.Minute),
			ViewerAvg:     val,
			ViewerSamples: 1,
		}
	}
	stream := &StreamRecord{
		ViewerSamples: 20,
		StartedAt:     now,
		EndedAt:       ptrTime(now.Add(20 * time.Minute)),
	}
	if !hasGoodViewerCoverageFromRollups(rollups, stream) {
		t.Fatal("expected good coverage for varied viewer rollups")
	}

	flat := make([]MinuteRollup, len(rollups))
	for i := range flat {
		flat[i] = MinuteRollup{
			MinuteTS:      now.Add(time.Duration(i) * time.Minute),
			ViewerAvg:     500,
			ViewerSamples: 1,
		}
	}
	if hasGoodViewerCoverageFromRollups(flat, stream) {
		t.Fatal("expected flat viewer line to fail coverage check")
	}
}

func ptrTime(ts time.Time) *time.Time {
	return &ts
}
