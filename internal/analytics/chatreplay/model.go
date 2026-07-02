package chatreplay

import "time"

// VODChatMessage is a single persisted VOD chat message. It mirrors the
// analytics_vod_chat_messages table (migration 000012). The ID field is the
// BIGSERIAL primary key; MessageID stores the Twitch-provided comment id used
// for the UNIQUE (stream_id, message_id) dedupe key.
type VODChatMessage struct {
	ID             int64       `json:"id"`
	StreamID       string      `json:"streamId"`
	MinuteTS       time.Time   `json:"minuteTs"`
	MessageID      string      `json:"messageId"`
	DisplayName    string      `json:"displayName"`
	CommenterLogin string      `json:"commenterLogin,omitempty"`
	SenderHash     string      `json:"senderHash"`
	Text           string      `json:"text"`
	EmoteFrags     []EmoteFrag `json:"emoteFrags,omitempty"`
	OffsetSeconds  int         `json:"offsetSeconds"`
	SyncedAt       time.Time   `json:"syncedAt"`
}

// ChannelChatLogStream summarizes VOD chat coverage for one synced stream.
type ChannelChatLogStream struct {
	StreamID     string     `json:"streamId"`
	Title        string     `json:"title,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
	MessageCount int64      `json:"messageCount"`
	FirstOffset  int        `json:"firstOffsetSeconds"`
	LastOffset   int        `json:"lastOffsetSeconds"`
	Source       string     `json:"source"`
}

// LiveChatMessage is a persisted live chat line from the chat service ingest path.
type LiveChatMessage struct {
	ID          int64       `json:"id"`
	Channel     string      `json:"channel"`
	Login       string      `json:"login,omitempty"`
	DisplayName string      `json:"displayName"`
	MessageID   string      `json:"messageId"`
	Text        string      `json:"text"`
	Fragments   []EmoteFrag `json:"fragments,omitempty"`
	TS          time.Time   `json:"ts"`
	SyncedAt    time.Time   `json:"syncedAt"`
}

// ChatModEvent is a persisted moderation or notice event.
type ChatModEvent struct {
	ID          int64     `json:"id"`
	Channel     string    `json:"channel"`
	Kind        string    `json:"kind"`
	ActorLogin  string    `json:"actorLogin,omitempty"`
	TargetLogin string    `json:"targetLogin,omitempty"`
	DurationSec int       `json:"durationSec,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	MessageID   string    `json:"messageId,omitempty"`
	TextPreview string    `json:"textPreview,omitempty"`
	TS          time.Time `json:"ts"`
	SyncedAt    time.Time `json:"syncedAt"`
}

// UnifiedLogEntry is a merged timeline row for the logs viewer.
type UnifiedLogEntry struct {
	Kind            string      `json:"kind"`
	ID              int64       `json:"id"`
	TS              time.Time   `json:"ts"`
	OffsetSeconds   int         `json:"offsetSeconds,omitempty"`
	DisplayName     string      `json:"displayName,omitempty"`
	Login           string      `json:"login,omitempty"`
	SenderHash      string      `json:"senderHash,omitempty"`
	MessageID       string      `json:"messageId,omitempty"`
	Text            string      `json:"text,omitempty"`
	EmoteFrags      []EmoteFrag `json:"emoteFrags,omitempty"`
	ModKind         string      `json:"modKind,omitempty"`
	ModText         string      `json:"modText,omitempty"`
	Source          string      `json:"source"`
	StreamID        string      `json:"streamId,omitempty"`
	StreamTitle     string      `json:"streamTitle,omitempty"`
	StreamStartedAt *time.Time  `json:"streamStartedAt,omitempty"`
}

// EmoteFrag is a single emote fragment within a stored chat message. The
// ImageURL is the local emote-service URL of the form /emotes/{id}/1x.webp.
type EmoteFrag struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
	ImageURL string `json:"imageUrl"`
}
