package api

import (
	"net/http"
	"strconv"
	"strings"

	"streamclone/internal/storygraph/store"
)

func (h *Handler) bans(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListBanEvents(r.Context(), since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func (h *Handler) unlinkedEvidence(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListUnlinkedEvidence(r.Context(), since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func banEventsToCommunityPosts(events []store.BanEvent) []store.CommunityPost {
	out := make([]store.CommunityPost, 0, len(events))
	for _, event := range events {
		postedAt := event.OccurredAt
		subreddit := "StreamerBans"
		if event.Source == "reddit" {
			if sub := redditSubredditFromURL(event.SourceURL); sub != "" {
				subreddit = sub
			} else {
				subreddit = "LivestreamFail"
			}
		}
		out = append(out, store.CommunityPost{
			ID:                  event.ID,
			Title:               event.Headline,
			URL:                 event.SourceURL,
			Permalink:           event.SourceURL,
			Source:              event.Source,
			Subreddit:           subreddit,
			Category:            "bans",
			PreviewKind:         event.PreviewKind,
			PreviewURL:          event.PreviewURL,
			ThumbnailURL:        event.ThumbnailURL,
			StreamerLogin:       event.StreamerLogin,
			StreamerDisplayName: event.DisplayName,
			PostedAt:            &postedAt,
		})
	}
	return out
}

func redditSubredditFromURL(raw string) string {
	lower := strings.ToLower(raw)
	const marker = "/r/"
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(marker):]
	if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
		rest = rest[:cut]
	}
	return strings.TrimSpace(rest)
}
