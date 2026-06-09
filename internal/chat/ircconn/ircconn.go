package ircconn

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type LineHandler func(line string)

type conn struct {
	ws       *websocket.Conn
	channels map[string]struct{}
}

type Manager struct {
	mu            sync.Mutex
	ircURL        string
	maxPerSocket  int
	conns         []*conn
	channelToConn map[string]*conn
	handler       LineHandler
	subscriptions map[string]int
	log           *slog.Logger
}

func NewManager(ircURL string, maxPerSocket int, handler LineHandler, log *slog.Logger) *Manager {
	return &Manager{
		ircURL:        ircURL,
		maxPerSocket:  maxPerSocket,
		conns:         nil,
		channelToConn: make(map[string]*conn),
		handler:       handler,
		subscriptions: make(map[string]int),
		log:           log,
	}
}

func (m *Manager) Join(ctx context.Context, channel string) {
	m.mu.Lock()
	m.subscriptions[channel]++
	if m.subscriptions[channel] > 1 {
		m.mu.Unlock()
		return
	}
	c := m.pickConn()
	if c != nil {
		c.channels[channel] = struct{}{}
		m.channelToConn[channel] = c
		m.mu.Unlock()
		m.sendLine(ctx, c, "JOIN #"+channel)
		return
	}
	m.mu.Unlock()

	c = m.dial(ctx)
	if c == nil {
		m.mu.Lock()
		m.subscriptions[channel]--
		if m.subscriptions[channel] <= 0 {
			delete(m.subscriptions, channel)
		}
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	m.conns = append(m.conns, c)
	c.channels[channel] = struct{}{}
	m.channelToConn[channel] = c
	m.mu.Unlock()
	m.sendLine(ctx, c, "JOIN #"+channel)
}

func (m *Manager) Part(ctx context.Context, channel string) {
	m.mu.Lock()
	m.subscriptions[channel]--
	if m.subscriptions[channel] > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.subscriptions, channel)
	c := m.channelToConn[channel]
	if c != nil {
		delete(c.channels, channel)
		delete(m.channelToConn, channel)
	}
	m.mu.Unlock()
	if c != nil {
		m.sendLine(ctx, c, "PART #"+channel)
	}
}

func (m *Manager) pickConn() *conn {
	for _, c := range m.conns {
		if len(c.channels) < m.maxPerSocket {
			return c
		}
	}
	return nil
}

func (m *Manager) dial(ctx context.Context) *conn {
	var ws *websocket.Conn
	var err error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 8; attempt++ {
		ws, _, err = websocket.Dial(ctx, m.ircURL, nil)
		if err == nil {
			break
		}
		m.log.Warn("irc dial failed", "err", err, "attempt", attempt)
		jitter := time.Duration(rand.Int63n(int64(backoff)))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff + jitter):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	if err != nil {
		m.log.Error("irc dial gave up", "err", err)
		return nil
	}

	nick := fmt.Sprintf("justinfan%d", 10000+rand.Intn(80000))
	for _, msg := range []string{
		"PASS SCHMOOPIIE",
		"NICK " + nick,
		"CAP REQ :twitch.tv/tags twitch.tv/commands",
	} {
		if err := ws.Write(ctx, websocket.MessageText, []byte(msg+"\r\n")); err != nil {
			m.log.Error("irc handshake failed", "err", err)
			ws.CloseNow()
			return nil
		}
	}

	c := &conn{ws: ws, channels: make(map[string]struct{})}
	go m.readLoop(ctx, c)
	return c
}

func (m *Manager) sendLine(ctx context.Context, c *conn, line string) {
	if err := c.ws.Write(ctx, websocket.MessageText, []byte(line+"\r\n")); err != nil {
		m.log.Warn("irc send failed", "err", err, "line", line)
	}
}

func (m *Manager) readLoop(ctx context.Context, c *conn) {
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			m.log.Warn("irc read error", "err", err)
			m.reconnect(ctx, c)
			return
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\r\n"), "\r\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "PING") {
				pong := "PONG" + strings.TrimPrefix(line, "PING")
				_ = c.ws.Write(ctx, websocket.MessageText, []byte(pong+"\r\n"))
				continue
			}
			m.handler(line)
		}
	}
}

func (m *Manager) reconnect(ctx context.Context, dead *conn) {
	m.mu.Lock()
	channels := make([]string, 0, len(dead.channels))
	for ch := range dead.channels {
		channels = append(channels, ch)
	}
	for i, c := range m.conns {
		if c == dead {
			m.conns = append(m.conns[:i], m.conns[i+1:]...)
			break
		}
	}
	for ch := range dead.channels {
		delete(m.channelToConn, ch)
	}
	m.mu.Unlock()

	var active []string
	m.mu.Lock()
	for _, ch := range channels {
		if m.subscriptions[ch] > 0 {
			active = append(active, ch)
		}
	}
	m.mu.Unlock()

	if len(active) == 0 {
		return
	}

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff + time.Duration(rand.Int63n(int64(backoff)))):
		}

		m.mu.Lock()
		still := make([]string, 0, len(active))
		for _, ch := range active {
			if m.subscriptions[ch] > 0 {
				still = append(still, ch)
			}
		}
		m.mu.Unlock()

		if len(still) == 0 {
			return
		}

		c := m.dial(ctx)
		if c == nil {
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}

		m.mu.Lock()
		m.conns = append(m.conns, c)
		for _, ch := range still {
			if m.subscriptions[ch] > 0 {
				c.channels[ch] = struct{}{}
				m.channelToConn[ch] = c
			}
		}
		rejoin := make([]string, 0, len(c.channels))
		for ch := range c.channels {
			rejoin = append(rejoin, ch)
		}
		m.mu.Unlock()

		for _, ch := range rejoin {
			m.sendLine(ctx, c, "JOIN #"+ch)
		}
		return
	}
}
