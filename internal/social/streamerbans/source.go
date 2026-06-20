package streamerbans

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/social"
	"streamclone/internal/social/scraper"
	"streamclone/internal/storygraph/reliability"
)

const (
	streamerBansUser    = "StreamerBans"
	streamerbansHomeURL = "https://streamerbans.com/"
	defaultXIngestURL   = "http://x-ingest:8098"
)

func init() {
	social.Register("streamerbans", func() (social.SocialSource, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		return NewSource(cfg), nil
	})
}

type Source struct {
	cfg    config.Config
	client *http.Client
}

func NewSource(cfg config.Config) *Source {
	return &Source{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Source) Name() string { return "streamerbans" }

func (s *Source) Risk() reliability.Risk { return reliability.RiskUnofficial }

func (s *Source) Capabilities() social.Caps { return social.Caps{Backfill: true} }

func (s *Source) Healthy(ctx context.Context) error {
	if !s.cfg.StreamerbansIngestEnabled {
		return fmt.Errorf("streamerbans: set STREAMERBANS_INGEST_ENABLED=true")
	}
	if s.emusksEnabled() {
		if err := s.sidecarHealth(ctx); err == nil {
			return nil
		}
	}
	return s.fallbackHealth(ctx)
}

func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {
	max := 20
	if q.Budget.MaxItems > 0 && q.Budget.MaxItems < max {
		max = q.Budget.MaxItems
	}
	since := q.Since
	if since.IsZero() {
		since = time.Now().Add(-7 * 24 * time.Hour)
	}
	retention := time.Now().Add(time.Duration(s.cfg.SocialRetentionDays) * 24 * time.Hour)

	var lines []timelineLine
	var err error
	// Tier 1: streamerbans.com dashboard feed (user expectation — bans from the site, not X/Reddit).
	if htmlLines, htmlErr := s.fetchStreamerbansHTML(ctx); htmlErr == nil {
		lines = htmlLines
	} else {
		err = htmlErr
	}
	if len(lines) == 0 && s.emusksEnabled() {
		if emusksLines, emusksErr := s.fetchEmusksTimeline(ctx, since); emusksErr == nil {
			lines = emusksLines
		} else if err == nil {
			err = emusksErr
		}
	}
	if len(lines) == 0 && s.emusksEnabled() {
		browserFetches := 0
		if scraperLines, scrapeErr := s.fetchScrapedTimeline(ctx, q.Budget.MaxBrowserFetches, &browserFetches); scrapeErr == nil {
			lines = scraperLines
		} else if err == nil {
			err = scrapeErr
		}
	}
	if len(lines) == 0 && err != nil {
		return social.Page{}, err
	}

	items := make([]social.Item, 0, max)
	seen := map[string]struct{}{}
	for _, line := range lines {
		if len(items) >= max {
			break
		}
		if !line.CreatedAt.IsZero() && line.CreatedAt.Before(since) {
			continue
		}
		login, display, ok := ParseBan(line.Text)
		if !ok {
			continue
		}
		if _, dup := seen[line.ExternalID]; dup {
			continue
		}
		seen[line.ExternalID] = struct{}{}
		headline := banHeadline(login, display)
		pageURL := line.URL
		if pageURL == "" {
			pageURL = userBanURL(login)
		}
		raw, _ := json.Marshal(line)
		sum := sha256.Sum256(raw)
		items = append(items, social.Item{
			Source:            "streamerbans_post",
			Kind:              "post",
			ExternalID:        line.ExternalID,
			URL:               pageURL,
			Author:            streamerBansUser,
			Text:              headline,
			FlairText:         "ban",
			CreatedAt:         line.CreatedAt,
			EntityTwitchLogin: login,
			EntityDisplayName: display,
			Metrics:           map[string]float64{},
			Provenance: social.Provenance{
				FetchedAt:  time.Now(),
				SourceAPI:  line.SourceAPI,
				HTTPStatus: line.HTTPStatus,
			},
			SnapshotSHA256: sum[:],
			ExpiresAt:      retention,
			Media:          streamerbansAvatarMedia(line.ProfileImageURL),
		})
	}
	return social.Page{Items: items}, nil
}

type timelineLine struct {
	ExternalID      string
	Text            string
	URL             string
	CreatedAt       time.Time
	SourceAPI       string
	HTTPStatus      int
	ProfileImageURL string
}

func (s *Source) emusksEnabled() bool {
	return s.cfg.XUnofficialOK && strings.TrimSpace(s.cfg.XContentToken()) != ""
}

func (s *Source) ingestBaseURL() string {
	base := strings.TrimSpace(s.cfg.XIngestURL)
	if base == "" {
		base = defaultXIngestURL
	}
	return strings.TrimRight(base, "/")
}

func (s *Source) sidecarHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.ingestBaseURL()+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("x-ingest health status %d", resp.StatusCode)
	}
	return nil
}

func (s *Source) homeURL() string {
	u := strings.TrimSpace(s.cfg.StreamerbansHomeURL)
	if u == "" {
		return streamerbansHomeURL
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u
}

func (s *Source) fallbackHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.homeURL(), nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("streamerbans.com status %d", resp.StatusCode)
	}
	return nil
}

func (s *Source) fetchEmusksTimeline(ctx context.Context, since time.Time) ([]timelineLine, error) {
	u := s.ingestBaseURL() + "/users/" + streamerBansUser + "/timeline"
	if !since.IsZero() {
		u += "?since=" + url.QueryEscape(since.UTC().Format(time.RFC3339))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("x-ingest status %d", resp.StatusCode)
	}
	var out struct {
		Items []struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			URL       string `json:"url"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	lines := make([]timelineLine, 0, len(out.Items))
	for _, item := range out.Items {
		created, _ := time.Parse(time.RFC3339, item.CreatedAt)
		if created.IsZero() {
			created, _ = time.Parse(time.RFC3339Nano, item.CreatedAt)
		}
		lines = append(lines, timelineLine{
			ExternalID: item.ID,
			Text:       item.Text,
			URL:        item.URL,
			CreatedAt:  created,
			SourceAPI:  "emusks/x-ingest",
			HTTPStatus: resp.StatusCode,
		})
	}
	return lines, nil
}

func (s *Source) fetchStreamerbansHTML(ctx context.Context) ([]timelineLine, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.homeURL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("streamerbans.com status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	htmlBody := string(body)
	lines := parseNextDataFeed(htmlBody)
	if len(lines) == 0 {
		lines = parseBanLinesFromHTML(htmlBody, "streamerbans.com/html", resp.StatusCode)
	}
	return lines, nil
}

var nextDataScriptRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

func parseNextDataFeed(html string) []timelineLine {
	match := nextDataScriptRe.FindStringSubmatch(html)
	if len(match) < 2 {
		return nil
	}
	var payload struct {
		Props struct {
			PageProps struct {
				Feed []struct {
					ID             string `json:"id"`
					IsBan          bool   `json:"is_ban"`
					CreatedAt      string `json:"created_at"`
					Suspensionable struct {
						LoginName       string `json:"login_name"`
						DisplayName     string `json:"display_name"`
						ProfileImageURL string `json:"profile_image_url"`
					} `json:"suspensionable"`
				} `json:"feed"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(match[1]), &payload); err != nil {
		return nil
	}
	out := make([]timelineLine, 0, len(payload.Props.PageProps.Feed))
	for _, item := range payload.Props.PageProps.Feed {
		login := strings.TrimSpace(item.Suspensionable.LoginName)
		display := strings.TrimSpace(item.Suspensionable.DisplayName)
		if login == "" || !item.IsBan {
			continue
		}
		if display == "" {
			display = login
		}
		created, _ := time.Parse(time.RFC3339, item.CreatedAt)
		if created.IsZero() {
			created, _ = time.Parse(time.RFC3339Nano, item.CreatedAt)
		}
		if created.IsZero() {
			created = time.Now().UTC()
		}
		externalID := strings.TrimSpace(item.ID)
		if externalID == "" {
			sum := sha256.Sum256([]byte(login + item.CreatedAt))
			externalID = fmt.Sprintf("%x", sum[:8])
		}
		out = append(out, timelineLine{
			ExternalID:      externalID,
			Text:            banHeadline(login, display),
			URL:             userBanURL(login),
			CreatedAt:       created,
			SourceAPI:       "streamerbans.com/next-data",
			HTTPStatus:      http.StatusOK,
			ProfileImageURL: strings.TrimSpace(item.Suspensionable.ProfileImageURL),
		})
	}
	return out
}

var banLineRe = regexp.MustCompile(`(?i)Twitch Partner "[^"]+"[^<\n]{0,120}has been banned`)

func (s *Source) fetchScrapedTimeline(ctx context.Context, maxBrowserFetches int, browserFetches *int) ([]timelineLine, error) {
	if maxBrowserFetches < 0 {
		return nil, fmt.Errorf("streamerbans: browser fetch budget exhausted")
	}
	if maxBrowserFetches > 0 && browserFetches != nil && *browserFetches >= maxBrowserFetches {
		return nil, fmt.Errorf("streamerbans: browser fetch budget exhausted")
	}
	scraperURL := strings.TrimSpace(s.cfg.ScraperAPIURL)
	if scraperURL == "" {
		return nil, fmt.Errorf("streamerbans: scraper not configured")
	}
	client := scraper.New(scraper.Config{
		URL:       scraperURL,
		Key:       s.cfg.ScraperAPIKey,
		UserAgent: "streamclone/1.0",
	})
	if browserFetches != nil {
		*browserFetches++
	}
	htmlBody, err := client.FetchHTML(ctx, "https://x.com/"+streamerBansUser)
	if err != nil {
		return nil, err
	}
	return parseBanLinesFromHTML(htmlBody, "scraper/x.com", 200), nil
}

func parseBanLinesFromHTML(html, sourceAPI string, status int) []timelineLine {
	seen := map[string]struct{}{}
	var out []timelineLine
	for _, match := range banLineRe.FindAllString(html, 64) {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		sum := sha256.Sum256([]byte(match))
		out = append(out, timelineLine{
			ExternalID: fmt.Sprintf("%x", sum[:8]),
			Text:       match,
			URL:        "https://x.com/" + streamerBansUser,
			CreatedAt:  time.Now().UTC(),
			SourceAPI:  sourceAPI,
			HTTPStatus: status,
		})
	}
	return out
}

func streamerbansAvatarMedia(profileImageURL string) []social.MediaRef {
	profileImageURL = strings.TrimSpace(profileImageURL)
	if profileImageURL == "" {
		return nil
	}
	return []social.MediaRef{{Kind: "image", URL: profileImageURL}}
}
