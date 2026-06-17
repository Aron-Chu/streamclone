package chatreplay

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type ingestMessage struct {
	Channel     string          `json:"channel"`
	Login       string          `json:"login"`
	DisplayName string          `json:"displayName"`
	MessageID   string          `json:"messageId"`
	Text        string          `json:"text"`
	Fragments   json.RawMessage `json:"fragments"`
	TS          int64           `json:"ts"`
}

type ingestEvent struct {
	Kind        string `json:"kind"`
	Channel     string `json:"channel"`
	ActorLogin  string `json:"actorLogin"`
	TargetLogin string `json:"targetLogin"`
	DurationSec int    `json:"durationSec"`
	Reason      string `json:"reason"`
	MessageID   string `json:"messageId"`
	TextPreview string `json:"textPreview"`
	TS          int64  `json:"ts"`
}

type ingestRequest struct {
	Messages []ingestMessage `json:"messages"`
	Events   []ingestEvent   `json:"events"`
}

// IngestEnabled reports whether live chat persistence is enabled.
type IngestEnabled func() bool

func (h *Handler) ingestLiveChat(w http.ResponseWriter, r *http.Request) {
	if h.ingestEnabled != nil && !h.ingestEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest_disabled"})
		return
	}
	var body ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	ctx := r.Context()
	for _, msg := range body.Messages {
		if strings.TrimSpace(msg.Channel) == "" || strings.TrimSpace(msg.MessageID) == "" {
			continue
		}
		ts := time.Now().UTC()
		if msg.TS > 0 {
			ts = time.UnixMilli(msg.TS).UTC()
		}
		var frags []EmoteFrag
		if len(msg.Fragments) > 0 {
			_ = json.Unmarshal(msg.Fragments, &frags)
		}
		_ = h.store.InsertLiveMessage(ctx, LiveChatMessage{
			Channel:     msg.Channel,
			Login:       strings.ToLower(strings.TrimSpace(msg.Login)),
			DisplayName: msg.DisplayName,
			MessageID:   msg.MessageID,
			Text:        msg.Text,
			Fragments:   frags,
			TS:          ts,
		})
	}
	for _, ev := range body.Events {
		if strings.TrimSpace(ev.Channel) == "" || strings.TrimSpace(ev.Kind) == "" {
			continue
		}
		ts := time.Now().UTC()
		if ev.TS > 0 {
			ts = time.UnixMilli(ev.TS).UTC()
		}
		_ = h.store.InsertModEvent(ctx, ChatModEvent{
			Channel:     ev.Channel,
			Kind:        ev.Kind,
			ActorLogin:  ev.ActorLogin,
			TargetLogin: ev.TargetLogin,
			DurationSec: ev.DurationSec,
			Reason:      ev.Reason,
			MessageID:   ev.MessageID,
			TextPreview: ev.TextPreview,
			TS:          ts,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// PurgeLiveRetention removes live chat and mod events older than cutoff.
func (s *Store) PurgeLiveRetention(ctx context.Context, cutoff time.Time) (int64, error) {
	live, err := s.PurgeLiveOlderThan(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	mod, err := s.PurgeModEventsOlderThan(ctx, cutoff)
	if err != nil {
		return live, err
	}
	return live + mod, nil
}
