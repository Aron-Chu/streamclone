package chatreplay

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handler serves the VOD chat-replay HTTP endpoint backed by a Store. It is
// registered on the analytics service router (see cmd/analytics).
type Handler struct {
	store *Store
	log   *slog.Logger
}

// NewHandler constructs a chat-replay Handler over the given Store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store, log: slog.Default()}
}

// WithLogger sets the logger used for admin purge auditing and returns the
// handler for chaining. A nil logger leaves the default in place.
func (h *Handler) WithLogger(logger *slog.Logger) *Handler {
	if logger != nil {
		h.log = logger
	}
	return h
}

// Routes registers the chat-replay routes on the provided chi router. The path
// uses the {streamID} URL parameter to match the analytics service convention.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/v1/analytics/streams/{streamID}/chat-replay", h.chatReplay)
	r.Delete("/v1/analytics/streams/{streamID}/chat-messages", h.purgeChatMessages)
}

// replayResponse is the JSON envelope returned by the chat-replay endpoint.
// Messages is always a non-nil slice so the JSON body carries an empty array
// rather than null. Unavailable signals that the stream has no persisted chat
// messages at all (Requirement 27.6).
type replayResponse struct {
	Messages    []VODChatMessage `json:"messages"`
	NextCursor  string           `json:"nextCursor"`
	Unavailable bool             `json:"unavailable"`
}

// chatReplay handles GET /v1/analytics/streams/{streamID}/chat-replay. It
// returns a page of stored VOD chat messages ordered by offset ascending
// (Requirement 27.4), honoring a default page limit of 200 capped at 500
// (Requirement 27.5). When the stream has no persisted messages it returns
// HTTP 200 with an empty array and unavailable=true (Requirement 27.6).
func (h *Handler) chatReplay(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}

	q := r.URL.Query()
	params := QueryParams{
		StreamID:    streamID,
		OffsetStart: parseIntDefault(q.Get("offsetStart"), 0),
		OffsetEnd:   parseIntDefault(q.Get("offsetEnd"), 0),
		Limit:       parseIntDefault(q.Get("limit"), 0),
		Cursor:      strings.TrimSpace(q.Get("cursor")),
	}

	result, err := h.store.Query(r.Context(), params)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query_failed"})
		return
	}

	messages := result.Messages
	if messages == nil {
		messages = []VODChatMessage{}
	}

	unavailable := false
	if len(messages) == 0 {
		count, err := h.store.CountByStream(r.Context(), streamID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query_failed"})
			return
		}
		unavailable = count == 0
	}

	writeJSON(w, http.StatusOK, replayResponse{
		Messages:    messages,
		NextCursor:  result.NextCursor,
		Unavailable: unavailable,
	})
}

// purgeChatMessages handles DELETE /v1/analytics/streams/{streamID}/chat-messages.
// It is an admin purge that removes all persisted VOD chat messages for a stream
// (Requirements 30.4) and returns HTTP 204 No Content. For privacy it logs only
// the stream id and purged row count, never message content (Requirement 30.3).
func (h *Handler) purgeChatMessages(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}

	purged, err := h.store.DeleteByStream(r.Context(), streamID)
	if err != nil {
		h.log.Warn("vod chat admin purge failed", "streamId", streamID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "purge_failed"})
		return
	}

	h.log.Info("vod chat admin purge", "streamId", streamID, "purged", purged)
	w.WriteHeader(http.StatusNoContent)
}

// parseIntDefault parses a base-10 integer, returning def for empty or invalid
// input so malformed query parameters degrade gracefully to defaults.
func parseIntDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
