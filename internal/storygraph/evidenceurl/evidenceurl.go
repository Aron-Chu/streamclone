package evidenceurl

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	PlatformGeneric    = "web"
	PlatformKick       = "kick"
	PlatformReddit     = "reddit"
	PlatformTikTok     = "tiktok"
	PlatformTwitchClip = "twitch_clip"
	PlatformX          = "x"
	PlatformYouTube    = "youtube"
)

var (
	urlRe        = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
	trailingJunk = regexp.MustCompile(`[.,;!?)\"'\]]+$`)
)

// Link is a canonical source URL extracted from social evidence.
type Link struct {
	RawURL       string
	CanonicalURL string
	Platform     string
}

// Extract finds and canonicalizes URLs in text.
func Extract(text string) []Link {
	seen := map[string]struct{}{}
	var out []Link
	for _, raw := range urlRe.FindAllString(text, -1) {
		link, ok := Canonicalize(raw)
		if !ok {
			continue
		}
		if _, exists := seen[link.CanonicalURL]; exists {
			continue
		}
		seen[link.CanonicalURL] = struct{}{}
		out = append(out, link)
	}
	return out
}

// Canonicalize normalizes known source URLs enough for de-duplication.
func Canonicalize(raw string) (Link, bool) {
	raw = strings.TrimSpace(raw)
	raw = trailingJunk.ReplaceAllString(raw, "")
	if raw == "" {
		return Link{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return Link{}, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Link{}, false
	}
	u.Scheme = "https"
	host := strings.ToLower(u.Hostname())
	if host == "" || !strings.Contains(host, ".") {
		return Link{}, false
	}
	u.Host = host
	u.Fragment = ""
	platform := PlatformGeneric

	switch {
	case host == "m.youtube.com" || host == "music.youtube.com":
		host = "www.youtube.com"
		u.Host = host
		platform = PlatformYouTube
		if strings.HasPrefix(u.Path, "/watch") {
			v := u.Query().Get("v")
			u.RawQuery = ""
			if v != "" {
				u.RawQuery = "v=" + url.QueryEscape(v)
			} else {
				return Link{}, false
			}
		} else if strings.HasPrefix(u.Path, "/shorts/") {
			if id := pathPart(u.Path, 1); id != "" {
				u.Path = "/watch"
				u.RawQuery = "v=" + url.QueryEscape(id)
			} else {
				return Link{}, false
			}
		} else {
			u.RawQuery = ""
		}
	case host == "youtu.be":
		id := firstPathPart(u.Path)
		if id == "" {
			return Link{}, false
		}
		u.Host = "www.youtube.com"
		u.Path = "/watch"
		u.RawQuery = "v=" + url.QueryEscape(id)
		platform = PlatformYouTube
	case hostMatches(host, "youtube.com"):
		platform = PlatformYouTube
		if strings.HasPrefix(u.Path, "/shorts/") {
			if id := pathPart(u.Path, 1); id != "" {
				u.Path = "/watch"
				u.RawQuery = "v=" + url.QueryEscape(id)
			} else {
				return Link{}, false
			}
		} else if u.Path == "/watch" {
			v := u.Query().Get("v")
			u.RawQuery = ""
			if v != "" {
				u.RawQuery = "v=" + url.QueryEscape(v)
			} else {
				return Link{}, false
			}
		} else {
			u.RawQuery = ""
		}
		u.Host = "www.youtube.com"
	case hostMatches(host, "tiktok.com"):
		platform = PlatformTikTok
		u.Host = "www.tiktok.com"
		u.RawQuery = ""
	case host == "twitter.com" || host == "mobile.twitter.com" || host == "x.com" || host == "mobile.x.com":
		platform = PlatformX
		u.Host = "x.com"
		u.RawQuery = ""
		u.Path = strings.TrimRight(u.Path, "/")
	case host == "clips.twitch.tv":
		platform = PlatformTwitchClip
		u.RawQuery = ""
	case hostMatches(host, "twitch.tv") && strings.Contains(u.Path, "/clip/"):
		platform = PlatformTwitchClip
		if slug := clipSlug(u.Path); slug != "" {
			u.Host = "clips.twitch.tv"
			u.Path = "/" + slug
		} else {
			return Link{}, false
		}
		u.RawQuery = ""
	case host == "redd.it":
		platform = PlatformReddit
		id := firstPathPart(u.Path)
		if id == "" {
			return Link{}, false
		}
		u.Host = "www.reddit.com"
		u.Path = "/comments/" + id
		u.RawQuery = ""
	case hostMatches(host, "reddit.com"):
		platform = PlatformReddit
		u.Host = "www.reddit.com"
		u.RawQuery = ""
		if strings.HasPrefix(u.Path, "/u/") || strings.HasPrefix(u.Path, "/user/") {
			return Link{}, false
		}
	case hostMatches(host, "kick.com"):
		platform = PlatformKick
		u.RawQuery = ""
	default:
		u.RawQuery = ""
	}

	return Link{RawURL: raw, CanonicalURL: u.String(), Platform: platform}, true
}

// Attachable reports whether a canonical link should become an Evidence Gallery card.
// CDN thumbnails and other non-page assets are skipped.
func Attachable(link Link) bool {
	if link.Platform != PlatformGeneric {
		return true
	}
	host := strings.ToLower(strings.TrimSpace(mustHost(link.CanonicalURL)))
	if host == "" {
		return false
	}
	if hostMatches(host, "jtvnw.net") {
		return false
	}
	if host == "clips-media-assets2.twitch.tv" {
		return false
	}
	if strings.Contains(link.CanonicalURL, "/thumb/") || strings.Contains(link.CanonicalURL, "-preview-") {
		return false
	}
	return true
}

func mustHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func hostMatches(host, root string) bool {
	return host == root || strings.HasSuffix(host, "."+root)
}

func firstPathPart(path string) string {
	return pathPart(path, 0)
}

func pathPart(path string, idx int) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if idx < 0 || idx >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[idx])
}

func clipSlug(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "clip" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
