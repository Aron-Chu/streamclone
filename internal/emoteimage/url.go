package emoteimage

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	twitchCDNTemplate  = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/2.0"
	sevenTVCDNTemplate = "https://cdn.7tv.app/emote/%s/4x.webp"
	ffzCDNTemplate     = "https://cdn.frankerfacez.com/emoticon/%s/4"
	bttvCDNTemplate    = "https://cdn.betterttv.net/emote/%s/3x"
)

var localEmoteID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsLocalEmoteID reports whether id is a synced emote-service UUID.
func IsLocalEmoteID(id string) bool {
	return localEmoteID.MatchString(strings.TrimSpace(id))
}

// LocalPath returns the emote-service proxy path for a synced emote id.
func LocalPath(id, scale string) string {
	if scale == "" {
		scale = "1x"
	}
	return fmt.Sprintf("/emotes/%s/%s.webp", id, scale)
}

// URL resolves a rollup/chat emote id to a browser-loadable image URL.
// Rollup keys use provider:id:name; id is usually a local emote-service UUID for
// third-party sets, but Twitch native ids are Twitch CDN keys (numeric or emotesv2_*).
func URL(provider, id, scale string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if scale == "" {
		scale = "1x"
	}
	provider = strings.ToLower(strings.TrimSpace(provider))

	var out string
	switch provider {
	case "twitch":
		out = fmt.Sprintf(twitchCDNTemplate, id)
	case "seventv", "7tv":
		if localEmoteID.MatchString(id) {
			out = LocalPath(id, scale)
		} else {
			out = fmt.Sprintf(sevenTVCDNTemplate, id)
		}
	case "ffz", "frankerfacez":
		if localEmoteID.MatchString(id) {
			out = LocalPath(id, scale)
		} else {
			out = fmt.Sprintf(ffzCDNTemplate, id)
		}
	case "bttv", "betterttv":
		if localEmoteID.MatchString(id) {
			out = LocalPath(id, scale)
		} else {
			out = fmt.Sprintf(bttvCDNTemplate, id)
		}
	default:
		out = LocalPath(id, scale)
	}

	return out
}

// AbsolutizeHostedCDN rewrites relative /emotes/ paths and loopback absolute URLs to a public CDN base.
// When cdnBase is empty, url is returned unchanged (local dev).
func AbsolutizeHostedCDN(cdnBase, url string) string {
	cdnBase = strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	url = strings.TrimSpace(url)
	if url == "" || cdnBase == "" {
		return url
	}
	if strings.HasPrefix(url, "/emotes/") {
		if strings.HasSuffix(cdnBase, "/emotes") {
			return cdnBase + strings.TrimPrefix(url, "/emotes")
		}
		return cdnBase + url
	}
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "http://localhost:") || strings.HasPrefix(lower, "http://127.0.0.1:") {
		if idx := strings.Index(url, "/emotes/"); idx >= 0 {
			suffix := url[idx:]
			if strings.HasSuffix(cdnBase, "/emotes") {
				return cdnBase + strings.TrimPrefix(suffix, "/emotes")
			}
			return cdnBase + suffix
		}
	}
	return url
}

// HostedBrowserURL resolves an emote for hosted clients (extension + portal).
func HostedBrowserURL(cdnBase, provider, localID, providerEmoteID string) string {
	return AbsolutizeHostedCDN(cdnBase, ExtensionBrowserURL(provider, localID, providerEmoteID))
}

// ExtensionBrowserURL returns an HTTPS URL suitable for the Pulse Chrome extension on
// twitch.tv. Synced 7TV emotes use local UUID ids in rollups; when providerEmoteID is
// known, prefer the public 7TV CDN so the extension never proxies localhost images.
func ExtensionBrowserURL(provider, localID, providerEmoteID string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerEmoteID = strings.TrimSpace(providerEmoteID)
	if (provider == "seventv" || provider == "7tv") && providerEmoteID != "" {
		return fmt.Sprintf(sevenTVCDNTemplate, providerEmoteID)
	}
	if (provider == "ffz" || provider == "frankerfacez") && providerEmoteID != "" {
		return fmt.Sprintf(ffzCDNTemplate, providerEmoteID)
	}
	if (provider == "bttv" || provider == "betterttv") && providerEmoteID != "" {
		return fmt.Sprintf(bttvCDNTemplate, providerEmoteID)
	}
	return URL(provider, localID, "1x")
}
