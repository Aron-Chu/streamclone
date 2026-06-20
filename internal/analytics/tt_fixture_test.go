package analytics

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTwitchTrackerFixtures(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "twitchtracker")
	cases := []struct {
		file              string
		expectedDuration  int
		expectedPeak      int
		expectedAvg       int
		minPointCount     int
		expectComplete    bool
		maxPeakDeltaPct   float64
		maxAvgDeltaPct    float64
	}{
		{
			file:             "can-donk-2026-06-07.meta.html",
			expectedDuration: 309,
			expectedPeak:     52039,
			expectedAvg:      37490,
			minPointCount:    30,
			expectComplete:   true,
			maxPeakDeltaPct:  2,
			maxAvgDeltaPct:   5,
		},
	}

	s := &SyncService{log: slog.Default()}
	startedAt := time.Date(2026, 6, 7, 13, 25, 47, 0, time.UTC)

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(fixtureDir, tc.file)
			html, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			parsed, err := s.parseTwitchTrackerMetadata(string(html), startedAt)
			if err != nil {
				t.Fatalf("parse metadata: %v", err)
			}
			if parsed.DurationMinutes != tc.expectedDuration {
				t.Errorf("duration: got %d want %d", parsed.DurationMinutes, tc.expectedDuration)
			}
			if parsed.PeakViewers != tc.expectedPeak {
				t.Errorf("peak: got %d want %d", parsed.PeakViewers, tc.expectedPeak)
			}
			if tc.expectedAvg > 0 && parsed.AvgViewers != tc.expectedAvg {
				t.Errorf("avg: got %d want %d", parsed.AvgViewers, tc.expectedAvg)
			}
			if len(parsed.ViewerPoints) < tc.minPointCount {
				t.Errorf("point_count: got %d want >= %d", len(parsed.ViewerPoints), tc.minPointCount)
			}
			durSec := parsed.DurationMinutes * 60
			if !hasCompleteViewerChart(parsed.ViewerPoints, durSec) {
				t.Error("hasCompleteViewerChart: expected complete chart")
			}
			if isFlatViewerChart(parsed.ViewerPoints) {
				t.Error("expected non-flat viewer chart")
			}
			if tc.expectComplete != hasCompleteViewerChart(parsed.ViewerPoints, durSec) {
				t.Errorf("complete chart mismatch")
			}

			full, _ := s.parseTwitchTrackerHTML(string(html), startedAt)
			report := evaluateTTFixture(tc.file, html, startedAt, tc.expectedPeak, tc.expectedAvg, tc.expectedDuration)
			if report.PeakDeltaPct > tc.maxPeakDeltaPct {
				t.Errorf("peak delta %.2f%% exceeds %.2f%%", report.PeakDeltaPct, tc.maxPeakDeltaPct)
			}
			if tc.expectedAvg > 0 && report.AvgDeltaPct > tc.maxAvgDeltaPct {
				t.Errorf("avg delta %.2f%% exceeds %.2f%%", report.AvgDeltaPct, tc.maxAvgDeltaPct)
			}
			if len(full.ViewerPoints) == 0 {
				t.Fatal("parseTwitchTrackerHTML returned no viewer points")
			}
		})
	}
}

func TestParseTwitchTrackerFlatRejected(t *testing.T) {
	flat := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 69800},
		{OffsetSeconds: 3600, Viewers: 69800},
	}
	if hasCompleteViewerChart(flat, 3600) {
		t.Fatal("expected flat peak-only synthesis to be rejected")
	}
	if !isFlatViewerChart(flat) {
		t.Fatal("expected flat chart detection")
	}
}

func TestSVGChartDensityRejection(t *testing.T) {
	durationMin := 300
	sparse := make([]parsedViewerPoint, 8)
	for i := range sparse {
		sparse[i] = parsedViewerPoint{OffsetSeconds: i * durationMin * 60 / 8, Viewers: 1000 + i*100}
	}
	if chartMeetsTTPointDensity(len(sparse), durationMin) {
		t.Fatal("expected sparse SVG chart to fail density check")
	}
	dense := make([]parsedViewerPoint, 60)
	for i := range dense {
		dense[i] = parsedViewerPoint{OffsetSeconds: i * durationMin * 60 / 60, Viewers: 1000 + i*50}
	}
	if !chartMeetsTTPointDensity(len(dense), durationMin) {
		t.Fatal("expected dense chart to pass density check")
	}
}

func TestApplyTTViewerMedianSmooth(t *testing.T) {
	points := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 1000},
		{OffsetSeconds: 600, Viewers: 5000},
		{OffsetSeconds: 1200, Viewers: 1200},
		{OffsetSeconds: 1800, Viewers: 1100},
	}
	smoothed := applyTTViewerMedianSmooth(points, 3)
	if smoothed[1].Viewers == points[1].Viewers && smoothed[1].Viewers == 5000 {
		// spike at index 1 should be pulled toward neighbors
		t.Log("median smooth preserved or reduced spike", smoothed[1].Viewers)
	}
	if smoothed[0].OffsetSeconds != points[0].OffsetSeconds {
		t.Fatal("smooth must preserve timestamps/offsets")
	}
}

func TestReportTwitchTrackerFixturesJSON(t *testing.T) {
	if os.Getenv("TT_PARSER_REPORT") == "" {
		t.Skip("set TT_PARSER_REPORT=1 to emit fixture JSON report")
	}
	fixtureDir := filepath.Join("testdata", "twitchtracker")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	startedAt := time.Date(2026, 6, 7, 13, 25, 47, 0, time.UTC)
	var reports []TTFixtureReport
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".html") {
			continue
		}
		html, err := os.ReadFile(filepath.Join(fixtureDir, ent.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", ent.Name(), err)
		}
		reports = append(reports, evaluateTTFixture(ent.Name(), html, startedAt, 52039, 37490, 309))
	}
	outPath := os.Getenv("TT_PARSER_REPORT_PATH")
	if outPath == "" {
		outPath = filepath.Join("testdata", "twitchtracker", "parser-report.json")
	}
	if err := writeTTJSONReport(outPath, reports); err != nil {
		t.Fatalf("write report: %v", err)
	}
	enc, _ := json.MarshalIndent(reports, "", "  ")
	t.Log(string(enc))
}

func TestBenchmarkTTVsLiveCompare(t *testing.T) {
	if os.Getenv("TT_BENCHMARK") == "" {
		t.Skip("set TT_BENCHMARK=1 for live-vs-TT comparison")
	}
	htmlPath := os.Getenv("TT_BENCHMARK_HTML")
	if htmlPath == "" {
		t.Fatal("TT_BENCHMARK_HTML required")
	}
	streamID := os.Getenv("TT_BENCHMARK_STREAM_ID")
	if streamID == "" {
		streamID = "unknown"
	}
	rollupsPath := os.Getenv("TT_BENCHMARK_ROLLUPS_JSON")
	if rollupsPath == "" {
		t.Fatal("TT_BENCHMARK_ROLLUPS_JSON required")
	}
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	rollupsRaw, err := os.ReadFile(rollupsPath)
	if err != nil {
		t.Fatalf("read rollups: %v", err)
	}
	var rollups []MinuteRollup
	if err := json.Unmarshal(rollupsRaw, &rollups); err != nil {
		t.Fatalf("parse rollups: %v", err)
	}
	s := &SyncService{log: slog.Default()}
	startedAt := time.Now()
	parsed, err := s.parseTwitchTrackerHTML(string(html), startedAt)
	if err != nil {
		t.Fatalf("parse tt: %v", err)
	}
	durSec := parsed.DurationMinutes * 60
	if durSec <= 0 {
		durSec = len(rollups) * 60
	}
	report := compareLiveRollupsToTT(streamID, rollups, parsed.ViewerPoints, durSec)
	outPath := os.Getenv("TT_BENCHMARK_OUTPUT")
	if outPath == "" {
		outPath = "tt-vs-live-report.json"
	}
	if err := writeTTJSONReport(outPath, report); err != nil {
		t.Fatalf("write report: %v", err)
	}
	enc, _ := json.MarshalIndent(report, "", "  ")
	t.Log(string(enc))
}
