package fingerprint

import (
	"encoding/json"
)

// EmoteCount is a counted emote in a moment window.
type EmoteCount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Count    int    `json:"count"`
}

// MomentFingerprint is the origin moment signal bundle (Phase 1: text/emote/timing).
type MomentFingerprint struct {
	EntityID   *int64
	StreamID   string
	VODOffsetS int
	Quotes     []string
	TopEmotes  []EmoteCount
	Game       string
	Version    int
}

// QuotesJSON returns transcript keywords as JSON for storage.
func (m MomentFingerprint) QuotesJSON() json.RawMessage {
	b, _ := json.Marshal(m.Quotes)
	return b
}

// TopEmotesJSON returns top emotes as JSON for storage.
func (m MomentFingerprint) TopEmotesJSON() json.RawMessage {
	b, _ := json.Marshal(m.TopEmotes)
	return b
}
