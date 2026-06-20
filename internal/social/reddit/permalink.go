package reddit

import (
	"net/url"
	"strings"
)

// CanonicalPermalink normalizes Reddit thread URLs for storage and UI (always www.reddit.com).
// Server-side fetch helpers may still target old.reddit.com separately.
func CanonicalPermalink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/r/") || strings.HasPrefix(raw, "/comments/") {
		raw = "https://www.reddit.com" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case strings.Contains(host, "reddit.com"):
		parsed.Scheme = "https"
		parsed.Host = "www.reddit.com"
	case host == "redd.it":
		id := strings.Trim(strings.Trim(parsed.Path, "/"), "/")
		if id == "" {
			return raw
		}
		parsed.Scheme = "https"
		parsed.Host = "www.reddit.com"
		parsed.Path = "/comments/" + id
	default:
		return raw
	}
	parsed.Fragment = ""
	if strings.Contains(parsed.Path, "/comments/") {
		parsed.RawQuery = ""
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if path == "" {
		return parsed.String()
	}
	return parsed.Scheme + "://" + parsed.Host + path + "/"
}
