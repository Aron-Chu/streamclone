package youtube

import (
	"context"

	"crypto/sha256"

	"encoding/json"

	"fmt"

	"net/http"

	"net/url"

	"strings"

	"time"

	"streamclone/internal/config"

	"streamclone/internal/social"

	"streamclone/internal/storygraph/reliability"
)

func init() {

	social.Register("youtube", func() (social.SocialSource, error) {

		cfg, err := config.Load()

		if err != nil {

			return nil, err

		}

		return NewSource(cfg), nil

	})

}

type Source struct {
	cfg config.Config

	client *http.Client
}

func NewSource(cfg config.Config) *Source {

	return &Source{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}

}

func (s *Source) Name() string { return "youtube" }

func (s *Source) Risk() reliability.Risk { return reliability.RiskPublicAPI }

func (s *Source) Capabilities() social.Caps {
	return social.Caps{RefreshMetrics: true, Backfill: true}
}

func (s *Source) Healthy(ctx context.Context) error {

	if strings.TrimSpace(s.cfg.YouTubeAPIKey) == "" && strings.TrimSpace(s.cfg.ScraperAPIURL) == "" {

		return fmt.Errorf("youtube not configured: set YOUTUBE_API_KEY or SCRAPER_API_URL")

	}

	return nil

}

func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {

	keywords := youtubeKeywords(q)

	if len(keywords) == 0 {

		return social.Page{}, nil

	}

	retention := time.Now().Add(time.Duration(s.cfg.SocialRetentionDays) * 24 * time.Hour)

	perKeyword := 4

	if q.Budget.MaxItems > 0 {

		perKeyword = max(1, q.Budget.MaxItems/len(keywords))

		if perKeyword > 6 {

			perKeyword = 6

		}

	}

	items := make([]social.Item, 0, len(keywords)*perKeyword)

	seen := map[string]struct{}{}

	useAPI := strings.TrimSpace(s.cfg.YouTubeAPIKey) != ""
	if !useAPI && q.Budget.MaxBrowserFetches < 0 {
		return social.Page{}, nil
	}
	browserFetches := 0

	for _, keyword := range keywords {

		var provAPI string

		var status int

		if useAPI {

			entries, st, err := s.searchKeywordAPI(ctx, keyword, perKeyword, q.Since)

			if err != nil {

				return social.Page{}, err

			}

			status = st

			provAPI = "youtube_data_v3"

			for _, it := range entries {

				appendAPIItem(&items, seen, it, keyword, retention, provAPI, status, q.Budget.MaxItems)

				if q.Budget.MaxItems > 0 && len(items) >= q.Budget.MaxItems {

					return social.Page{Items: items}, nil

				}

			}

			continue

		}

		entries, st, err := s.searchKeywordScrape(ctx, keyword, perKeyword, q.Since, q.Budget.MaxBrowserFetches, &browserFetches)

		if err != nil {
			if err == errYouTubeBrowserBudgetExhausted && len(items) > 0 {
				return social.Page{Items: items}, nil
			}

			return social.Page{}, err

		}

		status = st

		provAPI = "youtube_scrape"

		for _, it := range entries {

			appendScrapeItem(&items, seen, it, keyword, retention, provAPI, status, q.Budget.MaxItems)

			if q.Budget.MaxItems > 0 && len(items) >= q.Budget.MaxItems {

				return social.Page{Items: items}, nil

			}

		}

	}

	return social.Page{Items: items}, nil

}

// Backfill imports a bounded historical YouTube sweep.
func (s *Source) Backfill(ctx context.Context, q social.Query) (social.Page, error) {
	if q.Since.IsZero() {
		q.Since = time.Now().Add(-7 * 24 * time.Hour)
	}
	if q.Budget.MaxItems <= 0 {
		q.Budget.MaxItems = 24
	}
	return s.Search(ctx, q)
}

func appendAPIItem(items *[]social.Item, seen map[string]struct{}, it struct {
	ID struct {
		VideoID string `json:"videoId"`
	} `json:"id"`

	Snippet struct {
		Title string `json:"title"`

		ChannelTitle string `json:"channelTitle"`

		PublishedAt time.Time `json:"publishedAt"`
	} `json:"snippet"`
}, keyword string, retention time.Time, provAPI string, status, maxItems int) {

	vid := it.ID.VideoID

	if vid == "" {

		return

	}

	if _, ok := seen[vid]; ok {

		return

	}

	seen[vid] = struct{}{}

	link := "https://www.youtube.com/watch?v=" + vid

	raw, _ := json.Marshal(it)

	sum := sha256.Sum256(raw)

	*items = append(*items, social.Item{

		Source: "youtube",

		Kind: "video",

		ExternalID: vid,

		URL: link,

		Author: it.Snippet.ChannelTitle,

		Text: it.Snippet.Title,

		CreatedAt: it.Snippet.PublishedAt,

		Metrics: map[string]float64{},

		EntityTwitchLogin: normalizeKeywordLogin(keyword),

		EntityDisplayName: strings.TrimSpace(keyword),

		Provenance: social.Provenance{FetchedAt: time.Now(), SourceAPI: provAPI, HTTPStatus: status},

		SnapshotSHA256: sum[:],

		ExpiresAt: retention,
	})

}

func appendScrapeItem(items *[]social.Item, seen map[string]struct{}, it scrapedVideo, keyword string, retention time.Time, provAPI string, status, maxItems int) {

	vid := strings.TrimSpace(it.VideoID)

	if vid == "" {

		return

	}

	if _, ok := seen[vid]; ok {

		return

	}

	seen[vid] = struct{}{}

	link := "https://www.youtube.com/watch?v=" + vid

	raw, _ := json.Marshal(it)

	sum := sha256.Sum256(raw)

	*items = append(*items, social.Item{

		Source: "youtube",

		Kind: "video",

		ExternalID: vid,

		URL: link,

		Author: it.ChannelTitle,

		Text: it.Title,

		CreatedAt: time.Now().Add(-2 * time.Hour),

		Metrics: map[string]float64{},

		EntityTwitchLogin: normalizeKeywordLogin(keyword),

		EntityDisplayName: strings.TrimSpace(keyword),

		Provenance: social.Provenance{FetchedAt: time.Now(), SourceAPI: provAPI, HTTPStatus: status},

		SnapshotSHA256: sum[:],

		ExpiresAt: retention,
	})

}

func (s *Source) searchKeywordAPI(ctx context.Context, keyword string, maxResults int, since time.Time) ([]struct {
	ID struct {
		VideoID string `json:"videoId"`
	} `json:"id"`

	Snippet struct {
		Title string `json:"title"`

		ChannelTitle string `json:"channelTitle"`

		PublishedAt time.Time `json:"publishedAt"`
	} `json:"snippet"`
}, int, error) {

	if maxResults <= 0 {

		maxResults = 4

	}

	u, _ := url.Parse(strings.TrimRight(s.cfg.YouTubeAPIBaseURL, "/") + "/search")

	params := u.Query()

	params.Set("part", "snippet")

	params.Set("type", "video")

	params.Set("q", strings.TrimSpace(keyword)+" clip")

	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

	params.Set("key", s.cfg.YouTubeAPIKey)

	if !since.IsZero() {

		params.Set("publishedAfter", since.Format(time.RFC3339))

	}

	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)

	if err != nil {

		return nil, 0, err

	}

	resp, err := s.client.Do(req)

	if err != nil {

		return nil, 0, err

	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		return nil, resp.StatusCode, fmt.Errorf("youtube status %d", resp.StatusCode)

	}

	var out struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`

			Snippet struct {
				Title string `json:"title"`

				ChannelTitle string `json:"channelTitle"`

				PublishedAt time.Time `json:"publishedAt"`
			} `json:"snippet"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {

		return nil, resp.StatusCode, err

	}

	return out.Items, resp.StatusCode, nil

}

func youtubeKeywords(q social.Query) []string {

	if strings.TrimSpace(q.Entity.TwitchLogin) != "" {

		return []string{strings.TrimSpace(q.Entity.TwitchLogin)}

	}

	seen := map[string]struct{}{}

	out := make([]string, 0, len(q.Keywords))

	for _, keyword := range q.Keywords {

		keyword = strings.TrimSpace(keyword)

		if keyword == "" {

			continue

		}

		key := strings.ToLower(keyword)

		if _, ok := seen[key]; ok {

			continue

		}

		seen[key] = struct{}{}

		out = append(out, keyword)

	}

	return out

}

func normalizeKeywordLogin(keyword string) string {

	var out strings.Builder

	for _, r := range strings.ToLower(strings.TrimSpace(keyword)) {

		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {

			out.WriteRune(r)

		}

	}

	return out.String()

}

func max(a, b int) int {

	if a > b {

		return a

	}

	return b

}
