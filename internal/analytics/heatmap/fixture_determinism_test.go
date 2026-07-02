package heatmap

import (
	"testing"
	"time"
)

// fixtureBaseTS is the fixed minute-zero timestamp for the determinism fixture.
// It is a constant so the pinned MinuteTs expectations below never depend on the
// wall clock (Requirement 9.6 / 9.10).
var fixtureBaseTS = time.Date(2026, 6, 11, 18, 0, 0, 0, time.UTC)

// fixtureRollups builds a small but representative, deterministic, offset-sorted
// MinuteRollup slice for the scoring-determinism fixture (Requirement 9.10):
//   - window 0: quiet baseline (low chat/emote/viewer)
//   - window 1: ramping activity with a single 7TV emote
//   - window 2: a large spike (chat/emote/viewer) with a multi-emote map across
//     7TV / Twitch / FFZ providers
//   - window 3: a MISSING window (no rollup data -> forced score 0)
//   - window 4: post-spike cooldown reusing a previously seen emote (low novelty)
//   - window 5: a secondary smaller bump
//
// This exercises log/z-score normalization, provider-spike derivation, top-emote
// dominance/novelty, EWMA smoothing across a gap, non-max suppression, reason
// selection, top-emote ordering, and per-window/stream confidence.
func fixtureRollups() []MinuteRollup {
	return []MinuteRollup{
		{
			MinuteTS:        fixtureBaseTS,
			ViewerAvg:       1200,
			ViewerMax:       1250,
			ViewerLatest:    1200,
			ViewerSamples:   4,
			ChatCount:       40,
			TotalEmoteCount: 8,
			Emotes:          map[string]int{"seventv:e1:KEKW": 5, "twitch:t1:Kappa": 3},
		},
		{
			MinuteTS:          fixtureBaseTS.Add(1 * time.Minute),
			ViewerAvg:         1300,
			ViewerMax:         1400,
			ViewerLatest:      1320,
			ViewerSamples:     4,
			ChatCount:         90,
			TotalEmoteCount:   30,
			SevenTVEmoteCount: 20,
			Emotes:            map[string]int{"seventv:e1:KEKW": 20, "twitch:t1:Kappa": 10},
		},
		{
			MinuteTS:          fixtureBaseTS.Add(2 * time.Minute),
			ViewerAvg:         2600,
			ViewerMax:         2800,
			ViewerLatest:      2700,
			ViewerSamples:     5,
			ChatCount:         520,
			TotalEmoteCount:   410,
			SevenTVEmoteCount: 300,
			Emotes: map[string]int{
				"seventv:e2:OMEGALUL": 300,
				"twitch:t1:Kappa":     70,
				"ffz:f1:Pog":          40,
			},
		},
		{
			MinuteTS: fixtureBaseTS.Add(3 * time.Minute),
			Missing:  true,
		},
		{
			MinuteTS:          fixtureBaseTS.Add(4 * time.Minute),
			ViewerAvg:         1500,
			ViewerMax:         1600,
			ViewerLatest:      1520,
			ViewerSamples:     4,
			ChatCount:         110,
			TotalEmoteCount:   45,
			SevenTVEmoteCount: 30,
			Emotes:            map[string]int{"seventv:e2:OMEGALUL": 30, "twitch:t1:Kappa": 15},
		},
		{
			MinuteTS:          fixtureBaseTS.Add(5 * time.Minute),
			ViewerAvg:         1700,
			ViewerMax:         1800,
			ViewerLatest:      1720,
			ViewerSamples:     4,
			ChatCount:         160,
			TotalEmoteCount:   70,
			SevenTVEmoteCount: 50,
			Emotes:            map[string]int{"seventv:e3:PepeLaugh": 50, "ffz:f1:Pog": 20},
		},
	}
}

// expectedEmote is a pinned top-emote expectation for a fixture point.
type expectedEmote struct {
	id       string
	name     string
	imageURL string
	count    int
	provider string
}

// expectedPoint is a fully pinned ReplayHeatmapPoint expectation for the v1
// scoring fixture.
type expectedPoint struct {
	offsetSeconds   int
	durationSeconds int
	score           int
	confidence      float64
	reason          string
	emotes          []expectedEmote
}

// expectedFixtureV1 encodes the EXACT current ComputeHeatmap output for
// fixtureRollups() under DefaultScoringConfig() (scoring version "v1"). The
// engine is deterministic, so these values were captured from a real run and
// pinned here as a golden fixture.
//
// IMPORTANT: If a code change alters any of these values, the scoring behavior
// has changed. Do NOT simply update these numbers — changing scoring behavior
// for an existing version is unversioned scoring drift and breaks cache keys and
// reproducibility (Requirement 9.10). Instead, bump ScoringConfig.Version (e.g.
// "v1" -> "v2") in DefaultScoringConfig and add a NEW pinned fixture for the new
// version, leaving this v1 fixture intact for historical reproducibility.
var (
	expectedFixtureVersion       = "v1"
	expectedFixtureWindowSeconds = 60
	expectedFixtureConfidence    = 1.0

	expectedFixturePoints = []expectedPoint{
		{
			offsetSeconds: 0, durationSeconds: 60, score: 5, confidence: 1.0, reason: ReasonChatSpike,
			emotes: []expectedEmote{
				{id: "e1", name: "KEKW", imageURL: "https://cdn.7tv.app/emote/e1/4x.webp", count: 5, provider: "seventv"},
				{id: "t1", name: "Kappa", imageURL: "https://static-cdn.jtvnw.net/emoticons/v2/t1/default/dark/2.0", count: 3, provider: "twitch"},
			},
		},
		{
			offsetSeconds: 60, durationSeconds: 60, score: 6, confidence: 1.0, reason: ReasonChatSpike,
			emotes: []expectedEmote{
				{id: "e1", name: "KEKW", imageURL: "https://cdn.7tv.app/emote/e1/4x.webp", count: 20, provider: "seventv"},
				{id: "t1", name: "Kappa", imageURL: "https://static-cdn.jtvnw.net/emoticons/v2/t1/default/dark/2.0", count: 10, provider: "twitch"},
			},
		},
		{
			offsetSeconds: 120, durationSeconds: 60, score: 22, confidence: 1.0, reason: ReasonSevenTVSpike,
			emotes: []expectedEmote{
				{id: "e2", name: "OMEGALUL", imageURL: "https://cdn.7tv.app/emote/e2/4x.webp", count: 300, provider: "seventv"},
				{id: "t1", name: "Kappa", imageURL: "https://static-cdn.jtvnw.net/emoticons/v2/t1/default/dark/2.0", count: 70, provider: "twitch"},
				{id: "f1", name: "Pog", imageURL: "https://cdn.frankerfacez.com/emoticon/f1/4", count: 40, provider: "ffz"},
			},
		},
		{
			offsetSeconds: 240, durationSeconds: 60, score: 9, confidence: 1.0, reason: ReasonChatSpike,
			emotes: []expectedEmote{
				{id: "e2", name: "OMEGALUL", imageURL: "https://cdn.7tv.app/emote/e2/4x.webp", count: 30, provider: "seventv"},
				{id: "t1", name: "Kappa", imageURL: "https://static-cdn.jtvnw.net/emoticons/v2/t1/default/dark/2.0", count: 15, provider: "twitch"},
			},
		},
		{
			offsetSeconds: 300, durationSeconds: 60, score: 14, confidence: 1.0, reason: ReasonSevenTVSpike,
			emotes: []expectedEmote{
				{id: "e3", name: "PepeLaugh", imageURL: "https://cdn.7tv.app/emote/e3/4x.webp", count: 50, provider: "seventv"},
				{id: "f1", name: "Pog", imageURL: "https://cdn.frankerfacez.com/emoticon/f1/4", count: 20, provider: "ffz"},
			},
		},
	}
)

// TestFixtureDeterminismV1 pins ComputeHeatmap output for a known MinuteRollup
// set under DefaultScoringConfig (scoring version "v1") and asserts every point
// against hardcoded EXPECTED values (Requirement 9.10). Because the engine is
// deterministic, any future change that alters scores, reasons, offsets,
// emotes, or confidence for v1 will fail this test — catching unversioned
// scoring drift.
//
// If you intentionally change scoring behavior, you MUST bump the scoring
// Version and pin a new fixture for the new version rather than editing the v1
// expectations here (see expectedFixtureV1 doc comment).
func TestFixtureDeterminismV1(t *testing.T) {
	resp := ComputeHeatmap(fixtureRollups(), DefaultScoringConfig())

	if resp.ScoringVersion != expectedFixtureVersion {
		t.Errorf("ScoringVersion = %q, want %q", resp.ScoringVersion, expectedFixtureVersion)
	}
	if resp.WindowSeconds != expectedFixtureWindowSeconds {
		t.Errorf("WindowSeconds = %d, want %d", resp.WindowSeconds, expectedFixtureWindowSeconds)
	}
	if resp.Confidence != expectedFixtureConfidence {
		t.Errorf("stream Confidence = %.6f, want %.6f", resp.Confidence, expectedFixtureConfidence)
	}

	if len(resp.Points) != len(expectedFixturePoints) {
		t.Fatalf("points length = %d, want %d (the missing window at offset 180 must be omitted)", len(resp.Points), len(expectedFixturePoints))
	}

	for i, want := range expectedFixturePoints {
		got := resp.Points[i]
		if got.OffsetSeconds != want.offsetSeconds {
			t.Errorf("point[%d] OffsetSeconds = %d, want %d", i, got.OffsetSeconds, want.offsetSeconds)
		}
		if got.DurationSeconds != want.durationSeconds {
			t.Errorf("point[%d] DurationSeconds = %d, want %d", i, got.DurationSeconds, want.durationSeconds)
		}
		if got.Score != want.score {
			t.Errorf("point[%d] Score = %d, want %d (scoring drift — bump Version, do not edit fixture)", i, got.Score, want.score)
		}
		if got.Confidence != want.confidence {
			t.Errorf("point[%d] Confidence = %.6f, want %.6f", i, got.Confidence, want.confidence)
		}
		if got.Reason != want.reason {
			t.Errorf("point[%d] Reason = %q, want %q", i, got.Reason, want.reason)
		}
		if len(got.TopEmotes) != len(want.emotes) {
			t.Errorf("point[%d] TopEmotes length = %d, want %d", i, len(got.TopEmotes), len(want.emotes))
			continue
		}
		for j, we := range want.emotes {
			ge := got.TopEmotes[j]
			if ge.ID != we.id || ge.Name != we.name || ge.ImageURL != we.imageURL || ge.Count != we.count || ge.Provider != we.provider {
				t.Errorf("point[%d] emote[%d] = {ID:%q Name:%q ImageURL:%q Count:%d Provider:%q}, want {ID:%q Name:%q ImageURL:%q Count:%d Provider:%q}",
					i, j, ge.ID, ge.Name, ge.ImageURL, ge.Count, ge.Provider, we.id, we.name, we.imageURL, we.count, we.provider)
			}
		}
		// MinuteTs must track the fixture base timestamp for the rollup index
		// that produced this point (offset / 60 minutes after base).
		wantTS := fixtureBaseTS.Add(time.Duration(want.offsetSeconds/expectedFixtureWindowSeconds) * time.Minute)
		if !got.MinuteTs.Equal(wantTS) {
			t.Errorf("point[%d] MinuteTs = %s, want %s", i, got.MinuteTs, wantTS)
		}
	}
}
