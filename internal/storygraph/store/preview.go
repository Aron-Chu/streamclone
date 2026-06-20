package store

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	clipURLPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)clips\.twitch\.tv/([^/?#]+)`),
		regexp.MustCompile(`(?i)/clip/([^/?#]+)`),
	}
	redditPostIDRE = regexp.MustCompile(`(?i)^1[a-z0-9]{4,}$`)
	redditThumbHosts = map[string]struct{}{
		"preview.redd.it":            {},
		"external-preview.redd.it":   {},
		"i.redd.it":                  {},
		"b.thumbs.redditmedia.com":   {},
		"a.thumbs.redditmedia.com":   {},
		"styles.redditmedia.com":     {},
	}
)

// twitchClipPreviewURL derives a Twitch clip CDN preview only from a URL containing a clip slug.
func twitchClipPreviewURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	for _, re := range clipURLPatterns {
		if m := re.FindStringSubmatch(rawURL); len(m) > 1 {
			slug := strings.TrimSpace(m[1])
			if slug != "" && isLikelyTwitchClipSlug(slug) {
				return "https://clips-media-assets2.twitch.tv/" + slug + "-preview-480x272.jpg"
			}
		}
	}
	return ""
}

func twitchClipPreviewFromSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	if !isLikelyTwitchClipSlug(slug) {
		return ""
	}
	return "https://clips-media-assets2.twitch.tv/" + slug + "-preview-480x272.jpg"
}

func isLikelyTwitchClipSlug(slug string) bool {
	if slug == "" || redditPostIDRE.MatchString(slug) {
		return false
	}
	return len(slug) >= 10
}

func isRedditThumbURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if _, ok := redditThumbHosts[host]; ok {
		return true
	}
	return strings.HasSuffix(host, ".redd.it")
}

func isProxiedThumbHost(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if isRedditThumbURL(raw) {
		return true
	}
	switch host {
	case "clips-media-assets2.twitch.tv", "clips-media-assets.twitch.tv", "static-cdn.jtvnw.net":
		return true
	default:
		return false
	}
}

func proxiedThumbPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !isProxiedThumbHost(raw) {
		return ""
	}
	return "/v1/pulse-wire/thumb?u=" + url.QueryEscape(raw)
}

// preferServingThumb picks the thumbnail most likely to load through the proxy.
// Helix often stores static-cdn.jtvnw.net URLs while evidence previews may only
// have clips-media-assets URLs that upstream rejects with 403.
func preferServingThumb(candidates ...string) string {
	var jtvnw, reddit, other, clipCDN string
	for _, raw := range candidates {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		lower := strings.ToLower(candidate)
		switch {
		case strings.Contains(lower, "static-cdn.jtvnw.net"):
			jtvnw = candidate
		case isRedditThumbURL(candidate):
			reddit = candidate
		case strings.Contains(lower, "clips-media-assets"):
			if clipCDN == "" {
				clipCDN = candidate
			}
		default:
			if other == "" {
				other = candidate
			}
		}
	}
	if jtvnw != "" {
		return jtvnw
	}
	if reddit != "" {
		return reddit
	}
	if other != "" {
		return other
	}
	return clipCDN
}

func classifyThumb(candidate string) (kind, rawURL, proxied string) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "none", "", ""
	}
	if isRedditThumbURL(candidate) {
		return "reddit", candidate, proxiedThumbPath(candidate)
	}
	if clip := twitchClipPreviewURL(candidate); clip != "" {
		return "twitch", clip, proxiedThumbPath(clip)
	}
	if strings.HasPrefix(strings.ToLower(candidate), "https://") && isProxiedThumbHost(candidate) {
		kind := "fallback"
		if strings.Contains(candidate, "jtvnw.net") {
			kind = "twitch"
		}
		return kind, candidate, proxiedThumbPath(candidate)
	}
	return "none", "", ""
}

// resolvePreview picks preview kind, raw image URL, and same-origin proxy path.
func resolvePreview(storedThumb, metricsThumb, text, postURL string) (kind, rawURL, proxied string) {
	if kind, rawURL, proxied = classifyThumb(preferServingThumb(storedThumb, metricsThumb)); kind != "none" {
		return kind, rawURL, proxied
	}
	for _, candidate := range []string{text, postURL} {
		if clip := twitchClipPreviewURL(candidate); clip != "" {
			return "twitch", clip, proxiedThumbPath(clip)
		}
	}
	return "none", "", ""
}

func communityPreviewKind(previewStatus, storedThumb, metricsThumb, text, postURL string) (kind, rawURL, proxied string) {
	if kind, rawURL, proxied = resolvePreview(storedThumb, metricsThumb, text, postURL); kind != "none" {
		return kind, rawURL, proxied
	}
	switch strings.ToLower(strings.TrimSpace(previewStatus)) {
	case "ready":
		if storedThumb != "" {
			return classifyThumb(storedThumb)
		}
		return "none", "", ""
	case "fallback":
		return "none", "", ""
	default:
		return "none", "", ""
	}
}
