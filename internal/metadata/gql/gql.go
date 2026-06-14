package gql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"streamclone/internal/upstream"
)

type Stream struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	ViewersCount    int    `json:"viewers"`
	ThumbnailURL    string `json:"thumbnailUrl"`
	Category        string `json:"category"`
	Login           string `json:"login"`
	DisplayName     string `json:"displayName"`
	IsLive          bool   `json:"isLive,omitempty"`
	ProfileImageURL string `json:"profileImageUrl,omitempty"`
}

type Category struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Viewers      int    `json:"viewers,omitempty"`
}

type Channel struct {
	ID           string `json:"id"`
	Login        string `json:"login"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	ProfileImage string `json:"profileImage,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	IsLive       bool   `json:"isLive,omitempty"`
	StreamID     string `json:"streamId,omitempty"`
	StreamTitle  string `json:"streamTitle,omitempty"`
	Category     string `json:"category,omitempty"`
	Viewers      int    `json:"viewers,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
}

type AboutPanel struct {
	ID          string `json:"id,omitempty"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	LinkURL     string `json:"linkUrl,omitempty"`
}

type SocialLink struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type ChannelAbout struct {
	Panels      []AboutPanel `json:"panels,omitempty"`
	SocialLinks []SocialLink `json:"socialLinks,omitempty"`
}

type Page[T any] struct {
	Items  []T    `json:"items"`
	Cursor string `json:"cursor"`
}

type HeaderProvider interface {
	Headers() map[string]string
	Refresh(ctx context.Context) error
}

type staticProvider struct {
	clientID  string
	userAgent string
}

func (p *staticProvider) Headers() map[string]string {
	return map[string]string{
		"Client-ID":  p.clientID,
		"User-Agent": p.userAgent,
	}
}

func (p *staticProvider) Refresh(_ context.Context) error { return nil }

func NewStaticProvider(clientID, userAgent string) HeaderProvider {
	return &staticProvider{clientID: clientID, userAgent: userAgent}
}

type Client struct {
	http     *http.Client
	url      string
	provider HeaderProvider
}

func New(e upstream.Endpoints, provider HeaderProvider) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		url:      e.TwitchGQLURL,
		provider: provider,
	}
}

func normalizeThumbnail(u string) string {
	u = strings.ReplaceAll(u, "%{width}", "{width}")
	u = strings.ReplaceAll(u, "%{height}", "{height}")
	u = strings.ReplaceAll(u, "{width}x{height}", "{width}x{height}")
	return u
}

func (c *Client) do(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.provider.Headers() {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		if rerr := c.provider.Refresh(ctx); rerr != nil {
			return nil, fmt.Errorf("header refresh failed: %w", rerr)
		}
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		for k, v := range c.provider.Headers() {
			req2.Header.Set(k, v)
		}
		return c.http.Do(req2)
	}
	return resp, nil
}

type streamNode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ViewersCount int    `json:"viewersCount"`
	PreviewURL   string `json:"previewImageURL"`
	Broadcaster  *struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
	} `json:"broadcaster"`
	Game *struct {
		Name string `json:"name"`
	} `json:"game"`
}

func (n streamNode) toStream() Stream {
	s := Stream{
		ID:           n.ID,
		Title:        n.Title,
		ViewersCount: n.ViewersCount,
		ThumbnailURL: normalizeThumbnail(n.PreviewURL),
	}
	if n.Broadcaster != nil {
		s.Login = n.Broadcaster.Login
		s.DisplayName = n.Broadcaster.DisplayName
	}
	if n.Game != nil {
		s.Category = n.Game.Name
	}
	return s
}

const queryTopStreams = upstream.MetadataTopStreamsOperation

func (c *Client) TopStreams(ctx context.Context, limit int, cursor string) (Page[Stream], error) {
	vars := map[string]any{"first": limit}
	if cursor != "" {
		vars["after"] = cursor
	}
	body, _ := json.Marshal(map[string]any{"operationName": "TopStreams", "query": queryTopStreams, "variables": vars})
	resp, err := c.do(ctx, body)
	if err != nil {
		return Page[Stream]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page[Stream]{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			Streams *struct {
				Edges []struct {
					Cursor string     `json:"cursor"`
					Node   streamNode `json:"node"`
				} `json:"edges"`
			} `json:"streams"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Page[Stream]{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.Streams == nil {
		return Page[Stream]{}, upstream.ErrUpstreamSchema
	}
	var page Page[Stream]
	for _, e := range r.Data.Streams.Edges {
		if e.Node.Broadcaster == nil {
			continue
		}
		page.Items = append(page.Items, e.Node.toStream())
		page.Cursor = e.Cursor
	}
	return page, nil
}

const queryCategories = upstream.MetadataCategoriesOperation

func (c *Client) Categories(ctx context.Context, limit int, cursor string) (Page[Category], error) {
	vars := map[string]any{"first": limit}
	if cursor != "" {
		vars["after"] = cursor
	}
	body, _ := json.Marshal(map[string]any{"operationName": "TopCategories", "query": queryCategories, "variables": vars})
	resp, err := c.do(ctx, body)
	if err != nil {
		return Page[Category]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page[Category]{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			Games *struct {
				Edges []struct {
					Cursor string `json:"cursor"`
					Node   struct {
						ID        string `json:"id"`
						Name      string `json:"name"`
						Viewers   int    `json:"viewersCount"`
						BoxArtURL string `json:"boxArtURL"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"games"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Page[Category]{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.Games == nil {
		return Page[Category]{}, upstream.ErrUpstreamSchema
	}
	var page Page[Category]
	for _, e := range r.Data.Games.Edges {
		page.Items = append(page.Items, Category{ID: e.Node.ID, Name: e.Node.Name, ThumbnailURL: normalizeThumbnail(e.Node.BoxArtURL), Viewers: e.Node.Viewers})
		page.Cursor = e.Cursor
	}
	return page, nil
}

const queryCategoryStreams = upstream.MetadataCategoryStreamsOperation

func (c *Client) CategoryStreams(ctx context.Context, categoryID string, limit int, cursor string) (Page[Stream], error) {
	vars := map[string]any{"id": categoryID, "first": limit}
	if cursor != "" {
		vars["after"] = cursor
	}
	body, _ := json.Marshal(map[string]any{"operationName": "CategoryStreams", "query": queryCategoryStreams, "variables": vars})
	resp, err := c.do(ctx, body)
	if err != nil {
		return Page[Stream]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page[Stream]{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			Game *struct {
				Streams *struct {
					Edges []struct {
						Cursor string     `json:"cursor"`
						Node   streamNode `json:"node"`
					} `json:"edges"`
				} `json:"streams"`
			} `json:"game"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Page[Stream]{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.Game == nil || r.Data.Game.Streams == nil {
		return Page[Stream]{}, upstream.ErrUpstreamSchema
	}
	var page Page[Stream]
	for _, e := range r.Data.Game.Streams.Edges {
		if e.Node.Broadcaster == nil {
			continue
		}
		page.Items = append(page.Items, e.Node.toStream())
		page.Cursor = e.Cursor
	}
	return page, nil
}

type SearchResult struct {
	Streams    []Stream   `json:"streams"`
	Categories []Category `json:"categories"`
}

const querySearch = upstream.MetadataSearchOperation

func (c *Client) Search(ctx context.Context, query string, limit int) (SearchResult, error) {
	body, _ := json.Marshal(map[string]any{"operationName": "SearchResults", "query": querySearch, "variables": map[string]any{"query": query}})
	resp, err := c.do(ctx, body)
	if err != nil {
		return SearchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SearchResult{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			SearchFor *struct {
				Channels *struct {
					Items []struct {
						ID               string `json:"id"`
						Login            string `json:"login"`
						DisplayName      string `json:"displayName"`
						ProfileImageURL  string `json:"profileImageURL"`
						Stream           *struct {
							ID          string `json:"id"`
							Title       string `json:"title"`
							ViewersCount int   `json:"viewersCount"`
							PreviewURL  string `json:"previewImageURL"`
							Game        *struct {
								Name string `json:"name"`
							} `json:"game"`
						} `json:"stream"`
					} `json:"items"`
				} `json:"channels"`
				Games *struct {
					Items []struct {
						ID        string `json:"id"`
						Name      string `json:"name"`
						BoxArtURL string `json:"boxArtURL"`
					} `json:"items"`
				} `json:"games"`
			} `json:"searchFor"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return SearchResult{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.SearchFor == nil {
		return SearchResult{}, upstream.ErrUpstreamSchema
	}
	var result SearchResult
	if r.Data.SearchFor.Channels != nil {
		for i, item := range r.Data.SearchFor.Channels.Items {
			if limit > 0 && i >= limit {
				break
			}
			stream := Stream{
				ID:              item.ID,
				Login:           item.Login,
				DisplayName:     item.DisplayName,
				ProfileImageURL: item.ProfileImageURL,
			}
			if item.Stream != nil && item.Stream.ID != "" {
				stream.IsLive = true
				stream.ID = item.Stream.ID
				stream.Title = strings.TrimSpace(item.Stream.Title)
				stream.ViewersCount = item.Stream.ViewersCount
				stream.ThumbnailURL = normalizeThumbnail(item.Stream.PreviewURL)
				if item.Stream.Game != nil {
					stream.Category = strings.TrimSpace(item.Stream.Game.Name)
				}
			}
			if stream.Title == "" {
				stream.Title = item.DisplayName
			}
			if stream.Category == "" {
				stream.Category = "Live"
			}
			result.Streams = append(result.Streams, stream)
		}
	}
	if r.Data.SearchFor.Games != nil {
		for i, item := range r.Data.SearchFor.Games.Items {
			if limit > 0 && i >= limit {
				break
			}
			result.Categories = append(result.Categories, Category{ID: item.ID, Name: item.Name, ThumbnailURL: normalizeThumbnail(item.BoxArtURL)})
		}
	}
	return result, nil
}

const queryChannel = upstream.MetadataChannelOperation

func (c *Client) Channel(ctx context.Context, login string) (Channel, error) {
	body, _ := json.Marshal(map[string]any{"operationName": "ChannelByLogin", "query": queryChannel, "variables": map[string]any{"login": login}})
	resp, err := c.do(ctx, body)
	if err != nil {
		return Channel{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Channel{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			User *struct {
				ID              string `json:"id"`
				Login           string `json:"login"`
				DisplayName     string `json:"displayName"`
				Description     string `json:"description"`
				ProfileImageURL string `json:"profileImageURL"`
				CreatedAt       string `json:"createdAt"`
				Stream          *struct {
					ID           string `json:"id"`
					Title        string `json:"title"`
					ViewersCount int    `json:"viewersCount"`
					PreviewURL   string `json:"previewImageURL"`
					CreatedAt    string `json:"createdAt"`
					Game         *struct {
						Name string `json:"name"`
					} `json:"game"`
				} `json:"stream"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Channel{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.User == nil || r.Data.User.ID == "" {
		return Channel{}, upstream.ErrUpstreamSchema
	}
	ch := Channel{
		ID:           r.Data.User.ID,
		Login:        r.Data.User.Login,
		DisplayName:  r.Data.User.DisplayName,
		Description:  r.Data.User.Description,
		ProfileImage: r.Data.User.ProfileImageURL,
		CreatedAt:    r.Data.User.CreatedAt,
	}
	if r.Data.User.Stream != nil {
		ch.IsLive = true
		ch.StreamID = r.Data.User.Stream.ID
		ch.StreamTitle = r.Data.User.Stream.Title
		ch.Viewers = r.Data.User.Stream.ViewersCount
		ch.ThumbnailURL = normalizeThumbnail(r.Data.User.Stream.PreviewURL)
		ch.StartedAt = r.Data.User.Stream.CreatedAt
		if r.Data.User.Stream.Game != nil {
			ch.Category = r.Data.User.Stream.Game.Name
		}
	}
	return ch, nil
}

const queryChannelAbout = upstream.MetadataChannelAboutOperation

func (c *Client) ChannelAbout(ctx context.Context, login string) (ChannelAbout, error) {
	body, _ := json.Marshal(map[string]any{"operationName": "ChannelAbout", "query": queryChannelAbout, "variables": map[string]any{"login": login}})
	resp, err := c.do(ctx, body)
	if err != nil {
		return ChannelAbout{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ChannelAbout{}, fmt.Errorf("gql status %d", resp.StatusCode)
	}
	var r struct {
		Data struct {
			User *struct {
				Panels []struct {
					ID          string `json:"id"`
					Type        string `json:"type"`
					Title       string `json:"title"`
					Description string `json:"description"`
					ImageURL    string `json:"imageURL"`
					LinkURL     string `json:"linkURL"`
				} `json:"panels"`
				SocialMedias []struct {
					ID    string `json:"id"`
					Name  string `json:"name"`
					Title string `json:"title"`
					URL   string `json:"url"`
				} `json:"socialMedias"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return ChannelAbout{}, fmt.Errorf("%w: %v", upstream.ErrUpstreamSchema, err)
	}
	if r.Data.User == nil {
		return ChannelAbout{}, upstream.ErrUpstreamSchema
	}
	out := ChannelAbout{
		Panels:      make([]AboutPanel, 0, len(r.Data.User.Panels)),
		SocialLinks: make([]SocialLink, 0, len(r.Data.User.SocialMedias)),
	}
	for _, panel := range r.Data.User.Panels {
		if strings.TrimSpace(panel.Title+panel.Description+panel.ImageURL+panel.LinkURL) == "" {
			continue
		}
		out.Panels = append(out.Panels, AboutPanel{
			ID:          panel.ID,
			Type:        panel.Type,
			Title:       panel.Title,
			Description: panel.Description,
			ImageURL:    panel.ImageURL,
			LinkURL:     panel.LinkURL,
		})
		if panel.LinkURL != "" {
			title := panel.Title
			if title == "" {
				title = firstLine(panel.Description)
			}
			if title == "" {
				title = hostLabel(panel.LinkURL)
			}
			out.SocialLinks = append(out.SocialLinks, SocialLink{ID: panel.ID, Title: title, URL: panel.LinkURL})
		}
	}
	for _, social := range r.Data.User.SocialMedias {
		title := social.Title
		if title == "" {
			title = social.Name
		}
		if strings.TrimSpace(title+social.URL) == "" {
			continue
		}
		out.SocialLinks = append(out.SocialLinks, SocialLink{ID: social.ID, Title: title, URL: social.URL})
	}
	return out, nil
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if len(trimmed) > 48 {
				return trimmed[:48]
			}
			return trimmed
		}
	}
	return ""
}

func hostLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch {
	case strings.Contains(host, "youtube"):
		return "YouTube"
	case strings.Contains(host, "twitter"), strings.Contains(host, "x.com"):
		return "Twitter"
	case strings.Contains(host, "discord"):
		return "Discord"
	case strings.Contains(host, "streamelements"):
		return "StreamElements"
	case strings.Contains(host, "twitch"):
		return "Twitch"
	default:
		return host
	}
}
