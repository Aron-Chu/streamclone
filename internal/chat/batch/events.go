package batch

import (
	"encoding/json"
	"sync"
	"time"
)

type ChatEvent struct {
	Kind         string `json:"kind"`
	TargetLogin  string `json:"targetLogin,omitempty"`
	ActorLogin   string `json:"actorLogin,omitempty"`
	DurationSec  int    `json:"durationSec,omitempty"`
	Reason       string `json:"reason,omitempty"`
	MessageID    string `json:"messageId,omitempty"`
	TextPreview  string `json:"textPreview,omitempty"`
	NoticeMsgID  string `json:"noticeMsgId,omitempty"`
	DisplayText  string `json:"displayText,omitempty"`
	TS           int64  `json:"ts"`
	SummaryText  string `json:"summaryText,omitempty"`
}

type EventFrame struct {
	Type         string      `json:"type"`
	Channel      string      `json:"channel"`
	ServerSentTS int64       `json:"server_sent_ts"`
	Events       []ChatEvent `json:"events"`
}

type EventFlusher func(channel string, frame []byte)

type EventBatcher struct {
	mu      sync.Mutex
	pending map[string][]ChatEvent
	window  time.Duration
	flusher EventFlusher
	timers  map[string]*time.Timer
}

func NewEventBatcher(windowMS int, flusher EventFlusher) *EventBatcher {
	return &EventBatcher{
		pending: make(map[string][]ChatEvent),
		window:  time.Duration(windowMS) * time.Millisecond,
		flusher: flusher,
		timers:  make(map[string]*time.Timer),
	}
}

func (b *EventBatcher) Add(channel string, ev ChatEvent) {
	var immediate []ChatEvent
	b.mu.Lock()
	if _, exists := b.timers[channel]; !exists {
		immediate = []ChatEvent{ev}
		t := time.AfterFunc(b.window, func() {
			b.flush(channel)
		})
		b.timers[channel] = t
	} else {
		b.pending[channel] = append(b.pending[channel], ev)
	}
	b.mu.Unlock()

	if len(immediate) > 0 {
		b.emit(channel, immediate)
	}
}

func (b *EventBatcher) flush(channel string) {
	b.mu.Lock()
	events := b.pending[channel]
	delete(b.pending, channel)
	delete(b.timers, channel)
	b.mu.Unlock()
	if len(events) == 0 {
		return
	}
	b.emit(channel, events)
}

func (b *EventBatcher) emit(channel string, events []ChatEvent) {
	frame := EventFrame{
		Type:         "events",
		Channel:      channel,
		ServerSentTS: time.Now().UnixMilli(),
		Events:       events,
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	b.flusher(channel, data)
}
