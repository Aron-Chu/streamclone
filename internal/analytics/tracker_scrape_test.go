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
		t.Fatalf("48h-7d stream maxAge=%d want %d", got, ttMaxAgeStaleMS)
	}

	endedVery := time.Now().Add(-8 * 24 * time.Hour)
	gotVery := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &endedVery}, false)
	if gotVery != ttMaxAgeVeryStaleMS {
		t.Fatalf("archived stream maxAge=%d want %d", gotVery, ttMaxAgeVeryStaleMS)
	}

	endedMid := time.Now().Add(-50 * time.Hour)
	gotMid := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &endedMid}, false)
	if gotMid != ttMaxAgeStaleMS {
		t.Fatalf("mid-stale stream maxAge=%d want %d", gotMid, ttMaxAgeStaleMS)
	}

	if got := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &ended}, true); got != 0 {
		t.Fatalf("viewers-only maxAge=%d want 0", got)
	}

	svc.ttMaxAgeMSDefault = 12345
	if got := svc.trackerScrapeMaxAgeMS(nil, false); got != 12345 {
		t.Fatalf("override maxAge=%d want 12345", got)
	}

	svc.ttMaxAgeMSDefault = 0
	svc.ttStaleMaxAgeMS = 99999
	endedCustom := time.Now().Add(-8 * 24 * time.Hour)
	if got := svc.trackerScrapeMaxAgeMS(&StreamRecord{EndedAt: &endedCustom}, false); got != 99999 {
		t.Fatalf("custom stale maxAge=%d want 99999", got)
	}
}

func TestShouldTryDirectHTTP(t *testing.T) {
	recent := time.Now().Add(-2 * time.Hour)
	stale := time.Now().Add(-12 * time.Hour)

	svc := &SyncService{ttDirectHTTPEnabled: true, ttDirectHTTPStaleOnly: true}
	if svc.shouldTryDirectHTTP(&StreamRecord{EndedAt: &recent}) {
		t.Fatal("expected recent stream to skip direct HTTP when stale-only")
	}
	if !svc.shouldTryDirectHTTP(&StreamRecord{EndedAt: &stale}) {
		t.Fatal("expected stale stream to allow direct HTTP when stale-only")
	}
	if svc.shouldTryDirectHTTP(nil) {
		t.Fatal("expected nil stream to skip direct HTTP when stale-only")
	}

	svc.ttDirectHTTPStaleOnly = false
	if !svc.shouldTryDirectHTTP(&StreamRecord{EndedAt: &recent}) {
		t.Fatal("expected direct HTTP for recent stream when stale-only disabled")
	}
}

func TestParseTrackerDurationMinutesFromHTML(t *testing.T) {
	html := `<div class="g-x-s-value">4<small>h</small>39<small>m</small></div><div class="g-x-s-label color-streamed">Stream duration</div>`
	if got := parseTrackerDurationMinutesFromHTML(html); got != 279 {
		t.Fatalf("duration minutes=%d want 279", got)
	}
}

func TestShouldSkipTrackerPrefersLiveCollector(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Minute)
	ended := now.Add(20 * time.Minute)
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
		EndedAt:       &ended,
		LastSeenAt:    now.Add(5 * time.Minute),
	}
	if !hasLiveCollectorViewerCoverage(stream, rollups) {
		t.Fatal("expected live collector coverage for recent varied rollups")
	}

	staleSeen := now.Add(-72 * time.Hour)
	staleStream := &StreamRecord{
		ViewerSamples: 20,
		StartedAt:     now.Add(-73 * time.Hour),
		EndedAt:       &staleSeen,
		LastSeenAt:    staleSeen,
	}
	if hasLiveCollectorViewerCoverage(staleStream, rollups) {
		t.Fatal("expected stale stream to skip live collector preference")
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
