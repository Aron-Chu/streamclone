package store

import "testing"

func TestTwitchClipPreviewURLRejectsRedditPostID(t *testing.T) {
	if got := twitchClipPreviewURL("https://www.reddit.com/r/LivestreamFail/comments/1u8f3ut/example/"); got != "" {
		t.Fatalf("expected empty for reddit URL, got %q", got)
	}
	if got := twitchClipPreviewFromSlug("1u8f3ut"); got != "" {
		t.Fatalf("reddit post id must not become clip preview, got %q", got)
	}
}

func TestTwitchClipPreviewURLFromClipLink(t *testing.T) {
	got := twitchClipPreviewURL("https://clips.twitch.tv/FurrySpinelessDogCoolCat-_GW0JH5No8Lm5_sc")
	want := "https://clips-media-assets2.twitch.tv/FurrySpinelessDogCoolCat-_GW0JH5No8Lm5_sc-preview-480x272.jpg"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreferServingThumbPrefersJTVNW(t *testing.T) {
	evidence := "https://clips-media-assets2.twitch.tv/FurrySpinelessDogCoolCat-_GW0JH5No8Lm5_sc-preview-480x272.jpg"
	helix := "https://static-cdn.jtvnw.net/twitch-video-assets/twitch-vap-video-assets-prod-us-west-2/583c2eee-a52d-4990-83d9-77fd27f7b51f/landscape/thumb/thumb-0000000000-480x272.jpg"
	if got := preferServingThumb(evidence, helix); got != helix {
		t.Fatalf("got %q want helix jtvnw URL", got)
	}
}

func TestResolvePreviewRedditAndNone(t *testing.T) {
	kind, raw, proxied := resolvePreview("", "", "Example headline", "https://reddit.com/x")
	if kind != "none" || raw != "" || proxied != "" {
		t.Fatalf("text-only reddit post should have no preview: kind=%s raw=%q proxied=%q", kind, raw, proxied)
	}
	kind, raw, proxied = resolvePreview("https://preview.redd.it/abc.jpg", "", "", "")
	if kind != "reddit" || raw == "" || proxied == "" {
		t.Fatalf("reddit preview expected, got kind=%s raw=%q proxied=%q", kind, raw, proxied)
	}
}
