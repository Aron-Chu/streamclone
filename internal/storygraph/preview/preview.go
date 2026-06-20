package preview

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"

	"streamclone/internal/social/reddit"
	"streamclone/internal/storygraph/evidenceurl"
	"streamclone/internal/storygraph/store"
)

const (
	maxMetadataBytes = 512 * 1024
	readyPreviewTTL  = 7 * 24 * time.Hour
	errorRetryAfter  = time.Hour
	maxHydrateTries  = 3
)

var (
	metaRe           = regexp.MustCompile(`(?is)<meta\s+[^>]*(?:property|name)=["']([^"']+)["'][^>]*content=["']([^"']*)["'][^>]*>`)
	titleRe          = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptTagRe      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleTagRe       = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	jsURLRe          = regexp.MustCompile(`(?i)javascript:`)
	eventHandlerRe   = regexp.MustCompile(`(?i)\s(on\w+)\s*=`)
	youtubeIDRe      = regexp.MustCompile(`[?&]v=([^&]+)`)
	twitchSlugRe     = regexp.MustCompile(`^https://clips\.twitch\.tv/([^/?#]+)`)
	twitchClipPathRe = regexp.MustCompile(`/clip/([^/?#]+)`)
	allowedEmbedTag  = regexp.MustCompile(`(?is)^<(blockquote|iframe|a|p|div|span)\b`)
	iframeSrcRe      = regexp.MustCompile(`(?i)\ssrc=["']([^"']+)["']`)
)

// Hydrator fetches bounded preview metadata for canonical evidence URLs.
type Hydrator struct {
	client *http.Client
	logger *slog.Logger
}

func NewHydrator(logger *slog.Logger) *Hydrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hydrator{
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

// AttachURL canonicalizes rawURL, hydrates or reuses a preview, and links it to the story.
func (h *Hydrator) AttachURL(ctx context.Context, st *store.Store, clusterID int64, evidenceID *int64, rawURL, matchKind, note, titleHint string) (store.EvidencePreview, bool, error) {
	link, ok := evidenceurl.Canonicalize(rawURL)
	if !ok {
		return store.EvidencePreview{}, false, nil
	}
	if !evidenceurl.Attachable(link) {
		return store.EvidencePreview{}, false, nil
	}
	existing, linked, err := st.FindPreviewLinkByCanonical(ctx, clusterID, link.CanonicalURL)
	if err != nil {
		return store.EvidencePreview{}, false, err
	}
	if linked && existing != nil {
		return *existing, true, nil
	}
	preview, reused := h.hydrateOrReuse(ctx, st, link)
	if titleHint = strings.TrimSpace(titleHint); titleHint != "" && strings.TrimSpace(preview.Title) == "" {
		preview.Title = titleHint
	}
	previewID, err := st.UpsertEvidencePreview(ctx, preview)
	if err != nil {
		return store.EvidencePreview{}, false, err
	}
	preview.ID = previewID
	if err := st.LinkEvidencePreview(ctx, clusterID, evidenceID, previewID, matchKind, note); err != nil {
		return store.EvidencePreview{}, false, err
	}
	return preview, reused, nil
}

func (h *Hydrator) hydrateOrReuse(ctx context.Context, st *store.Store, link evidenceurl.Link) (store.EvidencePreview, bool) {
	if existing, err := st.GetEvidencePreviewByCanonical(ctx, link.CanonicalURL); err == nil && existing != nil {
		if !shouldRehydrate(*existing) {
			return *existing, true
		}
	}
	return h.Hydrate(ctx, link), false
}

func shouldRehydrate(p store.EvidencePreview) bool {
	now := time.Now()
	if p.NextFetchAt != nil {
		return !now.Before(*p.NextFetchAt)
	}
	if p.ExpiresAt.IsZero() || !now.Before(p.ExpiresAt) {
		return true
	}
	status := computePreviewStatus(p)
	if status == "ready" {
		if p.Platform == evidenceurl.PlatformReddit && strings.TrimSpace(p.ThumbnailURL) == "" {
			return !p.FetchedAt.IsZero() && now.Sub(p.FetchedAt) >= errorRetryAfter
		}
		return false
	}
	return !p.FetchedAt.IsZero() && now.Sub(p.FetchedAt) >= errorRetryAfter
}

func computePreviewStatus(p store.EvidencePreview) string {
	if p.PreviewStatus != "" {
		return p.PreviewStatus
	}
	if p.Error != "" {
		return "error"
	}
	if p.EmbedURL != "" || p.EmbedHTML != "" || p.ThumbnailURL != "" || p.Title != "" {
		return "ready"
	}
	return "fallback"
}

func (h *Hydrator) Hydrate(ctx context.Context, link evidenceurl.Link) store.EvidencePreview {
	now := time.Now()
	nextFetchAt := now.Add(readyPreviewTTL)
	p := store.EvidencePreview{
		CanonicalURL:  link.CanonicalURL,
		Platform:      link.Platform,
		ProviderName:  providerName(link.Platform),
		FetchedAt:     now,
		ExpiresAt:     nextFetchAt,
		NextFetchAt:   &nextFetchAt,
		PreviewStatus: "fallback",
	}

	switch link.Platform {
	case evidenceurl.PlatformYouTube:
		return hydrateYouTube(p)
	case evidenceurl.PlatformTwitchClip:
		return hydrateTwitchClip(p)
	case evidenceurl.PlatformTikTok:
		if hydrated, ok := h.fetchOEmbedWithRetry(ctx, p, "https://www.tiktok.com/oembed?url="+url.QueryEscape(link.CanonicalURL)); ok {
			return hydrated
		}
	case evidenceurl.PlatformX:
		if hydrated, ok := h.fetchOEmbedWithRetry(ctx, p, "https://publish.twitter.com/oembed?url="+url.QueryEscape(link.CanonicalURL)); ok {
			return hydrated
		}
	case evidenceurl.PlatformReddit:
		oembedURL := normalizeRedditOEmbedURL(link.CanonicalURL)
		if hydrated, ok := h.fetchOEmbedWithRetry(ctx, p, "https://www.reddit.com/oembed?url="+url.QueryEscape(oembedURL)); ok {
			h.enrichRedditPreview(ctx, &hydrated, oembedURL)
			return hydrated
		}
		if meta, ok := reddit.FetchPostMeta(ctx, h.client, "streamclone/1.0", link.CanonicalURL); ok {
			if meta.Thumbnail != "" {
				p.ThumbnailURL = meta.Thumbnail
			}
			if meta.Thumbnail == "" {
				if clip := twitchClipSlug(meta.ExternalURL); clip != "" {
					p.ThumbnailURL = "https://clips-media-assets2.twitch.tv/" + clip + "-preview-480x272.jpg"
				}
			}
			p.PreviewStatus = previewStatus(p)
			p.ExpiresAt = previewExpiresAt(p)
			nextFetchAt := p.ExpiresAt
			p.NextFetchAt = &nextFetchAt
			return p
		}
	}

	if hydrated, ok := h.fetchOpenGraphWithRetry(ctx, p); ok {
		return hydrated
	}
	if p.Error != "" {
		p.PreviewStatus = "error"
		p.RetryCount = 1
		p.ExpiresAt = time.Now().Add(errorRetryAfter)
		nextFetchAt := p.ExpiresAt
		p.NextFetchAt = &nextFetchAt
	}
	return p
}

func hydrateYouTube(p store.EvidencePreview) store.EvidencePreview {
	id := youtubeID(p.CanonicalURL)
	if id == "" {
		return p
	}
	p.ThumbnailURL = "https://img.youtube.com/vi/" + id + "/hqdefault.jpg"
	p.EmbedURL = "https://www.youtube.com/embed/" + id
	p.PreviewStatus = "ready"
	p.Error = ""
	return p
}

func hydrateTwitchClip(p store.EvidencePreview) store.EvidencePreview {
	slug := twitchClipSlug(p.CanonicalURL)
	if slug == "" {
		return p
	}
	p.EmbedURL = "https://clips.twitch.tv/embed?clip=" + url.QueryEscape(slug)
	p.ThumbnailURL = ""
	p.PreviewStatus = "pending"
	p.Error = ""
	return p
}

func twitchClipSlug(canonicalURL string) string {
	if match := twitchSlugRe.FindStringSubmatch(canonicalURL); len(match) == 2 {
		return match[1]
	}
	if match := twitchClipPathRe.FindStringSubmatch(canonicalURL); len(match) == 2 {
		return match[1]
	}
	return ""
}

func (h *Hydrator) fetchOEmbedWithRetry(ctx context.Context, p store.EvidencePreview, endpoint string) (store.EvidencePreview, bool) {
	var last store.EvidencePreview
	for attempt := 0; attempt < maxHydrateTries; attempt++ {
		if attempt > 0 {
			if err := sleepBackoff(ctx, attempt); err != nil {
				return last, false
			}
		}
		hydrated, ok := h.fetchOEmbed(ctx, p, endpoint)
		last = hydrated
		if ok {
			return hydrated, true
		}
		if !isTransientHydrationError(hydrated.HTTPStatus, hydrated.Error) {
			break
		}
	}
	return last, false
}

func (h *Hydrator) fetchOpenGraphWithRetry(ctx context.Context, p store.EvidencePreview) (store.EvidencePreview, bool) {
	var last store.EvidencePreview
	for attempt := 0; attempt < maxHydrateTries; attempt++ {
		if attempt > 0 {
			if err := sleepBackoff(ctx, attempt); err != nil {
				return last, false
			}
		}
		hydrated, ok := h.fetchOpenGraph(ctx, p)
		last = hydrated
		if ok {
			return hydrated, true
		}
		if !isTransientHydrationError(hydrated.HTTPStatus, hydrated.Error) {
			break
		}
	}
	return last, false
}

func sleepBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt) * 400 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransientHydrationError(status int, errMsg string) bool {
	if status == 0 && strings.TrimSpace(errMsg) != "" {
		return true
	}
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(errMsg))
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "tls handshake")
}

func (h *Hydrator) enrichRedditPreview(ctx context.Context, p *store.EvidencePreview, postURL string) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.ThumbnailURL) == "" {
		if clip := twitchClipSlug(postURL); clip != "" {
			p.ThumbnailURL = "https://clips-media-assets2.twitch.tv/" + clip + "-preview-480x272.jpg"
		}
	}
	if strings.TrimSpace(p.ThumbnailURL) == "" {
		if meta, ok := reddit.FetchPostMeta(ctx, h.client, "streamclone/1.0", postURL); ok {
			if meta.Thumbnail != "" {
				p.ThumbnailURL = meta.Thumbnail
			} else if clip := twitchClipSlug(meta.ExternalURL); clip != "" {
				p.ThumbnailURL = "https://clips-media-assets2.twitch.tv/" + clip + "-preview-480x272.jpg"
			}
		}
	}
	p.PreviewStatus = previewStatus(*p)
	p.ExpiresAt = previewExpiresAt(*p)
	nextFetchAt := p.ExpiresAt
	p.NextFetchAt = &nextFetchAt
}

func (h *Hydrator) fetchOEmbed(ctx context.Context, p store.EvidencePreview, endpoint string) (store.EvidencePreview, bool) {
	var body struct {
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		ProviderName string `json:"provider_name"`
		ThumbnailURL string `json:"thumbnail_url"`
		HTML         string `json:"html"`
	}
	status, data, err := h.get(ctx, endpoint)
	p.HTTPStatus = status
	if err != nil {
		p.Error = err.Error()
		return p, false
	}
	if err := json.Unmarshal(data, &body); err != nil {
		p.Error = "invalid_oembed"
		return p, false
	}
	p.Title = strings.TrimSpace(body.Title)
	p.Author = strings.TrimSpace(body.AuthorName)
	p.ProviderName = firstNonEmpty(body.ProviderName, p.ProviderName)
	p.ThumbnailURL = strings.TrimSpace(body.ThumbnailURL)
	if isAllowedEmbedProvider(p.Platform, p.ProviderName) {
		p.EmbedHTML = sanitizeEmbedHTML(body.HTML, p.Platform)
	}
	p.Error = ""
	p.PreviewStatus = previewStatus(p)
	p.ExpiresAt = previewExpiresAt(p)
	nextFetchAt := p.ExpiresAt
	p.NextFetchAt = &nextFetchAt
	return p, true
}

func (h *Hydrator) fetchOpenGraph(ctx context.Context, p store.EvidencePreview) (store.EvidencePreview, bool) {
	status, data, err := h.get(ctx, p.CanonicalURL)
	p.HTTPStatus = status
	if err != nil {
		p.Error = err.Error()
		return p, false
	}
	matches := metaRe.FindAllStringSubmatch(string(data), -1)
	for _, match := range matches {
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(html.UnescapeString(match[2]))
		switch key {
		case "og:title", "twitter:title":
			if p.Title == "" {
				p.Title = value
			}
		case "og:image", "twitter:image":
			if p.ThumbnailURL == "" {
				p.ThumbnailURL = value
			}
		case "og:site_name":
			if p.ProviderName == "" {
				p.ProviderName = value
			}
		}
	}
	if p.Title == "" {
		if match := titleRe.FindStringSubmatch(string(data)); len(match) == 2 {
			p.Title = strings.TrimSpace(html.UnescapeString(match[1]))
		}
	}
	p.Error = ""
	p.PreviewStatus = previewStatus(p)
	p.ExpiresAt = previewExpiresAt(p)
	nextFetchAt := p.ExpiresAt
	p.NextFetchAt = &nextFetchAt
	return p, p.PreviewStatus != "fallback"
}

func (h *Hydrator) get(ctx context.Context, rawURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", "Streamclone-PulseWire/1.0")
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, data, http.ErrNotSupported
	}
	return resp.StatusCode, data, nil
}

func sanitizeEmbedHTML(raw, platform string) string {
	var out strings.Builder
	tokenizer := nethtml.NewTokenizer(strings.NewReader(raw))
	skipDepth := 0
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case nethtml.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return strings.TrimSpace(out.String())
			}
			return ""
		case nethtml.TextToken:
			if skipDepth > 0 {
				continue
			}
			out.WriteString(html.EscapeString(string(tokenizer.Text())))
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			token := tokenizer.Token()
			tag := strings.ToLower(token.Data)
			if tag == "script" || tag == "style" {
				skipDepth++
				continue
			}
			if skipDepth > 0 {
				continue
			}
			if !isAllowedEmbedTag(tag) {
				continue
			}
			attrs := sanitizeEmbedAttrs(platform, tag, token.Attr)
			out.WriteByte('<')
			out.WriteString(tag)
			for _, attr := range attrs {
				out.WriteByte(' ')
				out.WriteString(attr.Key)
				out.WriteString(`="`)
				out.WriteString(html.EscapeString(attr.Val))
				out.WriteByte('"')
			}
			if tokenType == nethtml.SelfClosingTagToken {
				out.WriteString("/>")
			} else {
				out.WriteByte('>')
			}
		case nethtml.EndTagToken:
			tag := strings.ToLower(tokenizer.Token().Data)
			if tag == "script" || tag == "style" {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if skipDepth > 0 {
				continue
			}
			if isAllowedEmbedTag(tag) {
				out.WriteString("</")
				out.WriteString(tag)
				out.WriteByte('>')
			}
		}
	}
}

func isAllowedEmbedTag(tag string) bool {
	switch tag {
	case "a", "blockquote", "br", "div", "iframe", "p", "span":
		return true
	default:
		return false
	}
}

func sanitizeEmbedAttrs(platform, tag string, attrs []nethtml.Attribute) []nethtml.Attribute {
	out := make([]nethtml.Attribute, 0, len(attrs))
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		value := strings.TrimSpace(attr.Val)
		if key == "" || value == "" || len(value) > 1024 || strings.HasPrefix(key, "on") {
			continue
		}
		if key == "href" || key == "src" || key == "cite" {
			if !embedURLAllowed(value, platform) {
				continue
			}
			out = append(out, nethtml.Attribute{Key: key, Val: value})
			continue
		}
		if tag == "iframe" && isAllowedIframeAttr(key) {
			out = append(out, nethtml.Attribute{Key: key, Val: value})
			continue
		}
		if key == "class" || strings.HasPrefix(key, "data-") {
			out = append(out, nethtml.Attribute{Key: key, Val: value})
		}
	}
	return out
}

func isAllowedIframeAttr(key string) bool {
	switch key {
	case "allow", "allowfullscreen", "frameborder", "height", "loading", "referrerpolicy", "scrolling", "title", "width":
		return true
	default:
		return false
	}
}

func iframeSrcAllowed(fragment, platform string) bool {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(fragment))
	for {
		switch tokenizer.Next() {
		case nethtml.ErrorToken:
			return false
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			token := tokenizer.Token()
			if strings.ToLower(token.Data) != "iframe" {
				continue
			}
			for _, attr := range token.Attr {
				if strings.EqualFold(attr.Key, "src") {
					return embedURLAllowed(attr.Val, platform)
				}
			}
			return false
		}
	}
}

func embedURLAllowed(raw, platform string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch platform {
	case evidenceurl.PlatformReddit:
		return hostMatches(host, "reddit.com") || hostMatches(host, "redditmedia.com")
	case evidenceurl.PlatformTikTok:
		return hostMatches(host, "tiktok.com")
	case evidenceurl.PlatformX:
		return hostMatches(host, "twitter.com") || hostMatches(host, "x.com")
	default:
		return false
	}
}

func SourceTypeForPlatform(platform string) string {
	switch platform {
	case evidenceurl.PlatformReddit:
		return "reddit_thread"
	case evidenceurl.PlatformYouTube:
		return "youtube_video"
	case evidenceurl.PlatformTwitchClip:
		return "twitch_clip"
	case evidenceurl.PlatformX:
		return "x_post"
	case evidenceurl.PlatformTikTok:
		return "tiktok_video"
	case "streamerbans":
		return "streamerbans_post"
	default:
		return "manual_curation"
	}
}

func providerName(platform string) string {
	switch platform {
	case evidenceurl.PlatformKick:
		return "Kick"
	case evidenceurl.PlatformReddit:
		return "Reddit"
	case evidenceurl.PlatformTikTok:
		return "TikTok"
	case evidenceurl.PlatformTwitchClip:
		return "Twitch"
	case evidenceurl.PlatformX:
		return "X"
	case evidenceurl.PlatformYouTube:
		return "YouTube"
	default:
		return "Web"
	}
}

func isAllowedEmbedProvider(platform, providerName string) bool {
	provider := strings.ToLower(strings.TrimSpace(providerName))
	switch platform {
	case evidenceurl.PlatformReddit:
		return provider == "reddit" || provider == "reddit.com"
	case evidenceurl.PlatformTikTok:
		return provider == "tiktok"
	case evidenceurl.PlatformX:
		return provider == "x" || provider == "twitter"
	default:
		return false
	}
}

func hostMatches(host, root string) bool {
	return host == root || strings.HasSuffix(host, "."+root)
}

func previewStatus(p store.EvidencePreview) string {
	if p.Error != "" {
		return "error"
	}
	if p.EmbedURL != "" || p.EmbedHTML != "" || p.ThumbnailURL != "" || p.Title != "" {
		return "ready"
	}
	return "fallback"
}

func previewExpiresAt(p store.EvidencePreview) time.Time {
	switch previewStatus(p) {
	case "ready":
		return time.Now().Add(readyPreviewTTL)
	case "error", "fallback":
		return time.Now().Add(errorRetryAfter)
	default:
		return time.Now().Add(readyPreviewTTL)
	}
}

func youtubeID(raw string) string {
	if match := youtubeIDRe.FindStringSubmatch(raw); len(match) == 2 {
		return match[1]
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeRedditOEmbedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Hostname()), "reddit.com") {
		return raw
	}
	parsed.Scheme = "https"
	parsed.Host = "www.reddit.com"
	return parsed.String()
}
