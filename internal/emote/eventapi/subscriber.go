package eventapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/seeder"
	"streamclone/internal/emote/store"
)

const (
	opSubscribe   = 35
	opUnsubscribe = 36
	opDispatch    = 0

	defaultIdleTTL    = 30 * time.Minute
	defaultReapPeriod = time.Minute
)

type Subscriber struct {
	st           *store.Store
	seed         *seeder.Seeder
	syncer       *seeder.SevenTVSyncer
	d            *dict.Dict
	log          *slog.Logger
	url          string
	idleTTL      time.Duration
	reapInterval time.Duration
	mu           sync.Mutex
	subs         map[string]subscription
	conn         *websocket.Conn
	cancel       context.CancelFunc
	enabled      bool
}

type subscription struct {
	login         string
	twitchID      string
	setID         string
	providerSetID string
	lastActive    time.Time
}

type wireMessage struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type subscribePayload struct {
	Type      string            `json:"type"`
	Condition map[string]string `json:"condition"`
}

type dispatchPayload struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

func Enabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("SEVENTV_EVENTAPI_ENABLED")), "true")
}

func DefaultURL() string {
	if v := strings.TrimSpace(os.Getenv("SEVENTV_EVENTAPI_URL")); v != "" {
		return v
	}
	return "wss://events.7tv.io/v3"
}

func New(st *store.Store, seed *seeder.Seeder, d *dict.Dict, log *slog.Logger) *Subscriber {
	return &Subscriber{
		st:           st,
		seed:         seed,
		syncer:       seeder.NewSevenTVSyncer(st, seed, d),
		d:            d,
		log:          log,
		url:          DefaultURL(),
		idleTTL:      defaultIdleTTL,
		reapInterval: defaultReapPeriod,
		subs:         make(map[string]subscription),
		enabled:      Enabled(),
	}
}

func (s *Subscriber) Start(ctx context.Context) {
	if !s.enabled {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.run(ctx)
	go s.reapLoop(ctx)
}

func (s *Subscriber) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Subscriber) Register(ctx context.Context, login, twitchID, providerSetID string) error {
	if !s.enabled || providerSetID == "" {
		return nil
	}
	login = strings.ToLower(strings.TrimSpace(login))
	channel, err := s.st.GetChannelByLogin(ctx, login)
	if err != nil {
		return err
	}
	setID := ""
	if channel.ActiveEmoteSetID != nil {
		setID = *channel.ActiveEmoteSetID
	}
	s.mu.Lock()
	existing, already := s.subs[login]
	s.subs[login] = subscription{
		login:         login,
		twitchID:      twitchID,
		setID:         setID,
		providerSetID: providerSetID,
		lastActive:    time.Now(),
	}
	s.mu.Unlock()
	// Already subscribed to the same provider set: just refresh activity, no resend.
	if already && existing.providerSetID == providerSetID {
		return nil
	}
	return s.sendSubscribe(providerSetID)
}

func (s *Subscriber) Unregister(login string) {
	if !s.enabled {
		return
	}
	login = strings.ToLower(strings.TrimSpace(login))
	s.mu.Lock()
	sub, ok := s.subs[login]
	delete(s.subs, login)
	s.mu.Unlock()
	if ok {
		_ = s.sendUnsubscribe(sub.providerSetID)
	}
}

func (s *Subscriber) run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.connectAndServe(ctx); err != nil && s.log != nil {
			s.log.Warn("7tv eventapi disconnected", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *Subscriber) connectAndServe(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, s.url, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.conn = conn
	active := make([]subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		active = append(active, sub)
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.conn = nil
		s.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "shutdown")
	}()

	for _, sub := range active {
		if err := s.writeSubscribe(conn, sub.providerSetID); err != nil {
			return err
		}
	}

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		setID, ok := parseDispatchSetID(data)
		if !ok {
			continue
		}
		s.handleSetUpdate(ctx, setID)
	}
}

// parseDispatchSetID extracts the affected emote-set object id from a raw 7TV
// EventAPI frame, returning ok=false for anything that is not an
// emote_set.update dispatch carrying a usable id. It is pure so message parsing
// can be unit tested without a live websocket.
func parseDispatchSetID(data []byte) (string, bool) {
	var msg wireMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", false
	}
	if msg.Op != opDispatch {
		return "", false
	}
	var payload dispatchPayload
	if err := json.Unmarshal(msg.D, &payload); err != nil {
		return "", false
	}
	if payload.Type != "emote_set.update" {
		return "", false
	}
	var envelope struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload.Body, &envelope); err != nil {
		return "", false
	}
	id := strings.TrimSpace(envelope.ID)
	return id, id != ""
}

func (s *Subscriber) handleSetUpdate(ctx context.Context, setID string) {
	if setID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	targets := make([]subscription, 0)
	for login, sub := range s.subs {
		if sub.providerSetID == setID {
			sub.lastActive = now
			s.subs[login] = sub
			targets = append(targets, sub)
		}
	}
	s.mu.Unlock()
	for _, sub := range targets {
		if sub.setID == "" {
			continue
		}
		if err := s.syncer.ApplyChannelSet(ctx, sub.login, sub.twitchID, sub.setID); err != nil && s.log != nil {
			s.log.Warn("7tv event apply failed", "login", sub.login, "set_id", setID, "err", err)
		} else if s.log != nil {
			s.log.Info("7tv event applied", "login", sub.login, "set_id", setID)
		}
	}
}

// reapLoop periodically drops subscriptions for channels that have not been
// ensured or received an event within idleTTL, bounding the number of live
// 7TV EventAPI subscriptions. Poll-based ensure re-subscribes on next view.
func (s *Subscriber) reapLoop(ctx context.Context) {
	if s.reapInterval <= 0 {
		return
	}
	ticker := time.NewTicker(s.reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reapIdle()
		}
	}
}

func (s *Subscriber) reapIdle() {
	if s.idleTTL <= 0 {
		return
	}
	cutoff := time.Now().Add(-s.idleTTL)
	s.mu.Lock()
	stale := make([]subscription, 0)
	for login, sub := range s.subs {
		if sub.lastActive.Before(cutoff) {
			stale = append(stale, sub)
			delete(s.subs, login)
		}
	}
	s.mu.Unlock()
	for _, sub := range stale {
		_ = s.sendUnsubscribe(sub.providerSetID)
		if s.log != nil {
			s.log.Info("7tv eventapi unsubscribed idle", "login", sub.login, "set_id", sub.providerSetID)
		}
	}
}

func (s *Subscriber) sendSubscribe(providerSetID string) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return s.writeSubscribe(conn, providerSetID)
}

func (s *Subscriber) sendUnsubscribe(providerSetID string) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return s.writeUnsubscribe(conn, providerSetID)
}

func (s *Subscriber) writeSubscribe(conn *websocket.Conn, providerSetID string) error {
	payload := wireMessage{
		Op: opSubscribe,
		D: mustJSON(subscribePayload{
			Type:      "emote_set.update",
			Condition: map[string]string{"object_id": providerSetID},
		}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, payloadBytes(payload))
}

func (s *Subscriber) writeUnsubscribe(conn *websocket.Conn, providerSetID string) error {
	payload := wireMessage{
		Op: opUnsubscribe,
		D: mustJSON(subscribePayload{
			Type:      "emote_set.update",
			Condition: map[string]string{"object_id": providerSetID},
		}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, payloadBytes(payload))
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func payloadBytes(msg wireMessage) []byte {
	raw, _ := json.Marshal(msg)
	return raw
}
