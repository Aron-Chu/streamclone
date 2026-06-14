package heatmap

import (
	"math"
	"testing"
	"time"
)

const floatTol = 1e-9

func TestLogTransform(t *testing.T) {
	if got := logTransform(0); got != 0 {
		t.Errorf("logTransform(0) = %v, want 0", got)
	}
	if got := logTransform(1); math.Abs(got-math.Ln2) > floatTol {
		t.Errorf("logTransform(1) = %v, want ln(2)=%v", got, math.Ln2)
	}
	// Monotonic and finite for increasing counts.
	prev := math.Inf(-1)
	for _, v := range []float64{0, 1, 5, 100, 1e6} {
		got := logTransform(v)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("logTransform(%v) not finite: %v", v, got)
		}
		if got < 0 {
			t.Errorf("logTransform(%v) negative: %v", v, got)
		}
		if got <= prev {
			t.Errorf("logTransform not increasing at %v: %v <= %v", v, got, prev)
		}
		prev = got
	}
}

func TestZScoreDivideByZero(t *testing.T) {
	// All identical values -> stddev 0 -> z-score 0.
	vals := []float64{4, 4, 4, 4}
	for i := range vals {
		if got := zScore(vals, i); got != 0 {
			t.Errorf("zScore(identical, %d) = %v, want 0", i, got)
		}
	}
	// Empty and single-element distributions.
	if got := zScore(nil, 0); got != 0 {
		t.Errorf("zScore(nil,0) = %v, want 0", got)
	}
	if got := zScore([]float64{9}, 0); got != 0 {
		t.Errorf("zScore(single,0) = %v, want 0", got)
	}
	// Out-of-range index.
	if got := zScore([]float64{1, 2, 3}, 5); got != 0 {
		t.Errorf("zScore(out-of-range) = %v, want 0", got)
	}
}

func TestZScoreSliceStats(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7}
	z := zScoreSlice(vals)
	if len(z) != len(vals) {
		t.Fatalf("zScoreSlice length = %d, want %d", len(z), len(vals))
	}
	if m := mean(z); math.Abs(m) > 1e-9 {
		t.Errorf("z-score mean = %v, want ~0", m)
	}
	if sd := stddev(z); math.Abs(sd-1.0) > 1e-9 {
		t.Errorf("z-score population stddev = %v, want ~1", sd)
	}
}

func TestProviderEmoteCounts(t *testing.T) {
	emotes := map[string]int{
		"twitch:t1:Kappa":  3,
		"twitch:t2:PogU":   2,
		"ffz:f1:catJAM":    5,
		"seventv:s1:OMEGA": 9, // 7TV comes from the dedicated field, not this map
		"badkey":           4, // no provider prefix -> ignored
	}
	twitch, ffz := providerEmoteCounts(emotes)
	if twitch != 5 {
		t.Errorf("twitch count = %d, want 5", twitch)
	}
	if ffz != 5 {
		t.Errorf("ffz count = %d, want 5", ffz)
	}
}

func TestTopEmoteHelpers(t *testing.T) {
	emotes := map[string]int{
		"seventv:a:AAA": 2,
		"seventv:b:BBB": 7,
		"seventv:c:CCC": 7, // tie with BBB; lexical smallest key wins
	}
	if got := topEmoteCount(emotes); got != 7 {
		t.Errorf("topEmoteCount = %d, want 7", got)
	}
	if got := topEmoteKey(emotes); got != "seventv:b:BBB" {
		t.Errorf("topEmoteKey = %q, want seventv:b:BBB", got)
	}
	if got := topEmoteCount(nil); got != 0 {
		t.Errorf("topEmoteCount(nil) = %d, want 0", got)
	}
	if got := topEmoteKey(nil); got != "" {
		t.Errorf("topEmoteKey(nil) = %q, want empty", got)
	}
}

func TestExtractRawSignalsMomentumAndNovelty(t *testing.T) {
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{
		{
			MinuteTS:        base,
			ViewerAvg:       100,
			ChatCount:       0,
			TotalEmoteCount: 4,
			Emotes:          map[string]int{"seventv:a:AAA": 4},
		},
		{
			MinuteTS:        base.Add(time.Minute),
			ViewerAvg:       150,
			ChatCount:       9,
			TotalEmoteCount: 4,
			Emotes:          map[string]int{"seventv:a:AAA": 4}, // same dominant emote as window 0
		},
	}
	raw := extractRawSignals(rollups)
	if len(raw) != 2 {
		t.Fatalf("extractRawSignals length = %d, want 2", len(raw))
	}

	// Window 0: first window momentum is 0; dominant emote is new -> novelty 1.0.
	if raw[0].viewerMomentum != 0 {
		t.Errorf("window0 viewerMomentum = %v, want 0", raw[0].viewerMomentum)
	}
	if math.Abs(raw[0].novelty-1.0) > floatTol {
		t.Errorf("window0 novelty = %v, want 1.0", raw[0].novelty)
	}
	if math.Abs(raw[0].topEmoteDominance-1.0) > floatTol {
		t.Errorf("window0 topEmoteDominance = %v, want 1.0", raw[0].topEmoteDominance)
	}

	// Window 1: momentum 150-100=50; dominant emote seen before (count 4 of 4)
	// -> novelty = 1 - 4/4 = 0.
	if raw[1].viewerMomentum != 50 {
		t.Errorf("window1 viewerMomentum = %v, want 50", raw[1].viewerMomentum)
	}
	if math.Abs(raw[1].novelty) > floatTol {
		t.Errorf("window1 novelty = %v, want 0", raw[1].novelty)
	}
	if math.Abs(raw[1].chatRate-logTransform(9)) > floatTol {
		t.Errorf("window1 chatRate = %v, want ln(10)", raw[1].chatRate)
	}
}

func TestExtractSignalsNormalizedKeys(t *testing.T) {
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{
		{MinuteTS: base, ViewerAvg: 10, ChatCount: 1, TotalEmoteCount: 1, SevenTVEmoteCount: 1, Emotes: map[string]int{"seventv:a:AAA": 1}},
		{MinuteTS: base.Add(time.Minute), ViewerAvg: 50, ChatCount: 40, TotalEmoteCount: 30, SevenTVEmoteCount: 20, Emotes: map[string]int{"seventv:b:BBB": 30}},
		{MinuteTS: base.Add(2 * time.Minute), ViewerAvg: 12, ChatCount: 2, TotalEmoteCount: 2, SevenTVEmoteCount: 1, Emotes: map[string]int{"twitch:t:Kappa": 2}},
	}
	normalized, raw := extractSignals(rollups)
	if len(normalized) != 3 || len(raw) != 3 {
		t.Fatalf("extractSignals lengths = %d/%d, want 3/3", len(normalized), len(raw))
	}
	wantKeys := []string{sigChatRate, sigEmoteRate, sigViewerMomentum, sigProviderSpike, sigTopEmoteDominance, sigNovelty}
	for _, m := range normalized {
		if len(m) != len(wantKeys) {
			t.Errorf("signal map has %d keys, want %d", len(m), len(wantKeys))
		}
		for _, k := range wantKeys {
			if _, ok := m[k]; !ok {
				t.Errorf("signal map missing key %q", k)
			}
		}
	}

	// The clear spike window (index 1) should carry the largest chat z-score.
	if normalized[1][sigChatRate] <= normalized[0][sigChatRate] {
		t.Errorf("spike window chat z-score not highest: %v vs %v", normalized[1][sigChatRate], normalized[0][sigChatRate])
	}
}

func TestExtractSignalsEmpty(t *testing.T) {
	normalized, raw := extractSignals(nil)
	if len(normalized) != 0 || len(raw) != 0 {
		t.Errorf("extractSignals(nil) = %d/%d, want 0/0", len(normalized), len(raw))
	}
}
