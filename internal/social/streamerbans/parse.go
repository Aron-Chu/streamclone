package streamerbans

import (
	"fmt"
	"regexp"
	"strings"
)

var banPartnerRe = regexp.MustCompile(`(?i)Twitch Partner "([^"]+)"(?:\s*\(@([^)]+)\))?[^<\n]{0,120}has been banned`)

// ParseBan extracts twitch login and display name from a StreamerBans announcement.
func ParseBan(text string) (login, display string, ok bool) {
	text = strings.TrimSpace(text)
	m := banPartnerRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return "", "", false
	}
	display = strings.TrimSpace(m[1])
	login = normalizeLogin(display)
	if len(m) >= 3 && strings.TrimSpace(m[2]) != "" {
		login = normalizeLogin(m[2])
	}
	if login == "" {
		return "", "", false
	}
	if display == "" {
		display = login
	}
	return login, display, true
}

// ParseBanPost extracts login and a wire headline from ban announcement text.
func ParseBanPost(text string) (login, headline string, ok bool) {
	login, display, ok := ParseBan(text)
	if !ok {
		return "", "", false
	}
	return login, banHeadline(login, display), true
}

func banHeadline(login, display string) string {
	if strings.TrimSpace(display) == "" {
		display = login
	}
	return fmt.Sprintf("Twitch Partner \"%s\" has been banned", display)
}

func userBanURL(login string) string {
	return "https://streamerbans.com/user/" + login
}

func normalizeLogin(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "@")
	var out strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}
