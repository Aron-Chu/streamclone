package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"streamclone/internal/metrics"
)

type ChannelJoiner interface {
	Join(ctx context.Context, channel string)
	Part(ctx context.Context, channel string)
}

type Subscriber interface {
	Subscribe(ctx context.Context, channel string, handler func([]byte)) (func(), error)
}

type Authenticator interface {
	SessionIDFromRequest(r *http.Request) (string, bool)
}

type MessageSender interface {
	Send(ctx context.Context, sessionID, channel, text string) error
}

type controlFrame struct {
	Op           string `json:"op"`
	Channel      string `json:"channel"`
	Text         string `json:"text"`
	ClientMsgID  string `json:"client_msg_id"`
	ClientSentTS int64  `json:"client_sent_ts"`
}

var channelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,24}$`)

type client struct {
	ws       *websocket.Conn
	send     chan []byte
	channels map[string]struct{}
	session  string
}

type Hub struct {
	mu          sync.Mutex
	clients     map[*client]struct{}
	chanClients map[string]map[*client]struct{}
	unsubs      map[string]func()
	joiner      ChannelJoiner
	subscriber  Subscriber
	auth        Authenticator
	sender      MessageSender
	queueSize   int
	graceMS     int
	log         *slog.Logger
}

func New(joiner ChannelJoiner, subscriber Subscriber, queueSize, graceMS int, log *slog.Logger) *Hub {
	return &Hub{
		clients:     make(map[*client]struct{}),
		chanClients: make(map[string]map[*client]struct{}),
		unsubs:      make(map[string]func()),
		joiner:      joiner,
		subscriber:  subscriber,
		queueSize:   queueSize,
		graceMS:     graceMS,
		log:         log,
	}
}

func (h *Hub) WithAuth(auth Authenticator, sender MessageSender) *Hub {
	h.auth = auth
	h.sender = sender
	return h
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		h.log.Warn("ws accept failed", "err", err)
		return
	}
	var sessionID string
	if h.auth != nil {
		if id, ok := h.auth.SessionIDFromRequest(r); ok {
			sessionID = id
		}
	}
	c := &client{
		ws:       ws,
		send:     make(chan []byte, h.queueSize),
		channels: make(map[string]struct{}),
		session:  sessionID,
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	metrics.ChatConnections.Inc()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go h.writeLoop(ctx, c)
	h.readLoop(ctx, c)

	h.removeClient(ctx, c)
}

func (h *Hub) writeLoop(ctx context.Context, c *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

func (h *Hub) readLoop(ctx context.Context, c *client) {
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var frame controlFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}
		switch frame.Op {
		case "subscribe":
			h.subscribe(ctx, c, frame.Channel)
		case "unsubscribe":
			h.unsubscribe(ctx, c, frame.Channel)
		case "send_message":
			h.sendMessage(ctx, c, frame)
		default:
			h.emit(c, "error", frame.Channel, "unknown op")
		}
	}
}

func (h *Hub) subscribe(ctx context.Context, c *client, channel string) {
	if !channelRe.MatchString(channel) {
		h.emit(c, "error", channel, "invalid channel")
		return
	}
	h.mu.Lock()
	c.channels[channel] = struct{}{}
	if h.chanClients[channel] == nil {
		h.chanClients[channel] = make(map[*client]struct{})
	}
	first := len(h.chanClients[channel]) == 0
	h.chanClients[channel][c] = struct{}{}
	subscribers := len(h.chanClients[channel])
	h.mu.Unlock()
	metrics.ChatChannelSubscribers.WithLabelValues(channel).Set(float64(subscribers))

	if first {
		channelCtx := context.Background()
		h.joiner.Join(channelCtx, channel)
		unsub, err := h.subscriber.Subscribe(channelCtx, channel, func(data []byte) {
			h.broadcast(channel, data)
		})
		if err != nil {
			h.log.Warn("redis subscribe failed", "channel", channel, "err", err)
			h.emit(c, "error", channel, "subscribe failed")
			h.unsubscribe(context.Background(), c, channel)
			return
		}
		h.mu.Lock()
		if old, ok := h.unsubs[channel]; ok {
			old()
		}
		h.unsubs[channel] = unsub
		h.mu.Unlock()
	}

	h.emit(c, "status", channel, "subscribed")
}

func (h *Hub) unsubscribe(ctx context.Context, c *client, channel string) {
	if !channelRe.MatchString(channel) {
		h.emit(c, "error", channel, "invalid channel")
		return
	}
	h.mu.Lock()
	delete(c.channels, channel)
	if h.chanClients[channel] != nil {
		delete(h.chanClients[channel], c)
	}
	remaining := len(h.chanClients[channel])
	h.mu.Unlock()
	metrics.ChatChannelSubscribers.WithLabelValues(channel).Set(float64(remaining))

	if remaining == 0 {
		go func() {
			if h.graceMS > 0 {
				time.Sleep(time.Duration(h.graceMS) * time.Millisecond)
			}
			h.mu.Lock()
			still := len(h.chanClients[channel])
			h.mu.Unlock()
			if still == 0 {
				h.joiner.Part(context.Background(), channel)
				h.mu.Lock()
				if unsub, ok := h.unsubs[channel]; ok {
					unsub()
					delete(h.unsubs, channel)
				}
				delete(h.chanClients, channel)
				h.mu.Unlock()
				metrics.ChatChannelSubscribers.WithLabelValues(channel).Set(0)
			}
		}()
	}
	h.emit(c, "status", channel, "unsubscribed")
}

func (h *Hub) sendMessage(ctx context.Context, c *client, frame controlFrame) {
	channel := strings.ToLower(strings.TrimSpace(frame.Channel))
	text := strings.TrimSpace(frame.Text)
	if !channelRe.MatchString(channel) {
		metrics.ChatSendAttempts.WithLabelValues("invalid_channel").Inc()
		h.emitMessageError(c, frame, "invalid channel")
		return
	}
	if text == "" {
		metrics.ChatSendAttempts.WithLabelValues("empty").Inc()
		h.emitMessageError(c, frame, "message is empty")
		return
	}
	if len(text) > 500 {
		metrics.ChatSendAttempts.WithLabelValues("too_long").Inc()
		h.emitMessageError(c, frame, "message is too long")
		return
	}
	if h.sender == nil || c.session == "" {
		metrics.ChatSendAttempts.WithLabelValues("auth_required").Inc()
		h.emitMessageError(c, frame, "auth_required")
		return
	}
	h.emitMessageAck(c, frame, "queued")
	if err := h.sender.Send(ctx, c.session, channel, text); err != nil {
		h.log.Warn("chat send failed", "channel", channel, "err", err)
		metrics.ChatSendAttempts.WithLabelValues("failed").Inc()
		h.emitMessageError(c, frame, "send_failed")
		return
	}
	metrics.ChatSendAttempts.WithLabelValues("sent").Inc()
	h.emitMessageAck(c, frame, "sent")
}

func (h *Hub) removeClient(ctx context.Context, c *client) {
	h.mu.Lock()
	channels := make([]string, 0, len(c.channels))
	for ch := range c.channels {
		channels = append(channels, ch)
	}
	delete(h.clients, c)
	h.mu.Unlock()

	for _, ch := range channels {
		h.unsubscribe(ctx, c, ch)
	}
	close(c.send)
	c.ws.CloseNow()
	metrics.ChatConnections.Dec()
}

func (h *Hub) deliver(c *client, data []byte) {
	select {
	case c.send <- data:
		metrics.ChatMessagesOut.Inc()
	default:
		select {
		case <-c.send:
			metrics.ChatQueueDrops.Inc()
		default:
		}
		select {
		case c.send <- data:
			metrics.ChatMessagesOut.Inc()
		default:
		}
	}
}

func (h *Hub) broadcast(channel string, data []byte) {
	h.mu.Lock()
	targets := make([]*client, 0, len(h.chanClients[channel]))
	for c := range h.chanClients[channel] {
		targets = append(targets, c)
	}
	h.mu.Unlock()

	for _, c := range targets {
		h.deliver(c, data)
	}
}

func (h *Hub) Deliver(channel string, data []byte) {
	h.broadcast(channel, data)
}

func (h *Hub) emit(c *client, typ, channel, state string) {
	field := "state"
	if typ == "error" {
		field = "message"
	}
	resp, _ := json.Marshal(map[string]string{"type": typ, "channel": channel, field: state})
	h.deliver(c, resp)
}

func (h *Hub) emitMessageAck(c *client, frame controlFrame, state string) {
	h.emitJSON(c, map[string]any{
		"type":           "message_ack",
		"channel":        frame.Channel,
		"client_msg_id":  frame.ClientMsgID,
		"client_sent_ts": frame.ClientSentTS,
		"state":          state,
		"server_ts":      time.Now().UnixMilli(),
	})
}

func (h *Hub) emitMessageError(c *client, frame controlFrame, message string) {
	h.emitJSON(c, map[string]any{
		"type":           "message_error",
		"channel":        frame.Channel,
		"client_msg_id":  frame.ClientMsgID,
		"client_sent_ts": frame.ClientSentTS,
		"message":        message,
		"server_ts":      time.Now().UnixMilli(),
	})
}

func (h *Hub) emitJSON(c *client, frame any) {
	resp, _ := json.Marshal(frame)
	h.deliver(c, resp)
}
