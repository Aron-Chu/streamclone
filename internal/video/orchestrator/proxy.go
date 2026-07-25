package orchestrator

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Approved Twitch playlist hosts for the HLS proxy path:
// usher.ttvnw.net and any subdomain of ttvnw.net (e.g. video-weaver.*.hls.ttvnw.net).
// Both http and https are accepted because Twitch usher historically returns
// http playlist URLs that immediately redirect to https weaver hosts.

const (
	maxProxyRedirects     = 5
	maxProxyPlaylistBytes = 2 << 20 // 2 MiB — playlists are small text manifests
)

func allowedProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme")
	}
	if u.User != nil {
		return nil, fmt.Errorf("url userinfo is not allowed")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing url host")
	}
	host := u.Hostname()
	if !isTwitchUsherHost(host) {
		return nil, fmt.Errorf("host is not allowed")
	}
	port := u.Port()
	if port != "" {
		switch u.Scheme {
		case "http":
			if port != "80" {
				return nil, fmt.Errorf("unexpected port")
			}
		case "https":
			if port != "443" {
				return nil, fmt.Errorf("unexpected port")
			}
		}
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

// isBlockedProxyIP rejects destinations that must never be reached by the playlist proxy.
func isBlockedProxyIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	} else if ip.To16() != nil && ip.To4() == nil {
		// IPv4-mapped IPv6 (:ffff:x.x.x.x) — evaluate the embedded v4.
		if v4 := extractIPv4Mapped(ip); v4 != nil {
			return isBlockedProxyIP(v4)
		}
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	// Documentation / benchmarking / CGNAT / reserved ranges not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 already unspecified; 100.64.0.0/10 CGNAT; 192.0.0.0/24 IETF;
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 documentation;
		// 198.18.0.0/15 benchmarking; 240.0.0.0/4 reserved.
		if ip4[0] == 0 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 0 && (ip4[2] == 0 || ip4[2] == 2) {
			return true
		}
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return true
		}
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return true
		}
		if ip4[0] == 198 && ip4[1] >= 18 && ip4[1] <= 19 {
			return true
		}
		if ip4[0] >= 240 {
			return true
		}
	} else {
		// IPv6 documentation 2001:db8::/32 and unique-local already partially via IsPrivate (fc00::/7).
		if len(ip) == net.IPv6len {
			if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
				return true
			}
		}
	}
	return false
}

func extractIPv4Mapped(ip net.IP) net.IP {
	if len(ip) != net.IPv6len {
		return nil
	}
	// ::ffff:0:0/96
	for i := 0; i < 10; i++ {
		if ip[i] != 0 {
			return nil
		}
	}
	if ip[10] != 0xff || ip[11] != 0xff {
		return nil
	}
	return net.IPv4(ip[12], ip[13], ip[14], ip[15])
}
