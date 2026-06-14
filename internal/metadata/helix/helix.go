package helix

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

	"streamclone/internal/metadata/model"
)

var ErrDisabled = errors.New("helix: missing client credentials")

type Client struct {
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

type ChatEmote struct {
	ID         string
	Name       string
	EmoteType  string
	EmoteSetID string
	URL1X      string
	URL2X      string
	URL4X      string
	Animated   bool
	IsGlobal   bool
}

func New(apiURL, tokenURL, clientID, secret, userAgent string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 10 * time.Second},
		apiURL:    strings.TrimRight(apiURL, "/"),
		tokenURL:  tokenURL,
		clientID:  clientID,
		secret:    secret,
		userAgent: userAgent,
	}
}

func (c *Client) Enabled() bool {
	return c.clientID != "" && c.secret != "" && c.apiURL != "" && c.tokenURL != ""
}

func (c *Client) bearer(ctx context.Context) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
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

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", errors.New("helix token response missing access_token")
	}
	if body.ExpiresIn <= 0 {
		body.ExpiresIn = 3600
	}

	c.mu.Lock()
	c.token = body.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	c.mu.Unlock()

	return body.AccessToken, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}

	u, err := url.Parse(c.apiURL + path)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()
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

func normalizeThumbnail(u string) string {
	u = strings.ReplaceAll(u, "%{width}", "{width}")
	u = strings.ReplaceAll(u, "%{height}", "{height}")
	return u
}

func (c *Client) ChannelDetails(ctx context.Context, login string) (model.ChannelDetails, error) {
	var users struct {
		Data []struct {
			ID              string `json:"id"`
			Login           string `json:"login"`
			DisplayName     string `json:"display_name"`
			Description     string `json:"description"`
			ProfileImageURL string `json:"profile_image_url"`
			CreatedAt       string `json:"created_at"`
		} `json:"data"`
	}
	q := url.Values{}
	q.Set("login", login)
	if err := c.get(ctx, "/users", q, &users); err != nil {
		return model.ChannelDetails{}, err
	}
	if len(users.Data) == 0 || users.Data[0].ID == "" {
		return model.ChannelDetails{}, errors.New("helix user not found")
	}

	user := users.Data[0]
	details := model.ChannelDetails{
		ID:           user.ID,
		Login:        user.Login,
		DisplayName:  user.DisplayName,
		Description:  user.Description,
		ProfileImage: user.ProfileImageURL,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    time.Now().UnixMilli(),
	}

	var channels struct {
		Data []struct {
			Title    string `json:"title"`
			GameName string `json:"game_name"`
		} `json:"data"`
	}
	q = url.Values{}
	q.Set("broadcaster_id", user.ID)
	if err := c.get(ctx, "/channels", q, &channels); err == nil && len(channels.Data) > 0 {
		details.StreamTitle = channels.Data[0].Title
		details.Category = channels.Data[0].GameName
	}

	var streams struct {
		Data []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			GameName     string `json:"game_name"`
			ViewerCount  int    `json:"viewer_count"`
			ThumbnailURL string `json:"thumbnail_url"`
			StartedAt    string `json:"started_at"`
		} `json:"data"`
	}
	q = url.Values{}
	q.Set("user_id", user.ID)
	if err := c.get(ctx, "/streams", q, &streams); err == nil && len(streams.Data) > 0 {
		stream := streams.Data[0]
		details.IsLive = true
		details.StreamID = stream.ID
		details.StreamTitle = stream.Title
		details.Category = stream.GameName
		details.Viewers = stream.ViewerCount
		details.ThumbnailURL = normalizeThumbnail(stream.ThumbnailURL)
		details.StartedAt = stream.StartedAt
	}

	return details, nil
}

func (c *Client) ChatBadges(ctx context.Context, broadcasterID string) (model.ChatBadgeCatalog, error) {
	badges := make(map[string]model.ChatBadge)
	if err := c.addBadgeSet(ctx, "/chat/badges/global", url.Values{}, badges); err != nil {
		return model.ChatBadgeCatalog{}, err
	}
	if broadcasterID != "" {
		q := url.Values{}
		q.Set("broadcaster_id", broadcasterID)
		if err := c.addBadgeSet(ctx, "/chat/badges", q, badges); err != nil {
			return model.ChatBadgeCatalog{}, err
		}
	}
	return model.ChatBadgeCatalog{Badges: badges, UpdatedAt: time.Now().UnixMilli()}, nil
}

func (c *Client) addBadgeSet(ctx context.Context, path string, params url.Values, badges map[string]model.ChatBadge) error {
	var resp struct {
		Data []struct {
			SetID    string `json:"set_id"`
			Versions []struct {
				ID          string `json:"id"`
				ImageURL1X  string `json:"image_url_1x"`
				ImageURL2X  string `json:"image_url_2x"`
				ImageURL4X  string `json:"image_url_4x"`
				Title       string `json:"title"`
				Description string `json:"description"`
				ClickURL    string `json:"click_url"`
			} `json:"versions"`
		} `json:"data"`
	}
	if err := c.get(ctx, path, params, &resp); err != nil {
		return err
	}
	for _, set := range resp.Data {
		setID := strings.TrimSpace(set.SetID)
		if setID == "" {
			continue
		}
		for _, version := range set.Versions {
			versionID := strings.TrimSpace(version.ID)
			if versionID == "" {
				continue
			}
			key := setID + "/" + versionID
			badges[key] = model.ChatBadge{
				SetID:       setID,
				VersionID:   versionID,
				Title:       version.Title,
				Description: version.Description,
				ClickURL:    version.ClickURL,
				ImageURL1X:  version.ImageURL1X,
				ImageURL2X:  version.ImageURL2X,
				ImageURL4X:  version.ImageURL4X,
			}
		}
	}
	return nil
}

func (c *Client) Clips(ctx context.Context, broadcasterID string, query model.ClipQuery) (model.ClipsResponse, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}
	var resp struct {
		Data []struct {
			ID              string  `json:"id"`
			URL             string  `json:"url"`
			EmbedURL        string  `json:"embed_url"`
			BroadcasterName string  `json:"broadcaster_name"`
			CreatorName     string  `json:"creator_name"`
			Title           string  `json:"title"`
			ViewCount       int     `json:"view_count"`
			CreatedAt       string  `json:"created_at"`
			ThumbnailURL    string  `json:"thumbnail_url"`
			Duration        float64 `json:"duration"`
		} `json:"data"`
		Pagination struct {
			Cursor string `json:"cursor"`
		} `json:"pagination"`
	}
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	q.Set("first", fmt.Sprintf("%d", limit))
	if query.Cursor != "" {
		q.Set("after", query.Cursor)
	}
	if query.StartedAt != nil {
		q.Set("started_at", query.StartedAt.UTC().Format(time.RFC3339))
	}
	if query.EndedAt != nil {
		q.Set("ended_at", query.EndedAt.UTC().Format(time.RFC3339))
	}
	if err := c.get(ctx, "/clips", q, &resp); err != nil {
		return model.ClipsResponse{}, err
	}
	clips := make([]model.ClipCard, 0, len(resp.Data))
	for _, item := range resp.Data {
		clips = append(clips, model.ClipCard{
			ID:              item.ID,
			Title:           item.Title,
			URL:             item.URL,
			EmbedURL:        item.EmbedURL,
			ThumbnailURL:    item.ThumbnailURL,
			BroadcasterName: item.BroadcasterName,
			CreatorName:     item.CreatorName,
			ViewCount:       item.ViewCount,
			CreatedAt:       item.CreatedAt,
			DurationSeconds: item.Duration,
		})
	}
	return model.ClipsResponse{Items: clips, Cursor: resp.Pagination.Cursor}, nil
}

func (c *Client) ChannelEmotes(ctx context.Context, broadcasterID string) ([]ChatEmote, error) {
	q := url.Values{}
	q.Set("broadcaster_id", broadcasterID)
	return c.chatEmotes(ctx, "/chat/emotes", q, false)
}

func (c *Client) GlobalEmotes(ctx context.Context) ([]ChatEmote, error) {
	return c.chatEmotes(ctx, "/chat/emotes/global", url.Values{}, true)
}

func (c *Client) chatEmotes(ctx context.Context, path string, params url.Values, isGlobal bool) ([]ChatEmote, error) {
	var resp struct {
		Data []struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			EmoteType  string   `json:"emote_type"`
			EmoteSetID string   `json:"emote_set_id"`
			Format     []string `json:"format"`
			Images     struct {
				URL1X string `json:"url_1x"`
				URL2X string `json:"url_2x"`
				URL4X string `json:"url_4x"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := c.get(ctx, path, params, &resp); err != nil {
		return nil, err
	}
	out := make([]ChatEmote, 0, len(resp.Data))
	for _, item := range resp.Data {
		out = append(out, ChatEmote{
			ID:         item.ID,
			Name:       item.Name,
			EmoteType:  item.EmoteType,
			EmoteSetID: item.EmoteSetID,
			URL1X:      item.Images.URL1X,
			URL2X:      item.Images.URL2X,
			URL4X:      item.Images.URL4X,
			Animated:   containsFold(item.Format, "animated"),
			IsGlobal:   isGlobal,
		})
	}
	return out, nil
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

// ArchivedStreamHistory lists past broadcasts via Helix /videos (no per-minute viewer stats).
func (c *Client) ArchivedStreamHistory(ctx context.Context, login string, limit int) ([]model.StreamStat, error) {
	if !c.Enabled() || login == "" {
		return nil, ErrDisabled
	}
	if limit <= 0 {
		limit = 80
	}

	var users struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	q := url.Values{}
	q.Set("login", login)
	if err := c.get(ctx, "/users", q, &users); err != nil {
		return nil, err
	}
	if len(users.Data) == 0 || users.Data[0].ID == "" {
		return nil, errors.New("helix user not found")
	}
	broadcasterID := users.Data[0].ID

	out := make([]model.StreamStat, 0, limit)
	cursor := ""
	for page := 0; page < 10 && len(out) < limit; page++ {
		q = url.Values{}
		q.Set("user_id", broadcasterID)
		q.Set("type", "archive")
		q.Set("first", "100")
		if cursor != "" {
			q.Set("after", cursor)
		}
		var resp struct {
			Data []struct {
				ID        string `json:"id"`
				StreamID  string `json:"stream_id"`
				Title     string `json:"title"`
				GameName  string `json:"game_name"`
				Thumbnail string `json:"thumbnail_url"`
				CreatedAt string `json:"created_at"`
				Duration  string `json:"duration"`
			} `json:"data"`
			Pagination struct {
				Cursor string `json:"cursor"`
			} `json:"pagination"`
		}
		if err := c.get(ctx, "/videos", q, &resp); err != nil {
			return out, err
		}
		for _, item := range resp.Data {
			if item.StreamID == "" {
				continue
			}
			startedAt, endedAt, durationMinutes := helixVideoTimes(item.CreatedAt, item.Duration)
			title := strings.TrimSpace(item.Title)
			if title == "" {
				title = "Stream"
			}
			out = append(out, model.StreamStat{
				ID:              item.StreamID,
				VideoID:         item.ID,
				Title:           title,
				Category:        strings.TrimSpace(item.GameName),
				ThumbnailURL:    strings.TrimSpace(item.Thumbnail),
				StartedAt:       startedAt,
				EndedAt:         endedAt,
				DurationMinutes: durationMinutes,
			})
			if len(out) >= limit {
				break
			}
		}
		cursor = resp.Pagination.Cursor
		if cursor == "" || len(resp.Data) == 0 {
			break
		}
	}
	return out, nil
}

// VideoExists reports whether Helix returns a /videos record for the VOD id.
// When the client is disabled, it returns (true, nil) so callers can skip preflight.
func (c *Client) VideoExists(ctx context.Context, videoID string) (bool, error) {
	if !c.Enabled() || videoID == "" {
		return true, nil
	}
	q := url.Values{}
	q.Set("id", videoID)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/videos", q, &resp); err != nil {
		return false, err
	}
	return len(resp.Data) > 0, nil
}

func helixVideoTimes(createdAt, duration string) (startedAt, endedAt string, durationMinutes int) {
	if createdAt == "" {
		return "", "", 0
	}
	start, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return createdAt, "", 0
	}
	startedAt = start.UTC().Format(time.RFC3339)
	if duration == "" {
		return startedAt, "", 0
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return startedAt, "", 0
	}
	durationMinutes = int(d.Round(time.Minute).Minutes())
	if durationMinutes > 0 {
		endedAt = start.Add(d).UTC().Format(time.RFC3339)
	}
	return startedAt, endedAt, durationMinutes
}
