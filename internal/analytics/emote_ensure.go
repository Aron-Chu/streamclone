package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics/netmeter"
	"streamclone/internal/chat/enrich"
)

const (
	defaultLiveEnsureCooldown      = 10 * time.Minute
	defaultLiveEnsureMaxConcurrent = 4
	defaultEnsurePollInterval      = 2 * time.Second
	defaultEnsureWaitTimeout       = 2 * time.Minute
	defaultEnsureHTTPTimeout       = 20 * time.Second
)

// EmoteSyncState reports live Pulse emote dictionary readiness for the extension BFF.
type EmoteSyncState string

const (
	EmoteSyncReady         EmoteSyncState = "ready"
	EmoteSyncSyncing       EmoteSyncState = "syncing"
	EmoteSyncStale         EmoteSyncState = "stale"
	EmoteSyncUnavailable   EmoteSyncState = "unavailable"
	EmoteSyncAggregateOnly EmoteSyncState = "aggregate_only"
)

// EmoteSyncSnapshot is returned on extension pulse payloads.
type EmoteSyncSnapshot struct {
	State          EmoteSyncState `json:"state"`
	Provider       string         `json:"provider"`
	LastSyncedAt   *time.Time     `json:"lastSyncedAt,omitempty"`
	EventAPIActive bool           `json:"eventApiActive"`
	Source         string         `json:"source,omitempty"`
	Message        string         `json:"message,omitempty"`
}

type emoteEnsureResponse struct {
	State     string `json:"state"`
	Count     int    `json:"count"`
	Pending   int    `json:"pending"`
	Providers []struct {
		Provider string `json:"provider"`
		State    string `json:"state"`
		Count    int    `json:"count"`
		Error    string `json:"error"`
	} `json:"providers"`
}

// EmoteEnsureClient talks to the emote service ensure API.
type EmoteEnsureClient struct {
	emoteURL string
	client   *http.Client
	log      *slog.Logger
}

func NewEmoteEnsureClient(emoteURL string, log *slog.Logger) *EmoteEnsureClient {
	emoteURL = strings.TrimRight(strings.TrimSpace(emoteURL), "/")
	if log == nil {
		log = slog.Default()
	}
	return &EmoteEnsureClient{
		emoteURL: emoteURL,
		client:   &http.Client{Timeout: defaultEnsureHTTPTimeout},
		log:      log,
	}
}

func (c *EmoteEnsureClient) enabled() bool {
	return c != nil && c.emoteURL != ""
}

func defaultEmoteEnsureProviders() []string {
	return []string{"seventv", "twitch", "ffz"}
}

func (c *EmoteEnsureClient) EnsureOnce(ctx context.Context, login, broadcasterID string) (emoteEnsureResponse, int, error) {
	var empty emoteEnsureResponse
	if !c.enabled() || login == "" || broadcasterID == "" {
		return empty, 0, fmt.Errorf("emote ensure client not configured")
	}
	body, err := json.Marshal(map[string]any{
		"twitch_id": broadcasterID,
		"providers": defaultEmoteEnsureProviders(),
	})
	if err != nil {
		return empty, 0, err
	}
	url := fmt.Sprintf("%s/v1/channels/%s/emotes/ensure", c.emoteURL, login)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return empty, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return empty, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return empty, resp.StatusCode, err
	}
	syncNetRecord(ctx, netmeter.OpEmote, int64(len(respBody)))
	var parsed emoteEnsureResponse
	_ = json.Unmarshal(respBody, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parsed, resp.StatusCode, fmt.Errorf("emote ensure status %d", resp.StatusCode)
	}
	return parsed, resp.StatusCode, nil
}

func (c *EmoteEnsureClient) WaitUntilReady(
	ctx context.Context,
	login, broadcasterID string,
	enricher *enrich.Enricher,
	timeout time.Duration,
) (emoteEnsureResponse, error) {
	var last emoteEnsureResponse
	if !c.enabled() {
		return last, fmt.Errorf("emote ensure client not configured")
	}
	if timeout <= 0 {
		timeout = defaultEnsureWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return last, ctx.Err()
			case <-time.After(defaultEnsurePollInterval):
			}
		}
		parsed, status, err := c.EnsureOnce(ctx, login, broadcasterID)
		last = parsed
		if err != nil {
			if c.log != nil {
				c.log.Warn("emote ensure request failed", "login", login, "status", status, "err", err)
			}
			return last, err
		}
		if emoteEnsureDictionaryUsable(parsed) {
			if enricher != nil {
				enricher.Invalidate(login)
			}
			return parsed, nil
		}
		if parsed.State == "failed" || (parsed.State == "ready" && parsed.Count == 0 && parsed.Pending == 0) {
			break
		}
	}
	return last, fmt.Errorf("emote ensure not ready")
}

func emoteEnsureDictionaryUsable(parsed emoteEnsureResponse) bool {
	if parsed.State == "failed" {
		return false
	}
	if parsed.State == "ready" && parsed.Count > 0 {
		return true
	}
	if parsed.Count > 0 && parsed.Pending == 0 {
		return true
	}
	for _, p := range parsed.Providers {
		if p.Count > 0 {
			return true
		}
	}
	return parsed.Count > 0
}

// RequireReadyForGold blocks gold chat tokenization until the emote dictionary is ready.
// Skips when emoteURL is unset (legacy aggregate-only sync without emote-service).
func (c *EmoteEnsureClient) RequireReadyForGold(
	ctx context.Context,
	login, broadcasterID string,
	enricher *enrich.Enricher,
) error {
	if !c.enabled() {
		return nil
	}
	login = normalizeLogin(login)
	if login == "" {
		return fmt.Errorf("emote dictionary: channel login required for gold sync")
	}
	if strings.TrimSpace(broadcasterID) == "" {
		return fmt.Errorf("emote dictionary: broadcaster id required for gold sync")
	}
	if _, err := c.WaitUntilReady(ctx, login, broadcasterID, enricher, defaultEnsureWaitTimeout); err != nil {
		return fmt.Errorf("emote dictionary not ready before chat tokenize: %w", err)
	}
	return nil
}

type emoteBroadcasterResolver interface {
	UsersByLogin(ctx context.Context, logins []string) (map[string]UserProfile, error)
}

type loginEmoteState struct {
	state        EmoteSyncState
	source       string
	lastSyncedAt time.Time
	lastKickoff  time.Time
	inFlight     bool
	hasCache     bool
	lastError    string
}

// LiveEmoteEnsurer kicks off non-blocking emote ensure jobs for live IRC tracking.
type LiveEmoteEnsurer struct {
	client     *EmoteEnsureClient
	enricher   *enrich.Enricher
	resolver   emoteBroadcasterResolver
	rdb        *redis.Client
	log        *slog.Logger
	cooldown   time.Duration
	sem        chan struct{}
	eventAPIOn bool

	mu    sync.Mutex
	state map[string]*loginEmoteState
}

type LiveEmoteEnsurerConfig struct {
	EmoteURL       string
	Enricher       *enrich.Enricher
	Resolver       emoteBroadcasterResolver
	Redis          *redis.Client
	Logger         *slog.Logger
	Cooldown       time.Duration
	MaxConcurrent  int
	EventAPIActive bool
}

func NewLiveEmoteEnsurer(cfg LiveEmoteEnsurerConfig) *LiveEmoteEnsurer {
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultLiveEnsureMaxConcurrent
	}
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = defaultLiveEnsureCooldown
	}
	eventAPIOn := cfg.EventAPIActive
	if !eventAPIOn {
		eventAPIOn = strings.EqualFold(strings.TrimSpace(os.Getenv("SEVENTV_EVENTAPI_ENABLED")), "true")
	}
	return &LiveEmoteEnsurer{
		client:     NewEmoteEnsureClient(cfg.EmoteURL, cfg.Logger),
		enricher:   cfg.Enricher,
		resolver:   cfg.Resolver,
		rdb:        cfg.Redis,
		log:        cfg.Logger,
		cooldown:   cooldown,
		sem:        make(chan struct{}, maxConcurrent),
		eventAPIOn: eventAPIOn,
		state:      make(map[string]*loginEmoteState),
	}
}

func (e *LiveEmoteEnsurer) enabled() bool {
	return e != nil && e.client != nil && e.client.enabled()
}

func channelEmoteRedisKey(login string) string {
	return "channel:emotes:" + normalizeLogin(login)
}

func (e *LiveEmoteEnsurer) hasCachedDict(ctx context.Context, login string) bool {
	if e == nil || e.rdb == nil || login == "" {
		return false
	}
	n, err := e.rdb.HLen(ctx, channelEmoteRedisKey(login)).Result()
	return err == nil && n > 0
}

func (e *LiveEmoteEnsurer) loginState(login string) *loginEmoteState {
	login = normalizeLogin(login)
	if login == "" {
		return &loginEmoteState{state: EmoteSyncAggregateOnly}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	st, ok := e.state[login]
	if !ok {
		st = &loginEmoteState{state: EmoteSyncAggregateOnly}
		e.state[login] = st
	}
	return st
}

func (e *LiveEmoteEnsurer) shouldSkipKickoff(st *loginEmoteState) bool {
	if st.inFlight {
		return true
	}
	if !st.lastKickoff.IsZero() && time.Since(st.lastKickoff) < e.cooldown {
		switch st.state {
		case EmoteSyncReady, EmoteSyncSyncing:
			return true
		}
	}
	return false
}

// Kickoff starts an async emote ensure without blocking the caller.
func (e *LiveEmoteEnsurer) Kickoff(ctx context.Context, login string) {
	if !e.enabled() {
		return
	}
	login = normalizeLogin(login)
	if login == "" {
		return
	}

	e.mu.Lock()
	st := e.state[login]
	if st == nil {
		st = &loginEmoteState{state: EmoteSyncAggregateOnly}
		e.state[login] = st
	}
	if e.shouldSkipKickoff(st) {
		e.mu.Unlock()
		return
	}
	st.lastKickoff = time.Now().UTC()
	st.inFlight = true
	if st.hasCache {
		st.state = EmoteSyncStale
		st.source = "cache"
	} else {
		st.state = EmoteSyncSyncing
	}
	e.mu.Unlock()

	go e.runEnsure(context.Background(), login)
}

func (e *LiveEmoteEnsurer) runEnsure(ctx context.Context, login string) {
	defer func() {
		e.mu.Lock()
		if st := e.state[login]; st != nil {
			st.inFlight = false
		}
		e.mu.Unlock()
	}()

	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return
	}

	hasCache := e.hasCachedDict(ctx, login)
	e.mu.Lock()
	if st := e.state[login]; st != nil {
		st.hasCache = hasCache
		if hasCache {
			st.state = EmoteSyncStale
			st.source = "cache"
		}
	}
	e.mu.Unlock()

	broadcasterID := e.resolveBroadcasterID(ctx, login)
	if broadcasterID == "" {
		e.finishEnsure(login, hasCache, fmt.Errorf("broadcaster id unavailable"))
		return
	}

	_, err := e.client.WaitUntilReady(ctx, login, broadcasterID, e.enricher, defaultEnsureWaitTimeout)
	e.finishEnsure(login, hasCache, err)
}

func (e *LiveEmoteEnsurer) resolveBroadcasterID(ctx context.Context, login string) string {
	if e.resolver == nil {
		return ""
	}
	profiles, err := e.resolver.UsersByLogin(ctx, []string{login})
	if err != nil {
		return ""
	}
	if profile, ok := profiles[login]; ok {
		return strings.TrimSpace(profile.ID)
	}
	return ""
}

func (e *LiveEmoteEnsurer) finishEnsure(login string, hadCache bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.state[login]
	if st == nil {
		st = &loginEmoteState{}
		e.state[login] = st
	}
	now := time.Now().UTC()
	if err == nil {
		st.state = EmoteSyncReady
		st.source = "fresh"
		st.lastSyncedAt = now
		st.lastError = ""
		st.hasCache = true
		return
	}
	st.lastError = err.Error()
	if hadCache || st.hasCache {
		st.state = EmoteSyncStale
		st.source = "cache"
		return
	}
	if !e.client.enabled() {
		st.state = EmoteSyncUnavailable
		return
	}
	st.state = EmoteSyncAggregateOnly
}

// Snapshot returns the current emote sync state for extension pulse responses.
func (e *LiveEmoteEnsurer) Snapshot(ctx context.Context, login string, tracking bool) EmoteSyncSnapshot {
	login = normalizeLogin(login)
	if login == "" {
		return emoteSyncSnapshotForState(EmoteSyncAggregateOnly, false, "", nil)
	}
	if e == nil || !e.enabled() {
		return emoteSyncSnapshotForState(EmoteSyncUnavailable, false, "", nil)
	}

	e.mu.Lock()
	st := e.state[login]
	var snap EmoteSyncSnapshot
	if st != nil {
		var lastSynced *time.Time
		if !st.lastSyncedAt.IsZero() {
			t := st.lastSyncedAt.UTC()
			lastSynced = &t
		}
		snap = emoteSyncSnapshotForState(st.state, e.eventAPIOn && tracking, st.source, lastSynced)
	} else {
		snap = emoteSyncSnapshotForState(EmoteSyncAggregateOnly, e.eventAPIOn && tracking, "", nil)
	}
	e.mu.Unlock()

	if snap.State == EmoteSyncAggregateOnly && e.hasCachedDict(ctx, login) {
		snap.State = EmoteSyncStale
		snap.Source = "cache"
		snap.Message = emoteSyncMessage(EmoteSyncStale, "cache")
	}
	return snap
}

func emoteSyncSnapshotForState(state EmoteSyncState, eventAPI bool, source string, lastSynced *time.Time) EmoteSyncSnapshot {
	return EmoteSyncSnapshot{
		State:          state,
		Provider:       "7tv",
		LastSyncedAt:   lastSynced,
		EventAPIActive: eventAPI,
		Source:         source,
		Message:        emoteSyncMessage(state, source),
	}
}

func emoteSyncMessage(state EmoteSyncState, source string) string {
	switch state {
	case EmoteSyncReady:
		return "7TV synced"
	case EmoteSyncSyncing:
		return "7TV syncing…"
	case EmoteSyncStale:
		if source == "cache" {
			return "7TV stale — using cached set"
		}
		return "7TV stale — refresh in progress"
	case EmoteSyncUnavailable:
		return "7TV unavailable — showing aggregate emote spikes only"
	default:
		return "7TV aggregate only — emote identity syncing"
	}
}
