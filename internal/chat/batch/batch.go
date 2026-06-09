package batch

import (
	"encoding/json"
	"sync"
	"time"
)

type Fragment struct {
	T        string `json:"t"`
	C        string `json:"c"`
	U        string `json:"u,omitempty"`
	Zw       bool   `json:"zw,omitempty"`
	ID       string `json:"id,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type BatchMessage struct {
	ID               string     `json:"id"`
	User             string     `json:"user"`
	Color            string     `json:"color"`
	Badges           []string   `json:"badges"`
	TS               int64      `json:"ts"`
	ServerReceivedTS int64      `json:"server_received_ts,omitempty"`
	Fragments        []Fragment `json:"fragments"`
}

type Frame struct {
	Type         string         `json:"type"`
	Channel      string         `json:"channel"`
	ServerSentTS int64          `json:"server_sent_ts"`
	Messages     []BatchMessage `json:"messages"`
}

type Flusher func(channel string, frame []byte)

type Batcher struct {
	mu      sync.Mutex
	pending map[string][]BatchMessage
	window  time.Duration
	flusher Flusher
	timers  map[string]*time.Timer
}

func New(windowMS int, flusher Flusher) *Batcher {
	return &Batcher{
		pending: make(map[string][]BatchMessage),
		window:  time.Duration(windowMS) * time.Millisecond,
		flusher: flusher,
		timers:  make(map[string]*time.Timer),
	}
}

func (b *Batcher) Add(channel string, msg BatchMessage) {
	if msg.ServerReceivedTS == 0 {
		msg.ServerReceivedTS = time.Now().UnixMilli()
	}

	var immediate []BatchMessage
	b.mu.Lock()
	if _, exists := b.timers[channel]; !exists {
		immediate = []BatchMessage{msg}
		t := time.AfterFunc(b.window, func() {
			b.flush(channel)
		})
		b.timers[channel] = t
	} else {
		b.pending[channel] = append(b.pending[channel], msg)
	}
	b.mu.Unlock()

	if len(immediate) > 0 {
		b.emit(channel, immediate)
	}
}

func (b *Batcher) flush(channel string) {
	b.mu.Lock()
	msgs := b.pending[channel]
	delete(b.pending, channel)
	delete(b.timers, channel)
	b.mu.Unlock()

	if len(msgs) == 0 {
		return
	}

	b.emit(channel, msgs)
}

func (b *Batcher) emit(channel string, msgs []BatchMessage) {
	frame := Frame{
		Type:         "batch",
		Channel:      channel,
		ServerSentTS: time.Now().UnixMilli(),
		Messages:     msgs,
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	b.flusher(channel, data)
}
