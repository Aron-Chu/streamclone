package store

import "testing"

func TestResolveDisplayTitlePrefersPreview(t *testing.T) {
	got := ResolveDisplayTitle(
		"17 comments",
		"14 comments",
		[]string{"Mitch Jones gets cooked by the new AI summary"},
		"https://reddit.com/r/LivestreamFail/comments/abc/mitch_jones_gets_cooked/",
	)
	want := "Mitch Jones gets cooked by the new AI summary"
	if got != want {
		t.Fatalf("ResolveDisplayTitle = %q, want %q", got, want)
	}
}

func TestResolveDisplayTitleUsesSlugBeforeClusterPlaceholder(t *testing.T) {
	got := ResolveDisplayTitle(
		"17 comments",
		"17 comments",
		nil,
		"https://reddit.com/r/LivestreamFail/comments/abc/streamer_does_something_wild/",
	)
	if got != "streamer does something wild" {
		t.Fatalf("unexpected slug title %q", got)
	}
}

func TestIsPlaceholderTitle(t *testing.T) {
	if !IsPlaceholderTitle("17 comments") {
		t.Fatal("comment count should be placeholder")
	}
	if IsPlaceholderTitle("Mitch Jones gets cooked") {
		t.Fatal("real headline should not be placeholder")
	}
}
