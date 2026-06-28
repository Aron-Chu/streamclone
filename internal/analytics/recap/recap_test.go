package recap

import (
	"testing"

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
