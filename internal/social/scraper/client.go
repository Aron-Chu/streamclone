package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const maxBody = 8 << 20

// Config configures the Streamclone scraper API client.
type Config struct {
	URL       string
	Key       string
	UserAgent string
}

// Client fetches public HTML via the scraper service.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds a scraper client.
func New(cfg Config) *Client {
	client := &http.Client{Timeout: 45 * time.Second}
	return &Client{cfg: cfg, http: client}
}

// FetchOptions tunes a single scraper HTML fetch.
type FetchOptions struct {
	SiteProfile string
	UseProxy    bool
	TimeoutMs   int
	MaxAgeMs    int
}

// FetchHTML returns page HTML from the scraper API.
func (c *Client) FetchHTML(ctx context.Context, pageURL string) (string, error) {
	return c.FetchHTMLWithOptions(ctx, pageURL, FetchOptions{})
}

// FetchHTMLWithOptions returns page HTML with optional proxy/site profile overrides.
func (c *Client) FetchHTMLWithOptions(ctx context.Context, pageURL string, opts FetchOptions) (string, error) {
	scraperURL := strings.TrimSpace(c.cfg.URL)
	if scraperURL == "" {
		return "", fmt.Errorf("scraper url not configured")
	}
	siteProfile := strings.TrimSpace(opts.SiteProfile)
	if siteProfile == "" {
		siteProfile = "social_public"
	}
	timeoutMs := opts.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 45000
	}
	maxAgeMs := opts.MaxAgeMs
	if maxAgeMs <= 0 {
		maxAgeMs = 300000
	}
	payload := map[string]any{
		"url":             pageURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"siteProfile":     siteProfile,
		"maxAge":          maxAgeMs,
		"timeout":         timeoutMs,
	}
	if opts.UseProxy {
		payload["useProxy"] = true
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scraperURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.cfg.Key); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if ua := strings.TrimSpace(c.cfg.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("scraper status %d", resp.StatusCode)
	}
	var out struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			HTML     string `json:"html"`
			RawHTML  string `json:"rawHtml"`
			Markdown string `json:"markdown"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return "", err
	}
	if !out.Success && out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	htmlBody := out.Data.HTML
	if htmlBody == "" {
		htmlBody = out.Data.RawHTML
	}
	if htmlBody == "" {
		htmlBody = out.Data.Markdown
	}
	if strings.TrimSpace(htmlBody) == "" {
		return "", fmt.Errorf("scrape response missing html")
	}
	return htmlBody, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err == nil || !IsTransientError(err) || req.Body == nil {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if req.GetBody == nil {
		return resp, err
	}
	timer := time.NewTimer(350 * time.Millisecond)
	select {
	case <-req.Context().Done():
		timer.Stop()
		return nil, req.Context().Err()
	case <-timer.C:
	}
	retry := req.Clone(req.Context())
	body, bodyErr := req.GetBody()
	if bodyErr != nil {
		return nil, err
	}
	retry.Body = body
	return c.http.Do(retry)
}

func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "server closed idle connection") ||
		strings.Contains(msg, "client.timeout exceeded")
}
