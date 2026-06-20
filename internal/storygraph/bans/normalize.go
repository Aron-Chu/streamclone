package bans

import (
	"context"
	"regexp"
	"strings"
	"time"

	"streamclone/internal/social"
	"streamclone/internal/storygraph/cluster"
	"streamclone/internal/storygraph/store"
)

var (
	unbanTitleRE       = regexp.MustCompile(`(?i)\bunban(?:ned|s)?\b`)
	banTitleRE         = regexp.MustCompile(`(?i)\b(?:ban(?:ned|s)?|suspended|suspension|permaban)\b`)
	suspendedTitleRE   = regexp.MustCompile(`(?i)\bsuspend(?:ed|sion)?\b`)
	twitchBanContextRE = regexp.MustCompile(`(?i)\b(?:on|from)\s+twitch\b|twitch partner`)
)

// UpsertFromSocialItem records a structured ban event when the item qualifies.
func UpsertFromSocialItem(ctx context.Context, st *store.Store, sourceName string, socialID int64, item social.Item) error {
	if st == nil || socialID <= 0 {
		return nil
	}
	login := strings.TrimSpace(item.EntityTwitchLogin)
	headline := evidenceHeadline(item.Text)
	if headline == "" {
		return nil
	}
	eventType, confidence, ok := classifyBanEvent(sourceName, headline, item.FlairText)
	if !ok || login == "" {
		return nil
	}
	occurred := item.CreatedAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	display := strings.TrimSpace(item.EntityDisplayName)
	return st.UpsertBanEvent(ctx, store.BanEvent{
		StreamerLogin: login,
		DisplayName:   display,
		EventType:     eventType,
		Platform:      "twitch",
		Source:        banSourceName(sourceName),
		SourceItemID:  &socialID,
		Headline:      headline,
		SourceURL:     strings.TrimSpace(item.URL),
		OccurredAt:    occurred,
		Confidence:    confidence,
	})
}

func classifyBanEvent(sourceName, headline, flair string) (eventType string, confidence float64, ok bool) {
	if sourceName == "streamerbans" || sourceName == "streamerbans_post" {
		eventType = "banned"
		if unbanTitleRE.MatchString(headline) {
			eventType = "unbanned"
		} else if suspendedTitleRE.MatchString(headline) {
			eventType = "suspended"
		}
		return eventType, 0.72, true
	}
	if cluster.ClassifyCategory(headline, flair) != "bans" && !(banTitleRE.MatchString(headline) && twitchBanContextRE.MatchString(headline)) {
		return "", 0, false
	}
	eventType = "banned"
	if unbanTitleRE.MatchString(headline) {
		eventType = "unbanned"
	} else if suspendedTitleRE.MatchString(headline) {
		eventType = "suspended"
	} else if !banTitleRE.MatchString(headline) {
		return "", 0, false
	}
	return eventType, 0.55, true
}

func banSourceName(sourceName string) string {
	switch sourceName {
	case "streamerbans", "streamerbans_post":
		return "streamerbans_post"
	case "reddit":
		return "reddit"
	default:
		return sourceName
	}
}

func evidenceHeadline(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "https://"); idx > 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "http://"); idx > 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return strings.Join(strings.Fields(raw), " ")
}
