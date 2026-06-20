package ingest

import (
	"reflect"
	"testing"

	"streamclone/internal/config"
	"streamclone/internal/social"
)

func TestExtractAttachableCommentLinksDedupesAndFilters(t *testing.T) {
	comments := []social.Item{
		{
			Text: "repost https://youtu.be/abc123?t=12 and same https://www.youtube.com/watch?v=abc123",
		},
		{
			Text: "origin clip https://clips.twitch.tv/FunnySlug plus X https://twitter.com/user/status/123?s=20",
		},
		{
			Text: "tiktok https://www.tiktok.com/@creator/video/999?lang=en thumb https://static-cdn.jtvnw.net/twitch-video-assets/foo/thumb/thumb-0000000000-480x272.jpg",
		},
	}

	got := extractAttachableCommentLinks(comments)
	want := []string{
		"https://www.youtube.com/watch?v=abc123",
		"https://clips.twitch.tv/FunnySlug",
		"https://x.com/user/status/123",
		"https://www.tiktok.com/@creator/video/999",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractAttachableCommentLinks() = %#v, want %#v", got, want)
	}
}

func TestBrowserFetchBudgetsClampToBoundedDefaults(t *testing.T) {
	cfg := config.Config{}
	if got := socialBrowserFetchBudget(cfg); got != -1 {
		t.Fatalf("socialBrowserFetchBudget() = %d, want -1", got)
	}
	if got := youtubeBrowserFetchBudget(cfg); got != -1 {
		t.Fatalf("youtubeBrowserFetchBudget() = %d, want -1", got)
	}

	cfg.SocialBrowserFetchBudget = 3
	cfg.YouTubeBrowserFetchBudget = 4
	if got := socialBrowserFetchBudget(cfg); got != 3 {
		t.Fatalf("socialBrowserFetchBudget(custom) = %d, want 3", got)
	}
	if got := youtubeBrowserFetchBudget(cfg); got != 4 {
		t.Fatalf("youtubeBrowserFetchBudget(custom) = %d, want 4", got)
	}
}
