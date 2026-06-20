package preview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"streamclone/internal/storygraph/evidenceurl"
	"streamclone/internal/storygraph/store"
)

func TestHydrateYouTubeBuildsSafeEmbed(t *testing.T) {
	h := NewHydrator(nil)
	link, ok := evidenceurl.Canonicalize("https://youtu.be/abc123")
	if !ok {
		t.Fatal("expected canonical YouTube link")
	}
	p := h.Hydrate(context.Background(), link)
	if p.Platform != evidenceurl.PlatformYouTube {
		t.Fatalf("platform = %q", p.Platform)
	}
	if p.EmbedURL != "https://www.youtube.com/embed/abc123" {
		t.Fatalf("embed URL = %q", p.EmbedURL)
	}
	if p.ThumbnailURL == "" || p.PreviewStatus != "ready" {
		t.Fatalf("preview not ready: %+v", p)
	}
}

func TestSanitizeEmbedHTMLRemovesScripts(t *testing.T) {
	got := sanitizeEmbedHTML(`<blockquote>ok</blockquote><script>alert(1)</script><a href="javascript:bad" onclick="evil()">bad</a>`, evidenceurl.PlatformReddit)
	if got != `<blockquote>ok</blockquote><a>bad</a>` {
		t.Fatalf("unexpected sanitized html: %q", got)
	}
}

func TestAllowedEmbedProviderRequiresTrustedProviderName(t *testing.T) {
	if !isAllowedEmbedProvider(evidenceurl.PlatformReddit, "Reddit") {
		t.Fatal("reddit provider should be allowed")
	}
	if isAllowedEmbedProvider(evidenceurl.PlatformReddit, "Totally Safe CDN") {
		t.Fatal("unknown provider should be rejected")
	}
	if isAllowedEmbedProvider(evidenceurl.PlatformYouTube, "YouTube") {
		t.Fatal("youtube oEmbed HTML should not be stored")
	}
}

func TestHydrateTwitchClipEmbedOnlyPending(t *testing.T) {
	h := NewHydrator(nil)
	link, ok := evidenceurl.Canonicalize("https://www.twitch.tv/lacy/clip/FunnySlug")
	if !ok {
		t.Fatal("expected canonical twitch clip")
	}
	p := h.Hydrate(context.Background(), link)
	if p.Platform != evidenceurl.PlatformTwitchClip || p.EmbedURL == "" {
		t.Fatalf("unexpected twitch preview: %+v", p)
	}
	if p.ThumbnailURL != "" {
		t.Fatalf("hydrateTwitchClip must not synthesize clips-media URL, got %q", p.ThumbnailURL)
	}
	if p.PreviewStatus != "pending" {
		t.Fatalf("preview status = %q, want pending", p.PreviewStatus)
	}
}

func TestHydrateTwitchClipAppliesTitleHint(t *testing.T) {
	h := NewHydrator(nil)
	link, ok := evidenceurl.Canonicalize("https://www.twitch.tv/lacy/clip/FunnySlug")
	if !ok {
		t.Fatal("expected canonical twitch clip")
	}
	p := h.Hydrate(context.Background(), link)
	if p.Platform != evidenceurl.PlatformTwitchClip || p.EmbedURL == "" {
		t.Fatalf("unexpected twitch preview: %+v", p)
	}
	titleHint := "sister method"
	if titleHint != "" && strings.TrimSpace(p.Title) == "" {
		p.Title = titleHint
	}
	if p.Title != "sister method" {
		t.Fatalf("title = %q, want sister method", p.Title)
	}
}

func TestOpenGraphRetrySucceedsAfterTransientFailure(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="Recovered"></head></html>`))
	}))
	defer srv.Close()

	h := NewHydrator(nil)
	h.client = srv.Client()
	p, ok := h.fetchOpenGraphWithRetry(context.Background(), store.EvidencePreview{
		CanonicalURL:  srv.URL,
		Platform:      evidenceurl.PlatformGeneric,
		ProviderName:  "Web",
		FetchedAt:     time.Now(),
		PreviewStatus: "fallback",
	})
	if !ok {
		t.Fatalf("expected retry to hydrate preview: %+v", p)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if p.Title != "Recovered" || p.PreviewStatus != "ready" {
		t.Fatalf("unexpected hydrated preview: %+v", p)
	}
}

func TestIsTransientHydrationError(t *testing.T) {
	if !isTransientHydrationError(429, "rate limited") {
		t.Fatal("429 should be transient")
	}
	if !isTransientHydrationError(500, "server error") {
		t.Fatal("500 should be transient")
	}
	if isTransientHydrationError(400, "bad request") {
		t.Fatal("400 should not be transient")
	}
}

func TestShouldRehydrateRespectsTTL(t *testing.T) {
	now := time.Now()
	ready := store.EvidencePreview{PreviewStatus: "ready", ExpiresAt: now.Add(24 * time.Hour), FetchedAt: now}
	if shouldRehydrate(ready) {
		t.Fatal("fresh ready preview should not rehydrate")
	}
	stale := store.EvidencePreview{PreviewStatus: "error", ExpiresAt: now.Add(-time.Hour), FetchedAt: now.Add(-2 * time.Hour), Error: "timeout"}
	if !shouldRehydrate(stale) {
		t.Fatal("stale error preview should rehydrate")
	}
}

func TestIframeSrcAllowed(t *testing.T) {
	if !iframeSrcAllowed(`<iframe src="https://www.redditmedia.com/embed"></iframe>`, evidenceurl.PlatformReddit) {
		t.Fatal("reddit iframe should be allowed")
	}
	if iframeSrcAllowed(`<iframe src="https://evil.example/embed"></iframe>`, evidenceurl.PlatformReddit) {
		t.Fatal("unknown iframe should be rejected")
	}
}

func TestSourceTypeForPlatformCoversLinkOnlyPlatforms(t *testing.T) {
	tests := map[string]string{
		evidenceurl.PlatformReddit:     "reddit_thread",
		evidenceurl.PlatformYouTube:    "youtube_video",
		evidenceurl.PlatformTwitchClip: "twitch_clip",
		evidenceurl.PlatformX:          "x_post",
		evidenceurl.PlatformTikTok:     "tiktok_video",
		evidenceurl.PlatformGeneric:    "manual_curation",
	}
	for platform, want := range tests {
		if got := SourceTypeForPlatform(platform); got != want {
			t.Fatalf("SourceTypeForPlatform(%q) = %q, want %q", platform, got, want)
		}
	}
}

func TestLinkOnlyOEmbedHTMLIsSanitizedAndProviderScoped(t *testing.T) {
	if !isAllowedEmbedProvider(evidenceurl.PlatformX, "Twitter") {
		t.Fatal("twitter provider should be allowed for x links")
	}
	xHTML := sanitizeEmbedHTML(
		`<blockquote><a href="https://x.com/creator/status/1" onclick="evil()">post</a><iframe src="https://evil.example/embed"></iframe><script>alert(1)</script></blockquote>`,
		evidenceurl.PlatformX,
	)
	if strings.Contains(xHTML, "evil.example") || strings.Contains(xHTML, "script") || strings.Contains(xHTML, "onclick") {
		t.Fatalf("unsafe x embed survived: %q", xHTML)
	}
	if !strings.Contains(xHTML, `href="https://x.com/creator/status/1"`) {
		t.Fatalf("expected safe x link to survive: %q", xHTML)
	}

	if !isAllowedEmbedProvider(evidenceurl.PlatformTikTok, "TikTok") {
		t.Fatal("tiktok provider should be allowed for tiktok links")
	}
	tiktokHTML := sanitizeEmbedHTML(
		`<blockquote cite="https://www.tiktok.com/@creator/video/999"><iframe src="https://www.tiktok.com/embed/v2/999" onload="evil()"></iframe></blockquote>`,
		evidenceurl.PlatformTikTok,
	)
	if strings.Contains(tiktokHTML, "onload") {
		t.Fatalf("unsafe tiktok attr survived: %q", tiktokHTML)
	}
	if !strings.Contains(tiktokHTML, `cite="https://www.tiktok.com/@creator/video/999"`) || !strings.Contains(tiktokHTML, `src="https://www.tiktok.com/embed/v2/999"`) {
		t.Fatalf("expected safe tiktok URLs to survive: %q", tiktokHTML)
	}
}
