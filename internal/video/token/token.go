package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"streamclone/internal/upstream"
)

type Token struct {
	Value     string
	Signature string
}

type Client struct {
	http     *http.Client
	url      string
	clientID string
	ua       string
}

func New(e upstream.Endpoints) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		url:      e.TwitchGQLURL,
		clientID: e.TwitchClientID,
		ua:       e.UserAgent,
	}
}

const query = upstream.PlaybackAccessTokenOperation

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

func (c *Client) Live(ctx context.Context, login string) (Token, error) {
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
