package analytics

import (
	"testing"
	"time"
)

func TestWindowsOverlap(t *testing.T) {
	start := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	otherStart := start.Add(30 * time.Minute)
	otherEnd := start.Add(3 * time.Hour)
	if !windowsOverlap(start, &end, otherStart, &otherEnd) {
		t.Fatal("expected overlapping windows to match")
	}
	disjointEnd := start.Add(-time.Hour)
	disjointStart := start.Add(-3 * time.Hour)
	if windowsOverlap(disjointStart, &disjointEnd, start, &end) {
		t.Fatal("expected disjoint windows not to match")
	}
}

func TestSessionsMatchByTwitchStreamID(t *testing.T) {
	a := sessionCandidate{
		Login:          "ohnepixel",
		TwitchStreamID: "317014684259",
		StartedAt:      time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC),
	}
	b := sessionCandidate{
		Login:          "ohnepixel",
		TwitchStreamID: "317014684259",
		StartedAt:      time.Date(2026, 6, 20, 13, 0, 0, 0, time.UTC),
	}
	if !sessionsMatch(a, b) {
		t.Fatal("expected same twitch stream id to match")
	}
}

func TestSessionsMatchByVodID(t *testing.T) {
	a := sessionCandidate{Login: "ohnepixel", VodID: "123456", StartedAt: time.Now()}
	b := sessionCandidate{Login: "ohnepixel", VodID: "123456", StartedAt: time.Now().Add(time.Hour)}
	if !sessionsMatch(a, b) {
		t.Fatal("expected same vod id to match")
	}
}

func TestMergeViewerSources(t *testing.T) {
	if got := mergeViewerSources(ViewerSourceLive, ViewerSourceTT); got != ViewerSourceMerged {
		t.Fatalf("live+tt = %q, want merged", got)
	}
	if got := mergeViewerSources(ViewerSourceUnknown, ViewerSourceLive); got != ViewerSourceLive {
		t.Fatalf("unknown+live = %q, want live", got)
	}
	if got := mergeViewerSources(ViewerSourceRestored, ViewerSourceTT); got != ViewerSourceRestored {
		t.Fatalf("restored wins, got %q", got)
	}
}

func TestPickCanonicalSessionPrefersLiveRow(t *testing.T) {
	started := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	prefetch := sessionCandidate{
		StreamID:      "316986179940",
		BroadcasterID: "pending",
		IsPlaceholder: true,
		StartedAt:     started,
	}
	live := sessionCandidate{
		StreamID:      "317014684259",
		BroadcasterID: "43683025",
		ViewerSamples: 120,
		ChatMessages:  5000,
		StartedAt:     started,
	}
	got := pickCanonicalSession(prefetch, live)
	if got.StreamID != "317014684259" {
		t.Fatalf("expected live row canonical id, got %s", got.StreamID)
	}
}

func TestCandidateFromInputMarksPlaceholder(t *testing.T) {
	in := SessionResolveInput{
		Login:         "ohnepixel",
		StreamID:      "316986179940",
		StartedAt:     time.Now(),
		Title:         "Syncing...",
		IsPlaceholder: true,
		Source:        ViewerSourceUnknown,
	}
	got := candidateFromInput(in)
	if !got.IsPlaceholder {
		t.Fatal("expected placeholder candidate")
	}
	if got.TTStreamID != "316986179940" {
		t.Fatalf("expected tt stream id fallback, got %q", got.TTStreamID)
	}
	if got.TwitchStreamID != "" {
		t.Fatalf("placeholder should not set twitch stream id, got %q", got.TwitchStreamID)
	}
}

func TestIsPlaceholderStreamTitle(t *testing.T) {
	if !isPlaceholderStreamTitle("Syncing...") {
		t.Fatal("expected syncing title to be placeholder")
	}
	if isPlaceholderStreamTitle("Live CS2") {
		t.Fatal("expected real title not to be placeholder")
	}
}
