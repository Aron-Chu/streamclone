package twitchclips

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/social"
	"streamclone/internal/social/reliability"
)

func init() {
	social.Register("twitchclips", func() (social.SocialSource, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		return NewSource(cfg), nil
	})
}

type Source struct {
	cfg       config.Config
	client    *http.Client
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewSource(cfg config.Config) *Source {
	return &Source{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Source) Name() string { return "twitchclips" }

func (s *Source) Risk() reliability.Risk { return reliability.RiskOfficial }

func (s *Source) Capabilities() social.Caps { return social.Caps{} }

func (s *Source) Healthy(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.TwitchOAuthClientID) == "" || strings.TrimSpace(s.cfg.TwitchOAuthClientSecret) == "" {
		return fmt.Errorf("twitch client credentials not configured")
	}
	return nil
}

func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {
	if strings.TrimSpace(q.Entity.TwitchLogin) != "" || strings.TrimSpace(q.Entity.TwitchID) != "" {
		return s.searchBroadcaster(ctx, q)
	}
	return s.searchGlobal(ctx, q)
}

func (s *Source) searchBroadcaster(ctx context.Context, q social.Query) (social.Page, error) {
	broadcasterID := strings.TrimSpace(q.Entity.TwitchID)
	login := normalizeLogin(q.Entity.TwitchLogin)
	if broadcasterID == "" && login != "" {
		resolvedID, err := s.lookupBroadcasterID(ctx, login)
		if err != nil {
			return social.Page{}, err
		}
		broadcasterID = resolvedID
	}
	if broadcasterID == "" {
		return social.Page{}, nil
	}
	limit := q.Budget.MaxItems
	if limit <= 0 {
		limit = 10
	}
	clips, status, err := s.fetchClips(ctx, broadcasterID, limit, q.Since)
	if err != nil {
		return social.Page{}, err
	}
	return social.Page{Items: s.itemsFromClips(clips, status, login, q.Entity.TwitchLogin)}, nil
}

func (s *Source) searchGlobal(ctx context.Context, q social.Query) (social.Page, error) {
	limit := q.Budget.MaxItems
	if limit <= 0 {
		limit = 18
	}
	streams, err := s.fetchTopStreams(ctx, min(limit, 6))
	if err != nil {
		return social.Page{}, err
	}
	perBroadcaster := max(1, limit/max(1, len(streams)))
	seen := map[string]struct{}{}
	items := make([]social.Item, 0, limit)
	for _, stream := range streams {
		clips, status, err := s.fetchClips(ctx, stream.ID, perBroadcaster, q.Since)
		if err != nil {
			continue
		}
		for _, item := range s.itemsFromClips(clips, status, stream.Login, stream.DisplayName) {
			if _, ok := seen[item.ExternalID]; ok {
				continue
			}
			seen[item.ExternalID] = struct{}{}
			items = append(items, item)
			if len(items) >= limit {
				return social.Page{Items: items}, nil
			}
		}
	}
	return social.Page{Items: items}, nil
}

func (s *Source) itemsFromClips(clips []helixClip, status int, login, displayName string) []social.Item {
	retention := time.Now().Add(time.Duration(s.cfg.SocialRetentionDays) * 24 * time.Hour)
	items := make([]social.Item, 0, len(clips))
	for _, clip := range clips {
		created, _ := time.Parse(time.RFC3339, clip.CreatedAt)
		raw, _ := json.Marshal(clip)
		sum := sha256.Sum256(raw)
		hintDisplay := strings.TrimSpace(displayName)
		if hintDisplay == "" {
			hintDisplay = strings.TrimSpace(clip.BroadcasterName)
		}
		hintLogin := normalizeLogin(login)
		if hintLogin == "" {
			hintLogin = normalizeLogin(clip.BroadcasterName)
		}
		text := strings.TrimSpace(clip.Title)
		media := []social.MediaRef{}
		if thumb := strings.TrimSpace(clip.ThumbnailURL); thumb != "" {
			media = append(media, social.MediaRef{Kind: "image", URL: thumb})
		}
		items = append(items, social.Item{
			Source:            "twitch_clip",
			Kind:              "clip",
			ExternalID:        clip.ID,
			URL:               clipURL(clip),
			Author:            clip.BroadcasterName,
			Text:              text,
			CreatedAt:         created,
			Metrics:           map[string]float64{"views": float64(clip.ViewCount)},
			Media:             media,
			EntityTwitchLogin: hintLogin,
			EntityDisplayName: hintDisplay,
			Provenance:        social.Provenance{FetchedAt: time.Now(), SourceAPI: "helix_clips", HTTPStatus: status},
			SnapshotSHA256:    sum[:],
			ExpiresAt:         retention,
		})
	}
	return items
}

type helixStream struct {
	ID          string
	Login       string
	DisplayName string
}

type helixClip struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	Title           string `json:"title"`
	ViewCount       int    `json:"view_count"`
	CreatedAt       string `json:"created_at"`
	BroadcasterName string `json:"broadcaster_name"`
	ThumbnailURL    string `json:"thumbnail_url"`
}

func (s *Source) fetchTopStreams(ctx context.Context, first int) ([]helixStream, error) {
	if first <= 0 {
		first = 6
	}
	var out struct {
		Data []struct {
			UserID    string `json:"user_id"`
			UserLogin string `json:"user_login"`
			UserName  string `json:"user_name"`
		} `json:"data"`
	}
	params := url.Values{}
	params.Set("first", fmt.Sprintf("%d", first))
	if err := s.helixGet(ctx, "/streams", params, &out); err != nil {
		return nil, err
	}
	streams := make([]helixStream, 0, len(out.Data))
	for _, row := range out.Data {
		if row.UserID == "" {
			continue
		}
		streams = append(streams, helixStream{
			ID:          row.UserID,
			Login:       normalizeLogin(row.UserLogin),
			DisplayName: strings.TrimSpace(row.UserName),
		})
	}
	return streams, nil
}

func (s *Source) lookupBroadcasterID(ctx context.Context, login string) (string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	params := url.Values{}
	params.Set("login", normalizeLogin(login))
	if err := s.helixGet(ctx, "/users", params, &out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", nil
	}
	return out.Data[0].ID, nil
}

func (s *Source) fetchClips(ctx context.Context, broadcasterID string, limit int, since time.Time) ([]helixClip, int, error) {
	if broadcasterID == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 6
	}
	var out struct {
		Data []helixClip `json:"data"`
	}
	params := url.Values{}
	params.Set("broadcaster_id", broadcasterID)
	params.Set("first", fmt.Sprintf("%d", limit))
	if !since.IsZero() {
		params.Set("started_at", since.Format(time.RFC3339))
	}
	status, err := s.helixGetStatus(ctx, "/clips", params, &out)
	return out.Data, status, err
}

func (s *Source) helixGet(ctx context.Context, path string, params url.Values, out any) error {
	_, err := s.helixGetStatus(ctx, path, params, out)
	return err
}

func (s *Source) helixGetStatus(ctx context.Context, path string, params url.Values, out any) (int, error) {
	token, err := s.bearer(ctx)
	if err != nil {
		return 0, err
	}
	u, err := url.Parse(strings.TrimRight(s.cfg.TwitchAPIURL, "/") + path)
	if err != nil {
		return 0, err
	}
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", s.cfg.TwitchOAuthClientID)
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("helix %s status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func (s *Source) bearer(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" && time.Now().Before(s.expiresAt.Add(-30*time.Second)) {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	form := url.Values{}
	form.Set("client_id", s.cfg.TwitchOAuthClientID)
	form.Set("client_secret", s.cfg.TwitchOAuthClientSecret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.TwitchTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
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
		return "", fmt.Errorf("helix token response missing access_token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	s.mu.Lock()
	s.token = out.AccessToken
	s.expiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	s.mu.Unlock()
	return out.AccessToken, nil
}

func normalizeLogin(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clipURL(clip helixClip) string {
	if u := strings.TrimSpace(clip.URL); u != "" {
		return u
	}
	id := strings.TrimSpace(clip.ID)
	if id == "" {
		return ""
	}
	return "https://clips.twitch.tv/" + id
}

// FetchClipByID loads a single clip from Helix by clip id.
func (s *Source) FetchClipByID(ctx context.Context, clipID string) (helixClip, error) {
	clipID = strings.TrimSpace(clipID)
	if clipID == "" {
		return helixClip{}, fmt.Errorf("missing clip id")
	}
	var out struct {
		Data []helixClip `json:"data"`
	}
	params := url.Values{}
	params.Set("id", clipID)
	if err := s.helixGet(ctx, "/clips", params, &out); err != nil {
		return helixClip{}, err
	}
	if len(out.Data) == 0 {
		return helixClip{}, fmt.Errorf("clip %q not found", clipID)
	}
	return out.Data[0], nil
}
