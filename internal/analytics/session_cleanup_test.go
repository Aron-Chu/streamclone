package analytics

import (
	"testing"
	"time"
)

func TestShouldMergeStubPairRequiresPlaceholder(t *testing.T) {
	started := mustTime("2026-06-20T15:47:54Z")
	stub := sessionCandidate{
		StreamID:      "318176155351",
		Login:         "ohnepixel",
		IsPlaceholder: true,
		Title:         "Syncing...",
		StartedAt:     started,
	}
	live := sessionCandidate{
		StreamID:       "319181844960",
		Login:          "ohnepixel",
		TwitchStreamID: "319181844960",
		Title:          "IEM COLOGNE",
		BroadcasterID:  "43683025",
		ViewerSamples:  883,
		StartedAt:      started,
	}
	if !shouldMergeStubPair(stub, live) {
		t.Fatal("expected placeholder stub to merge with overlapping live session")
	}

	distinctA := sessionCandidate{
		StreamID:       "319185549408",
		Login:          "sodapoppin",
		TwitchStreamID: "319185549408",
		Title:          "GYM STREAM",
		StartedAt:      mustTime("2026-06-20T18:01:30Z"),
	}
	distinctB := sessionCandidate{
		StreamID:       "319097572060",
		Login:          "sodapoppin",
		TwitchStreamID: "319097572060",
		Title:          "HIDING",
		StartedAt:      mustTime("2026-06-19T17:59:49Z"),
	}
	if shouldMergeStubPair(distinctA, distinctB) {
		t.Fatal("expected distinct canonical streams not to merge")
	}
}

func TestNormalizeLoginListDedupes(t *testing.T) {
	got := normalizeLoginList([]string{" OhnePixel ", "ohnepixel", "xqc", ""})
	if len(got) != 2 || got[0] != "ohnepixel" || got[1] != "xqc" {
		t.Fatalf("unexpected normalized logins: %#v", got)
	}
}

func mustTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
