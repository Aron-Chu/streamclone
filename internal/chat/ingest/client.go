package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	enabled bool
	client  *http.Client
}

type Message struct {
	Channel     string          `json:"channel"`
	Login       string          `json:"login"`
	DisplayName string          `json:"displayName"`
	MessageID   string          `json:"messageId"`
	Text        string          `json:"text"`
	Fragments   json.RawMessage `json:"fragments,omitempty"`
	TS          int64           `json:"ts"`
}

type Event struct {
	Kind        string `json:"kind"`
	Channel     string `json:"channel"`
	ActorLogin  string `json:"actorLogin,omitempty"`
	TargetLogin string `json:"targetLogin,omitempty"`
	DurationSec int    `json:"durationSec,omitempty"`
	Reason      string `json:"reason,omitempty"`
	MessageID   string `json:"messageId,omitempty"`
	TextPreview string `json:"textPreview,omitempty"`
	TS          int64  `json:"ts"`
}

type payload struct {
	Messages []Message `json:"messages,omitempty"`
	Events   []Event   `json:"events,omitempty"`
}

func New(baseURL string, enabled bool) *Client {
	return &Client{
		baseURL: baseURL,
		enabled: enabled,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled && c.baseURL != ""
}

func (c *Client) ForwardMessages(msgs ...Message) {
	if !c.Enabled() || len(msgs) == 0 {
		return
	}
	go c.post(payload{Messages: msgs})
}

func (c *Client) ForwardEvents(events ...Event) {
	if !c.Enabled() || len(events) == 0 {
		return
	}
	go c.post(payload{Events: events})
}

func (c *Client) post(body payload) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/analytics/chat/ingest", bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
