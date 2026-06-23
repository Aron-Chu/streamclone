package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/analytics/heatmap"
)

func TestExtensionHealth(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body ExtensionHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK {
		t.Fatal("expected ok=true")
	}
	if body.Version == "" {
		t.Fatal("expected version")
	}
	if body.Time <= 0 {
		t.Fatal("expected time ms")
	}
}

func TestExtensionPulseInvalidLogin(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/INVALID!!", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNormalizeLaneSeries(t *testing.T) {
	got := normalizeLaneSeries([]float64{0, 5, 10, 2.5})
	want := []int{0, 50, 100, 25}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDominantSignalFromReason(t *testing.T) {
	if dominantSignalFromReason("chat_spike") != "chat" {
		t.Fatal("expected chat")
	}
	if dominantSignalFromReason("seventv_spike") != "seventv" {
		t.Fatal("expected seventv")
	}
}

func TestExtensionPeaksFullStreamVsRecentWindow(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	const totalMinutes = 240
	rollups, points := syntheticExtensionHeatmap(streamStart, totalMinutes, 30, 95)

	fullPeaks := buildExtensionPeaks(rollups, points, false, "historical", streamStart)
	if len(fullPeaks) == 0 {
		t.Fatal("expected full-stream peaks")
	}
	foundEarly := false
	for _, p := range fullPeaks {
		if p.OffsetSeconds == 30*60 {
			foundEarly = true
			break
		}
	}
	if !foundEarly {
		t.Fatalf("expected peak at 30m offset in full stream, got %+v", fullPeaks)
	}

	windowRollups, windowPoints := trimExtensionRecentWindow(rollups, points, false)
	if len(windowRollups) > extPulseMaxRollups {
		t.Fatalf("recent window len = %d, want <= %d", len(windowRollups), extPulseMaxRollups)
	}
	recentPeaks := buildExtensionPeaks(windowRollups, windowPoints, false, "historical", streamStart)
	for _, p := range recentPeaks {
		if p.OffsetSeconds == 30*60 {
			t.Fatalf("recent-only peaks must not include 30m spike, got %+v", recentPeaks)
		}
	}

	extRollups, _ := buildExtensionRollupsAndLanes(windowRollups, windowPoints, streamStart)
	if len(extRollups) > extPulseMaxRollups {
		t.Fatalf("ext rollups len = %d, want <= %d", len(extRollups), extPulseMaxRollups)
	}
}

func TestExtensionPeaksWarmingUnderMinimum(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmap(streamStart, 3, -1, 80)
	peaks := buildExtensionPeaks(rollups, points, false, "historical", streamStart)
	if len(peaks) != 0 {
		t.Fatalf("expected no peaks with < %d completed rollups, got %d", extPulseMinCompleted, len(peaks))
	}
}

func TestExtensionReasonLabel(t *testing.T) {
	cases := map[string]string{
		heatmap.ReasonChatSpike:        "Chat spike",
		heatmap.ReasonSevenTVSpike:     "7TV emote spike",
		heatmap.ReasonTwitchEmoteSpike: "Twitch emote spike",
		heatmap.ReasonFFZSpike:         "FFZ emote spike",
		heatmap.ReasonViewerSpike:      "Viewer spike",
		heatmap.ReasonManual:           "Moment",
		"emote_spike":                  "Emote spike",
	}
	for reason, want := range cases {
		if got := extensionReasonLabel(reason); got != want {
			t.Fatalf("extensionReasonLabel(%q) = %q, want %q", reason, got, want)
		}
	}
	if got := extensionReasonLabel("game_change"); got != "game change" {
		t.Fatalf("fallback label = %q, want %q", got, "game change")
	}
}

func TestRollupAtOffset(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmap(streamStart, 5, 2, 90)
	got, ok := rollupAtOffset(rollups, points, 2*60)
	if !ok {
		t.Fatal("expected rollup at 2m offset")
	}
	if got.ChatCount != 5 {
		t.Fatalf("chatCount = %d, want 5", got.ChatCount)
	}
	if _, ok := rollupAtOffset(rollups, points, 999*60); ok {
		t.Fatal("expected no rollup for unknown offset")
	}
}

func TestExtensionPeaksEnrichedFields(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmapWithEmotes(streamStart, 10, 5, 88)
	peaks := buildExtensionPeaks(rollups, points, false, "historical", streamStart)
	if len(peaks) == 0 {
		t.Fatal("expected peaks")
	}

	var spike *ExtensionPeak
	for i := range peaks {
		if peaks[i].OffsetSeconds == 5*60 {
			spike = &peaks[i]
			break
		}
	}
	if spike == nil {
		t.Fatalf("expected peak at 5m, got %+v", peaks)
	}
	if spike.ReasonLabel != "Chat spike" {
		t.Fatalf("reasonLabel = %q, want %q", spike.ReasonLabel, "Chat spike")
	}
	if spike.ChatCount != 42 {
		t.Fatalf("chatCount = %d, want 42", spike.ChatCount)
	}
	if spike.EmoteCount != 15 {
		t.Fatalf("emoteCount = %d, want 15", spike.EmoteCount)
	}
	if len(spike.TopEmotes) != 1 {
		t.Fatalf("topEmotes len = %d, want 1", len(spike.TopEmotes))
	}
	emote := spike.TopEmotes[0]
	if emote.Name != "OMEGALUL" || emote.Count != 12 || emote.Provider != "seventv" {
		t.Fatalf("topEmote = %+v, want OMEGALUL/seventv/12", emote)
	}
	if emote.ImageURL != "https://cdn.example/emote.png" {
		t.Fatalf("imageUrl = %q", emote.ImageURL)
	}
}

func TestExtensionRollupStructuredEmotes(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmapWithEmotes(streamStart, 8, 3, 75)
	extRollups, _ := buildExtensionRollupsAndLanes(rollups, points, streamStart)
	if len(extRollups) == 0 {
		t.Fatal("expected rollups")
	}

	var spikeRollup *ExtensionRollup
	for i := range extRollups {
		if extRollups[i].OffsetSeconds == 3*60 {
			spikeRollup = &extRollups[i]
			break
		}
	}
	if spikeRollup == nil {
		t.Fatalf("expected rollup at 3m, got %+v", extRollups)
	}
	if len(spikeRollup.TopEmotes) != 1 {
		t.Fatalf("topEmotes len = %d, want 1", len(spikeRollup.TopEmotes))
	}
	if spikeRollup.TopEmotes[0].Name != "OMEGALUL" {
		t.Fatalf("topEmote name = %q", spikeRollup.TopEmotes[0].Name)
	}
	if spikeRollup.TotalEmoteCount != 15 {
		t.Fatalf("totalEmoteCount = %d, want 15", spikeRollup.TotalEmoteCount)
	}
}

func TestExtensionTopEmotesFromHeatmapRollups(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	heatmapRollups := []heatmap.MinuteRollup{
		{
			MinuteTS:        streamStart,
			Emotes:          map[string]int{"seventv:e1:KEKW": 10},
			TotalEmoteCount: 10,
		},
		{
			MinuteTS:        streamStart.Add(time.Minute),
			Emotes:          map[string]int{"seventv:e1:KEKW": 5, "twitch:t1:PepeHands": 3},
			TotalEmoteCount: 8,
		},
	}
	top := TopEmotesFromRollups(storeRollupsFromHeatmap(heatmapRollups), 3)
	ext := convertTopEmotesToExtension(top)
	if len(ext) != 2 {
		t.Fatalf("topEmotes len = %d, want 2", len(ext))
	}
	if ext[0].Name != "KEKW" || ext[0].Count != 15 {
		t.Fatalf("top emote[0] = %+v, want KEKW/15", ext[0])
	}
	if ext[1].Name != "PepeHands" || ext[1].Count != 3 {
		t.Fatalf("top emote[1] = %+v, want PepeHands/3", ext[1])
	}
}

func TestRewriteExtensionEmoteURLsUsesSevenTVCDN(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	in := []ExtensionEmote{{
		ID:       localID,
		Name:     "KEKW",
		Provider: "seventv",
		ImageURL: "/emotes/" + localID + "/1x.webp",
		Count:    12,
	}}
	out := rewriteExtensionEmoteURLs(in, map[string]string{localID: providerID})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	want := "https://cdn.7tv.app/emote/" + providerID + "/4x.webp"
	if out[0].ImageURL != want {
		t.Fatalf("imageUrl = %q, want %q", out[0].ImageURL, want)
	}
}

func TestTrimExtensionFullWindowKeepsRecentTail(t *testing.T) {
	const n = 500
	rollups := make([]heatmap.MinuteRollup, n)
	points := make([]heatmap.ReplayHeatmapDetailPoint, n)
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for i := range rollups {
		rollups[i] = heatmap.MinuteRollup{
			MinuteTS:  streamStart.Add(time.Duration(i) * time.Minute),
			ChatCount: i + 1,
		}
		points[i] = heatmap.ReplayHeatmapDetailPoint{
			ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: i * 60, Score: i + 1},
		}
	}
	trimmedRollups, trimmedPoints := trimExtensionFullWindow(rollups, points, 480, false)
	if len(trimmedRollups) != 480 || len(trimmedPoints) != 480 {
		t.Fatalf("trimmed len = %d/%d, want 480/480", len(trimmedRollups), len(trimmedPoints))
	}
	if trimmedRollups[0].ChatCount != 21 {
		t.Fatalf("first chatCount = %d, want 21", trimmedRollups[0].ChatCount)
	}
	if trimmedRollups[len(trimmedRollups)-1].ChatCount != 500 {
		t.Fatalf("last chatCount = %d, want 500", trimmedRollups[len(trimmedRollups)-1].ChatCount)
	}
}

func TestCollectExtensionSevenTVLocalIDs(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	payload := &ExtensionPulseResponse{
		TopEmotes: []ExtensionEmote{{
			ID:       localID,
			Provider: "seventv",
		}},
		Rollups: []ExtensionRollup{{
			TopEmotes: []ExtensionEmote{{
				ID:       localID,
				Provider: "7tv",
			}},
		}},
	}
	ids := collectExtensionSevenTVLocalIDs(payload)
	if len(ids) != 1 || ids[0] != localID {
		t.Fatalf("ids = %+v, want [%q]", ids, localID)
	}
}

func TestExtensionPeaksIndexAlignment(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, points := syntheticExtensionHeatmapWithEmotes(streamStart, 10, 5, 88)
	// Deliberately mismatch offset so offset-only lookup would miss a different index.
	points[5].OffsetSeconds = 99999

	peaks := buildExtensionPeaks(rollups, points, false, "historical", streamStart)
	var spike *ExtensionPeak
	for i := range peaks {
		if peaks[i].Score == 88 {
			spike = &peaks[i]
			break
		}
	}
	if spike == nil {
		t.Fatalf("expected scored peak, got %+v", peaks)
	}
	if spike.ChatCount != 42 {
		t.Fatalf("chatCount = %d, want 42 (index alignment)", spike.ChatCount)
	}
	if spike.EmoteCount != 15 {
		t.Fatalf("emoteCount = %d, want 15 (index alignment)", spike.EmoteCount)
	}
}

func TestExtensionRollupTotalEmoteCountFallback(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups := []heatmap.MinuteRollup{
		{
			MinuteTS:  streamStart,
			ChatCount: 3,
			Emotes:    map[string]int{"twitch:t1:LUL": 4, "seventv:e1:KEKW": 2},
		},
	}
	points := []heatmap.ReplayHeatmapDetailPoint{
		{
			ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{
				OffsetSeconds: 0,
				Score:         10,
				MinuteTs:      streamStart,
			},
		},
	}
	extRollups, _ := buildExtensionRollupsAndLanes(rollups, points, streamStart)
	if len(extRollups) != 1 {
		t.Fatalf("rollup len = %d, want 1", len(extRollups))
	}
	if extRollups[0].TotalEmoteCount != 6 {
		t.Fatalf("totalEmoteCount = %d, want 6 (sum of emotes map)", extRollups[0].TotalEmoteCount)
	}
}

func TestTrimExtensionRecentWindowDecimatedPointsMismatch(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups, alignedPoints := syntheticExtensionHeatmap(streamStart, 80, 40, 90)
	// Simulate decimated detail points (shorter than rollups) — old bug emptied rollups.
	decimated := make([]heatmap.ReplayHeatmapDetailPoint, 0, len(alignedPoints)/2)
	for i := 0; i < len(alignedPoints); i += 2 {
		decimated = append(decimated, alignedPoints[i])
	}
	if len(decimated) >= len(rollups) {
		t.Fatal("test setup: decimated slice must be shorter than rollups")
	}

	_, badPoints := trimExtensionRecentWindow(rollups, decimated, true)
	extBad, _ := buildExtensionRollupsAndLanes(rollups[len(rollups)-extPulseMaxRollups:], badPoints, streamStart)

	windowRollups, windowPoints := trimExtensionRecentWindow(rollups, alignedPoints, true)
	extRollups, _ := buildExtensionRollupsAndLanes(windowRollups, windowPoints, streamStart)

	if len(extRollups) == 0 {
		t.Fatalf("aligned trim produced no rollups, want recent window")
	}
	if len(extBad) == 0 && len(badPoints) == 0 {
		// Document regression: mismatched slices can drop all payload rollups.
		t.Log("decimated mismatch can zero extension rollups (regression case)")
	}
}

func TestCoverageStartOffsetSeconds(t *testing.T) {
	streamStart := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	rollups := []heatmap.MinuteRollup{
		{MinuteTS: streamStart.Add(102 * time.Minute), ChatCount: 12},
		{MinuteTS: streamStart.Add(103 * time.Minute), ChatCount: 8},
	}
	got := coverageStartOffsetSeconds(rollups, streamStart)
	want := 102 * 60
	if got != want {
		t.Fatalf("coverageStartOffsetSeconds = %d, want %d", got, want)
	}

	fromStart := []heatmap.MinuteRollup{
		{MinuteTS: streamStart, ChatCount: 5},
	}
	if coverageStartOffsetSeconds(fromStart, streamStart) != 0 {
		t.Fatal("expected 0 when tracking from stream start")
	}
}

func syntheticExtensionHeatmap(
	streamStart time.Time,
	minutes int,
	spikeMinute int,
	spikeScore int,
) ([]heatmap.MinuteRollup, []heatmap.ReplayHeatmapDetailPoint) {
	rollups := make([]heatmap.MinuteRollup, 0, minutes)
	points := make([]heatmap.ReplayHeatmapDetailPoint, 0, minutes)
	for i := 0; i < minutes; i++ {
		ts := streamStart.Add(time.Duration(i) * time.Minute)
		score := 10
		reason := heatmap.ReasonManual
		if i == spikeMinute {
			score = spikeScore
			reason = heatmap.ReasonChatSpike
		}
		rollups = append(rollups, heatmap.MinuteRollup{
			MinuteTS:  ts,
			ChatCount: 5,
		})
		points = append(points, heatmap.ReplayHeatmapDetailPoint{
			ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{
				OffsetSeconds: i * 60,
				Score:         score,
				Reason:        reason,
				MinuteTs:      ts,
			},
		})
	}
	return rollups, points
}

func syntheticExtensionHeatmapWithEmotes(
	streamStart time.Time,
	minutes int,
	spikeMinute int,
	spikeScore int,
) ([]heatmap.MinuteRollup, []heatmap.ReplayHeatmapDetailPoint) {
	rollups, points := syntheticExtensionHeatmap(streamStart, minutes, spikeMinute, spikeScore)
	if spikeMinute < 0 || spikeMinute >= len(rollups) {
		return rollups, points
	}
	rollups[spikeMinute].ChatCount = 42
	rollups[spikeMinute].TotalEmoteCount = 15
	rollups[spikeMinute].SevenTVEmoteCount = 15
	points[spikeMinute].TopEmotes = []heatmap.HeatmapEmote{
		{
			ID:       "emote-1",
			Name:     "OMEGALUL",
			ImageURL: "https://cdn.example/emote.png",
			Count:    12,
			Provider: "seventv",
		},
	}
	return rollups, points
}
