package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/metadata/gql"
)

var channelLoginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,24}$`)

type FollowStore interface {
	List(ctx context.Context) ([]string, error)
	Add(ctx context.Context, login string) error
	Remove(ctx context.Context, login string) error
}

type FollowedChannel struct {
	ID           string `json:"id"`
	Login        string `json:"login"`
	DisplayName  string `json:"displayName"`
	ProfileImage string `json:"profileImage,omitempty"`
	IsLive       bool   `json:"isLive"`
	Title        string `json:"title,omitempty"`
	Category     string `json:"category,omitempty"`
	Viewers      int    `json:"viewers,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

func (h *Handler) WithFollowStore(store FollowStore) *Handler {
	h.follows = store
	return h
}

func validChannelLogin(login string) bool {
	return channelLoginRe.MatchString(login)
}

func (h *Handler) followedList(w http.ResponseWriter, r *http.Request) {
	if h.follows == nil {
		writeJSON(w, http.StatusOK, map[string]any{"channels": []FollowedChannel{}})
		return
	}
	logins, err := h.follows.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "follow_list_failed"})
		return
	}
	channels := make([]FollowedChannel, 0, len(logins))
	for _, login := range logins {
		channels = append(channels, h.enrichFollowedChannel(r.Context(), login))
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (h *Handler) followChannel(w http.ResponseWriter, r *http.Request) {
	if h.follows == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "follow_store_unavailable"})
		return
	}
	login := normalizeLogin(chi.URLParam(r, "login"))
	if !validChannelLogin(login) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if err := h.follows.Add(r.Context(), login); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "follow_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": login, "following": true})
}

func (h *Handler) unfollowChannel(w http.ResponseWriter, r *http.Request) {
	if h.follows == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "follow_store_unavailable"})
		return
	}
	login := normalizeLogin(chi.URLParam(r, "login"))
	if !validChannelLogin(login) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if err := h.follows.Remove(r.Context(), login); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_following"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unfollow_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": login, "following": false})
}

func (h *Handler) enrichFollowedChannel(ctx context.Context, login string) FollowedChannel {
	out := FollowedChannel{
		ID:          login,
		Login:       login,
		DisplayName: login,
	}
	if h.g == nil {
		return out
	}
	ch, err := h.g.Channel(ctx, login)
	if err != nil {
		return out
	}
	return followedFromGQL(login, ch)
}

// followedFromGQL keeps the locally stored login canonical; GQL only enriches.
func followedFromGQL(login string, ch gql.Channel) FollowedChannel {
	display := strings.TrimSpace(ch.DisplayName)
	if display == "" {
		display = login
	}
	id := strings.TrimSpace(ch.ID)
	if id == "" {
		id = login
	}
	return FollowedChannel{
		ID:           id,
		Login:        login,
		DisplayName:  display,
		ProfileImage: ch.ProfileImage,
		IsLive:       ch.IsLive,
		Title:        ch.StreamTitle,
		Category:     ch.Category,
		Viewers:      ch.Viewers,
		ThumbnailURL: ch.ThumbnailURL,
	}
}
