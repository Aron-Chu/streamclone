package chatreplay

import "time"

// VODChatMessage is a single persisted VOD chat message. It mirrors the
// analytics_vod_chat_messages table (migration 000012). The ID field is the
// BIGSERIAL primary key; MessageID stores the Twitch-provided comment id used
// for the UNIQUE (stream_id, message_id) dedupe key.
type VODChatMessage struct {
	ID            int64       `json:"id"`
	StreamID      string      `json:"streamId"`
	MinuteTS      time.Time   `json:"minuteTs"`
	MessageID     string      `json:"messageId"`
	DisplayName   string      `json:"displayName"`
	SenderHash    string      `json:"senderHash"`
	Text          string      `json:"text"`
	EmoteFrags    []EmoteFrag `json:"emoteFrags,omitempty"`
	OffsetSeconds int         `json:"offsetSeconds"`
	SyncedAt      time.Time   `json:"syncedAt"`
}

// EmoteFrag is a single emote fragment within a stored chat message. The
// ImageURL is the local emote-service URL of the form /emotes/{id}/1x.webp.
type EmoteFrag struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
	ImageURL string `json:"imageUrl"`
}
