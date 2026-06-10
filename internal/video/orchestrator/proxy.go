package orchestrator

import (
	"fmt"
	"net/url"
	"strings"
)

func allowedProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing url host")
	}
	if !isTwitchUsherHost(u.Hostname()) {
		return nil, fmt.Errorf("host %q is not allowed", u.Hostname())
	}
	return u, nil
}

func isTwitchUsherHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if host == "usher.ttvnw.net" {
		return true
	}
	return strings.HasSuffix(host, ".ttvnw.net")
}
