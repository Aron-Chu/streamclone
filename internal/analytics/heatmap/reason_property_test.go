package heatmap

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: moment-timeline, Property 12: Reason Label Selection
//
// selectReason must attach exactly one Reason_Label to a scored window, drawn
// from the valid label set. The winning label is the signal component with the
// highest individual z-score; when no signal exceeds reasonZThreshold (1.0) the
// window falls back to chat_spike. chatRate -> chat_spike, viewerMomentum ->
// viewer_spike, and the emote-family signals (providerSpike, emoteRate,
// topEmoteDominance, novelty) resolve through the window's raw provider rates to
// a concrete provider label, or chat_spike when the window has no provider emote
// activity.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// **Validates: Requirements 10.1, 10.2**

// allSignalKeys is the fixed set of six per-window signal keys selectReason
// inspects. Keeping them together lets the generators always populate a full
// map so no key falls back to the zero value implicitly.
var allSignalKeys = []string{
	sigChatRate,
	sigEmoteRate,
	sigViewerMomentum,
	sigProviderSpike,
	sigTopEmoteDominance,
	sigNovelty,
}

// drawRawSignals draws an arbitrary rawWindowSignals with non-negative provider
// rates. Provider rates are log-transformed counts in the real engine, so they
// are always >= 0; the range spans 0 (no activity) up well past any realistic
// per-window value.
func drawRawSignals(t *rapid.T) rawWindowSignals {
	return rawWindowSignals{
		sevenTVRate: rapid.Float64Range(0, 20).Draw(t, "sevenTVRate"),
		twitchRate:  rapid.Float64Range(0, 20).Draw(t, "twitchRate"),
		ffzRate:     rapid.Float64Range(0, 20).Draw(t, "ffzRate"),
	}
}

// TestSelectReasonAlwaysValidSingleLabel checks the universal invariant: for any
// arbitrary signal map and raw provider rates, selectReason returns exactly one
// label that is a member of the valid Reason_Label set (Requirement 10.1).
func TestSelectReasonAlwaysValidSingleLabel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		signals := make(map[string]float64, len(allSignalKeys))
		for _, k := range allSignalKeys {
			// z-scores span both negative and positive surprise; the range is
			// wide enough to exercise the fallback band and clear winners.
			signals[k] = rapid.Float64Range(-10, 10).Draw(t, k)
		}
		raw := drawRawSignals(t)

		got := selectReason(signals, raw)
		if !IsValidReason(got) {
			t.Fatalf("selectReason returned invalid label %q for signals=%v raw=%+v", got, signals, raw)
		}
	})
}

// TestSelectReasonFallbackBelowThreshold checks that when every signal z-score
// is at or below reasonZThreshold (1.0), selectReason returns the chat_spike
// fallback regardless of the raw provider rates (Requirement 10.2).
func TestSelectReasonFallbackBelowThreshold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		signals := make(map[string]float64, len(allSignalKeys))
		for _, k := range allSignalKeys {
			// All signals constrained to <= 1.0 so the max cannot clear the bar.
			signals[k] = rapid.Float64Range(-10, reasonZThreshold).Draw(t, k)
		}
		raw := drawRawSignals(t)

		got := selectReason(signals, raw)
		if got != ReasonChatSpike {
			t.Fatalf("max z <= %v should fall back to %q, got %q (signals=%v)", reasonZThreshold, ReasonChatSpike, got, signals)
		}
	})
}

// drawConstrainedSignals builds a signal map where winnerKey holds the strict
// unique maximum z-score, that maximum exceeds reasonZThreshold, and every other
// signal sits at or below the threshold. This isolates a single deterministic
// winner so the label mapping can be asserted directly.
func drawConstrainedSignals(t *rapid.T, winnerKey string) map[string]float64 {
	signals := make(map[string]float64, len(allSignalKeys))
	for _, k := range allSignalKeys {
		// Losers stay <= 1.0, strictly below any winner value.
		signals[k] = rapid.Float64Range(-10, reasonZThreshold).Draw(t, k+"_loser")
	}
	// Winner is comfortably above the threshold and above every loser (which are
	// all <= 1.0), making it the strict unique maximum.
	signals[winnerKey] = rapid.Float64Range(reasonZThreshold+0.5, 50).Draw(t, winnerKey+"_winner")
	return signals
}

// TestSelectReasonChatRateWins checks that when chatRate is the strict unique
// maximum and exceeds the threshold, the label is chat_spike (Requirement 10.2).
func TestSelectReasonChatRateWins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		signals := drawConstrainedSignals(t, sigChatRate)
		raw := drawRawSignals(t)

		got := selectReason(signals, raw)
		if got != ReasonChatSpike {
			t.Fatalf("chatRate unique max should map to %q, got %q (signals=%v)", ReasonChatSpike, got, signals)
		}
	})
}

// TestSelectReasonViewerMomentumWins checks that when viewerMomentum is the
// strict unique maximum and exceeds the threshold, the label is viewer_spike
// (Requirement 10.2).
func TestSelectReasonViewerMomentumWins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		signals := drawConstrainedSignals(t, sigViewerMomentum)
		raw := drawRawSignals(t)

		got := selectReason(signals, raw)
		if got != ReasonViewerSpike {
			t.Fatalf("viewerMomentum unique max should map to %q, got %q (signals=%v)", ReasonViewerSpike, got, signals)
		}
	})
}

// TestSelectReasonEmoteFamilyWins checks that when any emote-family signal
// (providerSpike, emoteRate, topEmoteDominance, novelty) is the strict unique
// maximum above the threshold, the result resolves through the raw provider
// rates: a concrete provider spike label when the window has provider emote
// activity, or chat_spike when it has none (Requirements 10.1, 10.2).
func TestSelectReasonEmoteFamilyWins(t *testing.T) {
	emoteFamily := []string{sigProviderSpike, sigEmoteRate, sigTopEmoteDominance, sigNovelty}
	providerLabels := map[string]struct{}{
		ReasonSevenTVSpike:     {},
		ReasonTwitchEmoteSpike: {},
		ReasonFFZSpike:         {},
	}

	rapid.Check(t, func(t *rapid.T) {
		winnerKey := emoteFamily[rapid.IntRange(0, len(emoteFamily)-1).Draw(t, "emoteFamilyKey")]
		signals := drawConstrainedSignals(t, winnerKey)
		raw := drawRawSignals(t)

		got := selectReason(signals, raw)
		if !IsValidReason(got) {
			t.Fatalf("emote-family winner produced invalid label %q (signals=%v raw=%+v)", got, signals, raw)
		}

		hasProviderActivity := raw.sevenTVRate > 0 || raw.twitchRate > 0 || raw.ffzRate > 0
		if hasProviderActivity {
			if _, ok := providerLabels[got]; !ok {
				t.Fatalf("emote-family winner with provider activity should map to a provider label, got %q (raw=%+v)", got, raw)
			}
		} else if got != ReasonChatSpike {
			t.Fatalf("emote-family winner with no provider activity should map to %q, got %q (raw=%+v)", ReasonChatSpike, got, raw)
		}
	})
}
