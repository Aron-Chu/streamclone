package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrHelixDisabled = errors.New("analytics helix: missing client credentials")

type HelixClient struct {
	http      *http.Client
	apiURL    string
	tokenURL  string
	clientID  string
	secret    string
	userAgent string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewHelixClient(apiURL, tokenURL, clientID, secret, userAgent string) *HelixClient {
	return &HelixClient{
		http:      &http.Client{Timeout: 10 * time.Second},
		apiURL:    strings.TrimRight(apiURL, "/"),
		tokenURL:  tokenURL,
		clientID:  clientID,
		secret:    secret,
		userAgent: userAgent,
	}
}

func (c *HelixClient) Enabled() bool {
	return c.clientID != "" && c.secret != "" && c.apiURL != "" && c.tokenURL != ""
}

func (c *HelixClient) bearer(ctx context.Context) (string, error) {
	if !c.Enabled() {
		return "", ErrHelixDisabled
	}
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.secret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("helix token status %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("helix token response missing access_token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	c.mu.Lock()
	c.token = out.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	c.mu.Unlock()
	return out.AccessToken, nil
}

func (c *HelixClient) get(ctx context.Context, path string, q url.Values, out any) error {
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	u, err := url.Parse(c.apiURL + path)
	if err != nil {
		return err
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", c.clientID)
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("helix %s status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HelixClient) StreamsByLogin(ctx context.Context, logins []string) (map[string]LiveStream, error) {
	out := make(map[string]LiveStream)
	for _, chunk := range chunkStrings(logins, 100) {
		q := url.Values{}
		for _, login := range chunk {
			if login = normalizeLogin(login); login != "" {
				q.Add("user_login", login)
			}
		}
		if len(q["user_login"]) == 0 {
			continue
		}
		var resp struct {
			Data []struct {
				ID           string   `json:"id"`
				UserID       string   `json:"user_id"`
				UserLogin    string   `json:"user_login"`
				UserName     string   `json:"user_name"`
				GameName     string   `json:"game_name"`
				Title        string   `json:"title"`
				Tags         []string `json:"tags"`
				ViewerCount  int      `json:"viewer_count"`
				StartedAt    string   `json:"started_at"`
				Language     string   `json:"language"`
				ThumbnailURL string   `json:"thumbnail_url"`
			} `json:"data"`
		}
		if err := c.get(ctx, "/streams", q, &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Data {
			startedAt, _ := time.Parse(time.RFC3339, item.StartedAt)
			login := normalizeLogin(item.UserLogin)
			out[login] = LiveStream{
				ID:            item.ID,
				BroadcasterID: item.UserID,
				Login:         login,
				DisplayName:   item.UserName,
				GameName:      item.GameName,
				Title:         item.Title,
				Tags:          item.Tags,
				ViewerCount:   item.ViewerCount,
				StartedAt:     startedAt,
				Language:      item.Language,
				ThumbnailURL:  normalizeThumbnail(item.ThumbnailURL),
			}
		}
	}
	return out, nil
}

func (c *HelixClient) UsersByLogin(ctx context.Context, logins []string) (map[string]UserProfile, error) {
	out := make(map[string]UserProfile)
	for _, chunk := range chunkStrings(logins, 100) {
		q := url.Values{}
		for _, login := range chunk {
			if login = normalizeLogin(login); login != "" {
				q.Add("login", login)
			}
		}
		if len(q["login"]) == 0 {
			continue
		}
		var resp struct {
			Data []struct {
				ID              string `json:"id"`
				Login           string `json:"login"`
				DisplayName     string `json:"display_name"`
				ProfileImageURL string `json:"profile_image_url"`
				Description     string `json:"description"`
			} `json:"data"`
		}
		if err := c.get(ctx, "/users", q, &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Data {
			out[normalizeLogin(item.Login)] = UserProfile{
				ID:              item.ID,
				Login:           normalizeLogin(item.Login),
				DisplayName:     item.DisplayName,
				ProfileImageURL: item.ProfileImageURL,
				Description:     item.Description,
			}
		}
	}
	return out, nil
}

func normalizeThumbnail(raw string) string {
	raw = strings.ReplaceAll(raw, "%{width}", "{width}")
	raw = strings.ReplaceAll(raw, "%{height}", "{height}")
	return raw
}

func chunkStrings(items []string, size int) [][]string {
	if size <= 0 || len(items) == 0 {
		return nil
	}
	var chunks [][]string
	for len(items) > 0 {
		n := size
		if len(items) < n {
			n = len(items)
		}
		chunks = append(chunks, items[:n])
		items = items[n:]
	}
	return chunks
}

func (c *HelixClient) VideoIDByStreamID(ctx context.Context, broadcasterID, streamID string) (string, error) {
	if !c.Enabled() || broadcasterID == "" || streamID == "" {
		return "", nil
	}
	cursor := ""
	for page := 0; page < 25; page++ {
		q := url.Values{}
		q.Set("user_id", broadcasterID)
		q.Set("type", "archive")
		q.Set("first", "100")
		if cursor != "" {
			q.Set("after", cursor)
		}
		var resp struct {
			Data []struct {
				ID       string `json:"id"`
				StreamID string `json:"stream_id"`
			} `json:"data"`
			Pagination struct {
				Cursor string `json:"cursor"`
			} `json:"pagination"`
		}
		if err := c.get(ctx, "/videos", q, &resp); err != nil {
			return "", err
		}
		for _, item := range resp.Data {
			if item.StreamID == streamID {
				return item.ID, nil
			}
		}
		cursor = resp.Pagination.Cursor
		if cursor == "" || len(resp.Data) == 0 {
			break
		}
	}
	return "", nil
}

func (c *HelixClient) VideoDurationSeconds(ctx context.Context, videoID string) (int, error) {
	if !c.Enabled() || videoID == "" {
		return 0, nil
	}
	q := url.Values{}
	q.Set("id", videoID)
	var resp struct {
		Data []struct {
			Duration string `json:"duration"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/videos", q, &resp); err != nil {
		return 0, err
	}
	if len(resp.Data) == 0 || resp.Data[0].Duration == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(resp.Data[0].Duration)
	if err != nil {
		return 0, fmt.Errorf("parse helix duration %q: %w", resp.Data[0].Duration, err)
	}
	seconds := int(d.Seconds())
	if seconds <= 0 {
		return 0, nil
	}
	return seconds, nil
}

