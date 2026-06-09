package ircconn

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/coder/websocket"

	chatauth "streamclone/internal/chat/auth"
)

var ErrUnauthorized = errors.New("unauthorized")

type SessionProvider interface {
	Session(ctx context.Context, id string) (chatauth.Session, error)
}

type senderSocket interface {
	Write(context.Context, websocket.MessageType, []byte) error
	Read(context.Context) (websocket.MessageType, []byte, error)
	CloseNow() error
}

type dialSenderFunc func(context.Context, string) (senderSocket, error)

type SenderManager struct {
	mu       sync.Mutex
	ircURL   string
	sessions SessionProvider
	log      *slog.Logger
	dial     dialSenderFunc
	senders  map[string]*authSender
}

type authSender struct {
	mu      sync.Mutex
	socket  senderSocket
	login   string
	joined  map[string]struct{}
	closed  bool
	onClose func()
}

func NewSenderManager(ircURL string, sessions SessionProvider, log *slog.Logger) *SenderManager {
	return &SenderManager{
		ircURL:   ircURL,
		sessions: sessions,
		log:      log,
		dial:     websocketDialSender,
		senders:  make(map[string]*authSender),
	}
}

func (m *SenderManager) Send(ctx context.Context, sessionID, channel, text string) error {
	session, err := m.sessions.Session(ctx, sessionID)
	if err != nil {
		return ErrUnauthorized
	}
	if session.Login == "" || session.AccessToken == "" {
		return ErrUnauthorized
	}

	s, err := m.sender(ctx, session)
	if err != nil {
		return err
	}
	if err := s.Send(ctx, channel, text); err != nil {
		m.drop(session.ID)
		return err
	}
	return nil
}

func (m *SenderManager) sender(ctx context.Context, session chatauth.Session) (*authSender, error) {
	m.mu.Lock()
	if s := m.senders[session.ID]; s != nil && !s.closed {
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()

	socket, err := m.dial(ctx, m.ircURL)
	if err != nil {
		return nil, err
	}
	s, err := newAuthSender(ctx, socket, session.Login, session.AccessToken)
	if err != nil {
		socket.CloseNow()
		return nil, err
	}
	s.onClose = func() { m.drop(session.ID) }

	m.mu.Lock()
	if old := m.senders[session.ID]; old != nil {
		old.Close()
	}
	m.senders[session.ID] = s
	m.mu.Unlock()

	go s.readLoop(ctx, m.log)
	return s, nil
}

func (m *SenderManager) drop(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.senders, sessionID)
}

func websocketDialSender(ctx context.Context, ircURL string) (senderSocket, error) {
	ws, _, err := websocket.Dial(ctx, ircURL, nil)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

func newAuthSender(ctx context.Context, socket senderSocket, login, accessToken string) (*authSender, error) {
	s := &authSender{
		socket: socket,
		login:  strings.ToLower(login),
		joined: make(map[string]struct{}),
	}
	for _, line := range []string{
		"PASS oauth:" + strings.TrimPrefix(accessToken, "oauth:"),
		"NICK " + s.login,
		"CAP REQ :twitch.tv/tags twitch.tv/commands",
	} {
		if err := s.writeLine(ctx, line); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *authSender) Send(ctx context.Context, channel, text string) error {
	channel = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(channel), "#"))
	text = cleanMessage(text)
	if channel == "" || text == "" {
		return errors.New("empty channel or message")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("sender closed")
	}
	if _, ok := s.joined[channel]; !ok {
		if err := s.writeLineLocked(ctx, "JOIN #"+channel); err != nil {
			return err
		}
		s.joined[channel] = struct{}{}
	}
	return s.writeLineLocked(ctx, "PRIVMSG #"+channel+" :"+text)
}

func (s *authSender) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.socket.CloseNow()
}

func (s *authSender) readLoop(ctx context.Context, log *slog.Logger) {
	defer func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if s.onClose != nil {
			s.onClose()
		}
	}()
	for {
		_, data, err := s.socket.Read(ctx)
		if err != nil {
			return
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\r\n"), "\r\n") {
			if strings.HasPrefix(line, "PING") {
				if err := s.writeLine(ctx, "PONG"+strings.TrimPrefix(line, "PING")); err != nil && log != nil {
					log.Warn("authenticated irc pong failed", "err", err)
				}
			}
		}
	}
}

func (s *authSender) writeLine(ctx context.Context, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLineLocked(ctx, line)
}

func (s *authSender) writeLineLocked(ctx context.Context, line string) error {
	if s.closed {
		return errors.New("sender closed")
	}
	return s.socket.Write(ctx, websocket.MessageText, []byte(line+"\r\n"))
}

func cleanMessage(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}
