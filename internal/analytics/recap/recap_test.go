package recap

import (
	"testing"
	"time"

	"streamclone/internal/analytics/heatmap"
)

func TestBuildAggregatesRecap(t *testing.T) {
	recap := Build(Input{
		StreamID: "stream-1",
		Login:    "XQC",
		Rollups: []heatmap.MinuteRollup{
			{ChatCount: 10, SevenTVEmoteCount: 2, Emotes: map[string]int{"seventv:1:KEKW": 2}},
			{ChatCount: 40, SevenTVEmoteCount: 9, Emotes: map[string]int{"seventv:1:KEKW": 3, "seventv:2:OMEGALUL": 6}},
			{ChatCount: 5, TotalEmoteCount: 100, SevenTVEmoteCount: 0, Emotes: map[string]int{"twitch:3:Kappa": 100}},
		},
		Points: []heatmap.ReplayHeatmapDetailPoint{
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 120, Score: 30, Reason: heatmap.ReasonChatSpike}},
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 60, Score: 95, Reason: heatmap.ReasonSevenTVSpike, TopEmotes: []heatmap.HeatmapEmote{{Name: "OMEGALUL", Count: 6, Provider: "seventv"}}}},
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 0, Score: 80, Reason: heatmap.ReasonChatSpike}},
		},
	})

	if recap.Login != "xqc" {
		t.Fatalf("login = %q, want xqc", recap.Login)
	}
	if recap.TotalMessages != 55 {
		t.Fatalf("total messages = %d, want 55", recap.TotalMessages)
	}
	if recap.PeakChatPerMin != 40 || recap.BiggestChatSpike == nil || recap.BiggestChatSpike.OffsetSeconds != 60 {
		t.Fatalf("bad biggest chat spike: %+v peak=%d", recap.BiggestChatSpike, recap.PeakChatPerMin)
	}
	if recap.FunniestEmoteBurst == nil || recap.FunniestEmoteBurst.Code != "OMEGALUL" || recap.FunniestEmoteBurst.Count != 6 {
		t.Fatalf("bad funniest burst: %+v", recap.FunniestEmoteBurst)
	}
	if len(recap.TopEmotes) != 2 || recap.TopEmotes[0].Code != "OMEGALUL" || recap.TopEmotes[0].Count != 6 {
		t.Fatalf("bad top emotes: %+v", recap.TopEmotes)
	}
	if len(recap.TopMoments) != 3 || recap.TopMoments[0].OffsetSeconds != 60 || recap.TopMoments[1].OffsetSeconds != 0 {
		t.Fatalf("bad top moments: %+v", recap.TopMoments)
	}
	if len(recap.ClipCandidates) != 3 || recap.ClipCandidates[0].Score != 95 {
		t.Fatalf("bad clip candidates: %+v", recap.ClipCandidates)
	}
	if recap.TopMoments[0].ChatCount != 40 || recap.TopMoments[0].EmoteCount != 9 {
		t.Fatalf("top moment metrics = chat:%d emote:%d, want 40/9", recap.TopMoments[0].ChatCount, recap.TopMoments[0].EmoteCount)
	}
	if recap.TopMoments[0].Confidence != 0 {
		t.Fatalf("confidence = %f, want explicit zero for points without confidence", recap.TopMoments[0].Confidence)
	}
}

func TestBuildCarriesConfidenceAndRollupCoverage(t *testing.T) {
	recap := Build(Input{
		StreamID: "stream-coverage",
		Login:    "xqc",
		Rollups: []heatmap.MinuteRollup{
			{ChatCount: 12},
			{Missing: true},
			{Missing: true},
			{ChatCount: 40},
		},
		Points: []heatmap.ReplayHeatmapDetailPoint{
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 180, Score: 95, Confidence: 0.82, Reason: heatmap.ReasonChatSpike}},
		},
	})

	if recap.NonMissingRollupMinutes != 2 {
		t.Fatalf("non-missing rollup minutes = %d, want 2", recap.NonMissingRollupMinutes)
	}
	if len(recap.MissingWindows) != 1 || recap.MissingWindows[0].StartSeconds != 60 || recap.MissingWindows[0].EndSeconds != 179 {
		t.Fatalf("missing windows = %+v, want one 60-179 window", recap.MissingWindows)
	}
	if len(recap.TopMoments) != 1 || recap.TopMoments[0].Confidence != 0.82 {
		t.Fatalf("top moments = %+v, want confidence 0.82", recap.TopMoments)
	}
	if len(recap.ClipCandidates) != 1 || recap.ClipCandidates[0].Confidence != 0.82 {
		t.Fatalf("clip candidates = %+v, want confidence 0.82", recap.ClipCandidates)
	}
}

func TestTopMomentsDeprioritizeViewerOnlyWhenReactionCoverageExists(t *testing.T) {
	startedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	recap := Build(Input{
		StreamID:  "stream-viewer-rank",
		Login:     "xqc",
		StartedAt: startedAt,
		Rollups: []heatmap.MinuteRollup{
			{MinuteTS: startedAt.Add(2692 * time.Second), ChatCount: 445, SevenTVEmoteCount: 352, Emotes: map[string]int{"seventv:1:LO": 352}},
			{MinuteTS: startedAt.Add(2752 * time.Second), ViewerLatest: 18825, ViewerSamples: 1},
			{MinuteTS: startedAt.Add(6840 * time.Second), ChatCount: 680, SevenTVEmoteCount: 692, Emotes: map[string]int{"seventv:1:KEKW": 692}},
		},
		Points: []heatmap.ReplayHeatmapDetailPoint{
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 2760, Score: 54, Reason: heatmap.ReasonViewerSpike}},
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 6840, Score: 42, Reason: heatmap.ReasonSevenTVSpike}},
		},
	})
	if len(recap.TopMoments) != 2 {
		t.Fatalf("top moments = %+v, want 2", recap.TopMoments)
	}
	if recap.TopMoments[0].OffsetSeconds != 6840 {
		t.Fatalf("first moment offset = %d, want 6840 reaction-backed peak ranked first", recap.TopMoments[0].OffsetSeconds)
	}
	if recap.TopMoments[1].OffsetSeconds != 2760 {
		t.Fatalf("second moment offset = %d, want 2760 viewer-only deprioritized", recap.TopMoments[1].OffsetSeconds)
	}
}

func TestTopMomentsKeepsViewerOnlyWhenNoReactionCoverage(t *testing.T) {
	recap := Build(Input{
		StreamID: "stream-viewer-only",
		Login:    "xqc",
		Rollups: []heatmap.MinuteRollup{
			{ViewerLatest: 18825, ViewerSamples: 1},
			{ViewerLatest: 19231, ViewerSamples: 1},
		},
		Points: []heatmap.ReplayHeatmapDetailPoint{
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 2760, Score: 54, Reason: heatmap.ReasonViewerSpike}},
			{ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{OffsetSeconds: 2820, Score: 42, Reason: heatmap.ReasonViewerSpike}},
		},
	})
	if recap.TopMoments[0].OffsetSeconds != 2760 {
		t.Fatalf("first moment = %+v, want highest-score viewer-only when no chat coverage", recap.TopMoments[0])
	}
}

func TestBuildDoesNotFakeMissingSevenTVNames(t *testing.T) {
	recap := Build(Input{
		StreamID: "stream-1",
		Login:    "xqc",
		Rollups: []heatmap.MinuteRollup{
			{ChatCount: 1, SevenTVEmoteCount: 8},
		},
	})
	if len(recap.TopEmotes) != 0 {
		t.Fatalf("top emotes = %+v, want empty without per-emote identity", recap.TopEmotes)
	}
	if recap.FunniestEmoteBurst == nil || recap.FunniestEmoteBurst.Code != "" || recap.FunniestEmoteBurst.Count != 8 {
		t.Fatalf("bad honest burst fallback: %+v", recap.FunniestEmoteBurst)
	}
}
