package evidenceurl_test

import (
	"testing"

	"streamclone/internal/storygraph/evidenceurl"
)

func TestCanonicalizeKnownPlatforms(t *testing.T) {
	tests := []struct {
		raw      string
		platform string
		want     string
	}{
		{"https://youtu.be/abc123?t=7", evidenceurl.PlatformYouTube, "https://www.youtube.com/watch?v=abc123"},
		{"https://youtube.com/shorts/shortID?feature=share", evidenceurl.PlatformYouTube, "https://www.youtube.com/watch?v=shortID"},
		{"https://m.youtube.com/watch?v=mobileID&feature=share", evidenceurl.PlatformYouTube, "https://www.youtube.com/watch?v=mobileID"},
		{"https://twitter.com/user/status/123?s=20", evidenceurl.PlatformX, "https://x.com/user/status/123"},
		{"https://mobile.x.com/user/status/456/", evidenceurl.PlatformX, "https://x.com/user/status/456"},
		{"https://www.tiktok.com/@creator/video/999?lang=en", evidenceurl.PlatformTikTok, "https://www.tiktok.com/@creator/video/999"},
		{"https://www.twitch.tv/streamer/clip/FunnySlug?filter=clips", evidenceurl.PlatformTwitchClip, "https://clips.twitch.tv/FunnySlug"},
		{"https://old.reddit.com/r/LivestreamFail/comments/abc/title/?utm_source=share", evidenceurl.PlatformReddit, "https://www.reddit.com/r/LivestreamFail/comments/abc/title/"},
		{"https://redd.it/xyz789.", evidenceurl.PlatformReddit, "https://www.reddit.com/comments/xyz789"},
	}
	for _, tt := range tests {
		got, ok := evidenceurl.Canonicalize(tt.raw)
		if !ok {
			t.Fatalf("Canonicalize(%q) failed", tt.raw)
		}
		if got.Platform != tt.platform || got.CanonicalURL != tt.want {
			t.Fatalf("Canonicalize(%q) = (%s, %s), want (%s, %s)", tt.raw, got.Platform, got.CanonicalURL, tt.platform, tt.want)
		}
	}
}

func TestAttachableSkipsTwitchCDNAssets(t *testing.T) {
	thumb, ok := evidenceurl.Canonicalize("https://static-cdn.jtvnw.net/twitch-video-assets/foo/thumb/thumb-0000000000-480x272.jpg")
	if !ok {
		t.Fatal("expected canonical CDN url")
	}
	if evidenceurl.Attachable(thumb) {
		t.Fatal("expected twitch CDN thumbnail to be skipped")
	}
	clip, ok := evidenceurl.Canonicalize("https://www.twitch.tv/lacy/clip/FunnySlug")
	if !ok || !evidenceurl.Attachable(clip) {
		t.Fatal("expected twitch clip page to remain attachable")
	}
}

func TestCanonicalizeRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"not-a-url",
		"ftp://example.com/file",
		"https://",
		"https://m.youtube.com/watch?feature=share",
		"https://youtube.com/shorts/",
		"https://user:pass@x.com/user/status/123",
		"https://reddit.com/u/someone",
	}
	for _, raw := range bad {
		if _, ok := evidenceurl.Canonicalize(raw); ok {
			t.Fatalf("Canonicalize(%q) should fail", raw)
		}
	}
}

func TestCanonicalizeDoesNotTreatSuffixLookalikesAsKnownPlatforms(t *testing.T) {
	got, ok := evidenceurl.Canonicalize("https://notyoutube.com/watch?v=abc")
	if !ok {
		t.Fatal("generic web URL should still canonicalize")
	}
	if got.Platform != evidenceurl.PlatformGeneric {
		t.Fatalf("platform = %q, want %q", got.Platform, evidenceurl.PlatformGeneric)
	}
	if got.CanonicalURL != "https://notyoutube.com/watch" {
		t.Fatalf("canonical URL = %q", got.CanonicalURL)
	}
}

func TestExtractDeduplicates(t *testing.T) {
	links := evidenceurl.Extract("watch https://youtu.be/abc and again https://www.youtube.com/watch?v=abc&t=2")
	if len(links) != 1 {
		t.Fatalf("expected one canonical link, got %d: %+v", len(links), links)
	}
}

func TestExtractRecognizesKnownPlatformURLs(t *testing.T) {
	text := `sources:
	https://www.reddit.com/r/LivestreamFail/comments/abc/title/?utm_source=share
	https://x.com/creator/status/123?s=20
	https://www.tiktok.com/@creator/video/999?lang=en
	https://youtube.com/shorts/shortID?feature=share
	https://www.twitch.tv/streamer/clip/FunnySlug?filter=clips`
	links := evidenceurl.Extract(text)
	want := map[string]string{
		"https://www.reddit.com/r/LivestreamFail/comments/abc/title/": evidenceurl.PlatformReddit,
		"https://x.com/creator/status/123":                            evidenceurl.PlatformX,
		"https://www.tiktok.com/@creator/video/999":                   evidenceurl.PlatformTikTok,
		"https://www.youtube.com/watch?v=shortID":                     evidenceurl.PlatformYouTube,
		"https://clips.twitch.tv/FunnySlug":                           evidenceurl.PlatformTwitchClip,
	}
	if len(links) != len(want) {
		t.Fatalf("link count = %d, want %d: %+v", len(links), len(want), links)
	}
	for _, link := range links {
		if got, ok := want[link.CanonicalURL]; !ok || got != link.Platform {
			t.Fatalf("unexpected link %+v, want platform %q", link, got)
		}
	}
}

func TestExtractTrailingPunctuation(t *testing.T) {
	links := evidenceurl.Extract(`See https://clips.twitch.tv/FunnySlug).`)
	if len(links) != 1 {
		t.Fatalf("expected one link, got %d", len(links))
	}
	if links[0].CanonicalURL != "https://clips.twitch.tv/FunnySlug" {
		t.Fatalf("unexpected canonical url: %q", links[0].CanonicalURL)
	}
}
