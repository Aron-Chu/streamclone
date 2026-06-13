package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"streamclone/internal/upstream"
)

type Token struct {
	Value     string
	Signature string
}

type cachedEntry struct {
	token     Token
	expiresAt time.Time
}

type Client struct {
	http     *http.Client
	url      string
	clientID string
	ua       string

	mu    sync.Mutex
	cache map[string]cachedEntry
	sf    singleflight.Group
}

const tokenCacheTTL = 45 * time.Second

func New(e upstream.Endpoints) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		url:      e.TwitchGQLURL,
		clientID: e.TwitchClientID,
		ua:       e.UserAgent,
		cache:    make(map[string]cachedEntry),
	}
}

const (
	query    = upstream.PlaybackAccessTokenOperation
	vodQuery = upstream.VodPlaybackAccessTokenOperation
)

type response struct {
	Data struct {
		Stream *struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"streamPlaybackAccessToken"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type vodResponse struct {
	Data struct {
		Video *struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"videoPlaybackAccessToken"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) Live(ctx context.Context, login string) (Token, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return Token{}, fmt.Errorf("%w: empty login", upstream.ErrPlaybackToken)
	}
	if tok, ok := c.cached(login); ok {
		return tok, nil
	}
	v, err, _ := c.sf.Do(login, func() (any, error) {
		if tok, ok := c.cached(login); ok {
			return tok, nil
		}
		tok, err := c.fetchLive(ctx, login)
		if err != nil {
			return Token{}, err
		}
		c.store(login, tok)
		return tok, nil
	})
	if err != nil {
		return Token{}, err
	}
	return v.(Token), nil
}

func (c *Client) cached(login string) (Token, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[login]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(c.cache, login)
		}
		return Token{}, false
	}
	return entry.token, true
}

func (c *Client) store(login string, tok Token) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[login] = cachedEntry{token: tok, expiresAt: time.Now().Add(tokenCacheTTL)}
}

func (c *Client) fetchLive(ctx context.Context, login string) (Token, error) {
	playerType := os.Getenv("TWITCH_PLAYER_TYPE")
	if playerType == "" {
		playerType = "embed"
	}

	payload, _ := json.Marshal(map[string]any{
		"operationName": "PlaybackAccessTokenLive",
		"query":         query,
		"variables": map[string]any{
			"login":      login,
			"playerType": playerType,
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("%w: %v", upstream.ErrPlaybackToken, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("%w: status %d", upstream.ErrPlaybackToken, resp.StatusCode)
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Token{}, errors.Join(upstream.ErrPlaybackToken, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err))
	}
	if len(r.Errors) > 0 {
		return Token{}, fmt.Errorf("%w: %s", upstream.ErrPlaybackToken, r.Errors[0].Message)
	}
	if r.Data.Stream == nil || r.Data.Stream.Value == "" {
		return Token{}, errors.Join(upstream.ErrPlaybackToken, upstream.ErrUpstreamSchema)
	}
	return Token{Value: r.Data.Stream.Value, Signature: r.Data.Stream.Signature}, nil
}

func vodCacheKey(vodID string) string { return "vod:" + vodID }

func (c *Client) Vod(ctx context.Context, vodID string) (Token, error) {
	vodID = strings.TrimSpace(vodID)
	if vodID == "" {
		return Token{}, fmt.Errorf("%w: empty vod id", upstream.ErrPlaybackToken)
	}
	key := vodCacheKey(vodID)
	if tok, ok := c.cached(key); ok {
		return tok, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		if tok, ok := c.cached(key); ok {
			return tok, nil
		}
		tok, err := c.fetchVod(ctx, vodID)
		if err != nil {
			return Token{}, err
		}
		c.store(key, tok)
		return tok, nil
	})
	if err != nil {
		return Token{}, err
	}
	return v.(Token), nil
}

func (c *Client) fetchVod(ctx context.Context, vodID string) (Token, error) {
	playerType := os.Getenv("TWITCH_PLAYER_TYPE")
	if playerType == "" {
		playerType = "embed"
	}

	payload, _ := json.Marshal(map[string]any{
		"operationName": "PlaybackAccessTokenVod",
		"query":         vodQuery,
		"variables": map[string]any{
			"vodID":      vodID,
			"playerType": playerType,
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("%w: %v", upstream.ErrPlaybackToken, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("%w: status %d", upstream.ErrPlaybackToken, resp.StatusCode)
	}

	var r vodResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Token{}, errors.Join(upstream.ErrPlaybackToken, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err))
	}
	if len(r.Errors) > 0 {
		return Token{}, fmt.Errorf("%w: %s", upstream.ErrPlaybackToken, r.Errors[0].Message)
	}
	if r.Data.Video == nil || r.Data.Video.Value == "" {
		return Token{}, errors.Join(upstream.ErrPlaybackToken, upstream.ErrUpstreamSchema)
	}
	return Token{Value: r.Data.Video.Value, Signature: r.Data.Video.Signature}, nil
}
