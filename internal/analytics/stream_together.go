package analytics

import (
	"regexp"
	"strings"
)

var streamTogetherTitleRe = regexp.MustCompile(`(?i)streaming together with ([A-Za-z0-9_]+)`)

// StreamTogetherInfo is derived from Helix stream title/tags (no separate Twitch API).
type StreamTogetherInfo struct {
	Together    bool
	PartnerHint string // login parsed from title when present
}

func detectStreamTogether(title string, tags []string) StreamTogetherInfo {
	info := StreamTogetherInfo{}
	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if strings.Contains(t, "streamingtogether") ||
			strings.Contains(t, "co-stream") ||
			strings.Contains(t, "costream") ||
			t == "coop" {
			info.Together = true
		}
	}
	lowerTitle := strings.ToLower(strings.TrimSpace(title))
	if strings.Contains(lowerTitle, "streaming together") {
		info.Together = true
	}
	if m := streamTogetherTitleRe.FindStringSubmatch(title); len(m) > 1 {
		info.Together = true
		info.PartnerHint = normalizeLogin(m[1])
	}
	return info
}

func resolveStreamTogetherHubFields(
	login string,
	title string,
	tags []string,
	rosterByLogin map[string]Top500Current,
) (together bool, hostLogin string, togetherWith []string) {
	info := detectStreamTogether(title, tags)
	if !info.Together {
		return false, "", nil
	}
	together = true
	self := normalizeLogin(login)
	if info.PartnerHint != "" && info.PartnerHint != self {
		togetherWith = []string{info.PartnerHint}
		if host, ok := rosterByLogin[info.PartnerHint]; ok && host.IsLive {
			hostLogin = info.PartnerHint
		} else {
			hostLogin = info.PartnerHint
		}
		return together, hostLogin, togetherWith
	}
	return together, "", togetherWith
}

func categoryForTogetherStream(
	login string,
	categoryName string,
	title string,
	tags []string,
	rosterByLogin map[string]Top500Current,
) string {
	category := strings.TrimSpace(categoryName)
	together, hostLogin, _ := resolveStreamTogetherHubFields(login, title, tags, rosterByLogin)
	if !together || hostLogin == "" {
		return category
	}
	if host, ok := rosterByLogin[hostLogin]; ok {
		if hostCat := strings.TrimSpace(host.CategoryName); hostCat != "" {
			return hostCat
		}
	}
	return category
}
