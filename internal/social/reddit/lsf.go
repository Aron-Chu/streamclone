package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamclone/internal/metadata/model"
	"streamclone/internal/social/scraper"
)

type tokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

// Client fetches Reddit / LSF listings.
type Client struct {
	baseURL        string
	oauthAPIURL    string
	tokenURL       string
	provider       string
	clientID       string
	clientSecret   string
	accessToken    string
	htmlFallback   bool
	thirdPartyURL  string
	thirdPartyKey  string
	oldRedditURL   string
	lsfLowPriority bool
	scraperAPIURL  string
	scraperAPIKey  string
	socialScrapeUseProxy bool
	userAgent      string
	http           *http.Client
	scrapeHTTP     *http.Client
	mu             sync.Mutex
	token          tokenCache
	backoff        map[string]time.Time
}

type browserFetchBudget struct {
	max  int
	used int
}

func newBrowserFetchBudget(max int) *browserFetchBudget {
	return &browserFetchBudget{max: max}
}

func (b *browserFetchBudget) trySpend() bool {
	if b == nil || b.max == 0 {
		return true
	}
	if b.max < 0 {
		return false
	}
	if b.used >= b.max {
		return false
	}
	b.used++
	return true
}

func browserBudgetStatus(source, provider string) model.SourceStatus {
	return sourceWithProvider(source, provider, "unavailable", "browser fetch budget exhausted")
}

// Options configures a Reddit client.
type Options struct {
	Provider       string
	BaseURL        string
	OAuthAPIURL    string
	TokenURL       string
	ClientID       string
	ClientSecret   string
	AccessToken    string
	HTMLFallback   bool
	ThirdPartyURL  string
	ThirdPartyKey  string
	OldRedditURL   string
	ScraperURL     string
	ScraperKey     string
	SocialScrapeUseProxy bool
	LSFLowPriority bool
	FirecrawlURL   string
	FirecrawlKey   string
	UserAgent      string
}

// New creates a Reddit client with defaults.
func New(opts Options) *Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://www.reddit.com"
	}
	oauthAPIURL := opts.OAuthAPIURL
	if oauthAPIURL == "" {
		oauthAPIURL = "https://oautc.reddit.com"
	}
	tokenURL := opts.TokenURL
	if tokenURL == "" {
		tokenURL = "https://www.reddit.com/api/v1/access_token"
	}
	oldRedditURL := opts.OldRedditURL
	if oldRedditURL == "" {
		oldRedditURL = "https://old.reddit.com"
	}
	provider := opts.Provider
	if provider == "" {
		provider = "auto"
	}
	scraperURL := opts.ScraperURL
	if scraperURL == "" {
		scraperURL = opts.FirecrawlURL
	}
	scraperKey := opts.ScraperKey
	if scraperKey == "" {
		scraperKey = opts.FirecrawlKey
	}
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		oauthAPIURL:    strings.TrimRight(oauthAPIURL, "/"),
		tokenURL:       strings.TrimRight(tokenURL, "/"),
		provider:       strings.ToLower(strings.TrimSpace(provider)),
		clientID:       opts.ClientID,
		clientSecret:   opts.ClientSecret,
		accessToken:    opts.AccessToken,
		htmlFallback:   opts.HTMLFallback,
		thirdPartyURL:  strings.TrimRight(opts.ThirdPartyURL, "/"),
		thirdPartyKey:  opts.ThirdPartyKey,
		oldRedditURL:   strings.TrimRight(oldRedditURL, "/"),
		lsfLowPriority: opts.LSFLowPriority,
		scraperAPIURL:  strings.TrimRight(scraperURL, "/"),
		scraperAPIKey:  scraperKey,
		socialScrapeUseProxy: opts.SocialScrapeUseProxy,
		userAgent:      opts.UserAgent,
		http:           &http.Client{Timeout: 8 * time.Second},
		scrapeHTTP:     &http.Client{Timeout: 210 * time.Second},
		backoff:        map[string]time.Time{},
	}
}

// FetchLSF loads LivestreamFail posts for a streamer login.
func (c *Client) FetchLSF(ctx context.Context, login, period, sort string) ([]model.RedditPost, []model.SourceStatus) {
	return c.fetchLSF(ctx, login, period, sort, nil)
}

func (c *Client) fetchLSF(ctx context.Context, login, period, sort string, budget *browserFetchBudget) ([]model.RedditPost, []model.SourceStatus) {
	if c.baseURL == "" {
		return nil, []model.SourceStatus{sourceWithProvider("reddit_lsf", "none", "unavailable", "api url not configured")}
	}

	ctx, cancel := redditLSFRequestContext(ctx)
	defer cancel()

	provider := normalizeRedditProvider(c.provider)
	providersToTry := c.redditLSFProvidersToTry(provider)

	var statuses []model.SourceStatus

	for _, p := range providersToTry {
		var posts []model.RedditPost
		var status model.SourceStatus
		tried := false

		switch p {
		case "official":
			if c.clientID != "" || c.accessToken != "" {
				posts, status = c.fetchLSFOfficial(ctx, login, period, sort)
				tried = true
			}
		case "public_json":
			posts, status = c.fetchLSFJSON(ctx, login, period, sort)
			tried = true
		case "third_party":
			if c.thirdPartyURL != "" {
				posts, status = c.fetchLSFThirdParty(ctx, login, period, sort)
				tried = true
			}
		case "scraper":
			if c.scraperAPIKey != "" {
				posts, status = c.fetchLSFScraper(ctx, login, period, sort, budget)
				tried = true
			}
		}

		if tried {
			status = normalizeRedditLSFStatus(status)
			statuses = append(statuses, status)
			if status.State == "ready" && len(posts) > 0 {
				return posts, statuses
			}
			if p == "public_json" && status.State == "ready" && len(posts) == 0 && !c.lsfLowPriority {
				if recent, recentStatus := c.fetchLSFRecentHot(ctx, login, budget); len(recent) > 0 {
					statuses = append(statuses, recentStatus)
					return recent, statuses
				} else if recentStatus.State != "" {
					statuses = append(statuses, recentStatus)
				}
			}
		}
	}

	if !c.lsfLowPriority && (provider == "off" || provider == "auto") && !redditStatusContainsProvider(statuses, "public_json_hot") && !redditStatusContainsProvider(statuses, "scraper_hot") {
		if recent, recentStatus := c.fetchLSFRecentHot(ctx, login, budget); len(recent) > 0 {
			statuses = append(statuses, recentStatus)
			return recent, statuses
		} else if recentStatus.State != "" && !redditStatusContainsProvider(statuses, recentStatus.Provider) {
			statuses = append(statuses, recentStatus)
		}
	}

	if c.htmlFallback && provider != "scraper" && provider != "firecrawl" {
		htmlPosts, htmlStatus := c.fetchLSFHTML(ctx, login, period, sort)
		htmlStatus = normalizeRedditLSFStatus(htmlStatus)
		statuses = append(statuses, htmlStatus)
		if htmlStatus.State == "fallback" && len(htmlPosts) > 0 {
			return htmlPosts, statuses
		}
	} else {
		statuses = append(statuses, sourceWithProvider("reddit_lsf_html", "html", "unavailable", "html fallback disabled"))
	}
	return nil, statuses
}

// redditLSFProvidersToTry picks fetch paths for LSF highlights.
// When REDDIT_PROVIDER=off (compose default), use a lightweight chain:
// public Reddit JSON first, optional scraper when Reddit blocks JSON, then HTML fallback.
func (c *Client) redditLSFProvidersToTry(provider string) []string {
	if provider != "off" {
		providers := []string{provider}
		allProviders := []string{"official", "public_json", "third_party", "scraper"}
		for _, p := range allProviders {
			if p != provider {
				providers = append(providers, p)
			}
		}
		return providers
	}
	providers := []string{"public_json"}
	if c.scraperAPIKey != "" {
		providers = append(providers, "scraper")
	}
	return providers
}

// fetchLSFRecentHot scans the LSF hot feed and keeps posts that mention the streamer.
func (c *Client) fetchLSFOldRedditJSON(ctx context.Context, login string) ([]model.RedditPost, model.SourceStatus) {
	u, err := url.Parse(strings.TrimRight(c.oldRedditURL, "/") + "/r/LivestreamFail/hot.json")
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", "old_reddit_json", "error", err.Error())
	}
	q := u.Query()
	q.Set("limit", "25")
	q.Set("raw_json", "1")
	u.RawQuery = q.Encode()
	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", "old_reddit_json", "error", err.Error())
	}
	posts, status := c.doRedditListing(req, "old_reddit_json", login)
	if status.State != "ready" {
		return nil, status
	}
	filtered := filterRedditPostsForLogin(posts, login)
	if len(filtered) > 0 {
		return filtered, sourceWithProvider("reddit_lsf", "old_reddit_json", "ready", "")
	}
	return nil, sourceWithProvider("reddit_lsf", "old_reddit_json", "unavailable", "no hot posts matched this streamer")
}

// fetchLSFRecentHot scans the LSF hot feed and keeps posts that mention the streamer.
func (c *Client) fetchLSFRecentHot(ctx context.Context, login string, budget *browserFetchBudget) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := c.redditBackoffActive("public_json_hot"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "public_json_hot", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	// Docker/datacenter egress: old.reddit JSON is more reliable than www.
	if posts, status := c.fetchLSFOldRedditJSON(ctx, login); len(posts) > 0 {
		c.markRedditBackoff("public_json_hot", status)
		return posts, status
	}
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/r/LivestreamFail/hot.json")
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json_hot", "error", err.Error())
		c.markRedditBackoff("public_json_hot", status)
		return nil, status
	}
	q := u.Query()
	q.Set("limit", "25")
	q.Set("raw_json", "1")
	u.RawQuery = q.Encode()

	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json_hot", "error", err.Error())
		c.markRedditBackoff("public_json_hot", status)
		return nil, status
	}
	posts, status := c.doRedditListing(req, "public_json_hot", login)
	if status.State == "ready" {
		c.markRedditBackoff("public_json_hot", status)
		filtered := filterRedditPostsForLogin(posts, login)
		if len(filtered) > 0 {
			return filtered, sourceWithProvider("reddit_lsf", "public_json_hot", "ready", "")
		}
		return nil, sourceWithProvider("reddit_lsf", "public_json_hot", "unavailable", "no recent hot posts matched this streamer")
	}
	if status.State == "blocked" || status.State == "error" {
		if fallbackPosts, fallbackStatus := c.fetchLSFOldRedditJSON(ctx, login); len(fallbackPosts) > 0 {
			return fallbackPosts, fallbackStatus
		}
	}
	if c.scraperAPIKey != "" && (status.State == "blocked" || status.State == "error" || status.State == "unavailable") {
		hotURL := strings.TrimRight(c.oldRedditURL, "/") + "/r/livestreamfail/hot/"
		scrapePosts, scrapeStatus := c.scrapeRedditListingURL(ctx, hotURL, login, "scraper_hot", 90000, budget)
		filtered := filterRedditPostsForLogin(scrapePosts, login)
		if len(filtered) > 0 {
			return filtered, scrapeStatus
		}
		if len(scrapePosts) > 0 {
			scrapeStatus = sourceWithProvider("reddit_lsf", "scraper_hot", "unavailable", "no recent hot posts matched this streamer")
		}
		if scrapeStatus.State != "" && scrapeStatus.State != "ready" {
			return nil, scrapeStatus
		}
	}
	c.markRedditBackoff("public_json_hot", status)
	return nil, status
}

// fetchSubredditHot loads the hot feed for a subreddit beyond LSF.
func (c *Client) fetchSubredditHot(ctx context.Context, subreddit, login string) ([]model.RedditPost, model.SourceStatus) {
	subreddit = strings.TrimSpace(subreddit)
	if subreddit == "" {
		return nil, model.SourceStatus{Source: "reddit", Provider: "public_json_hot", State: "error", Message: "empty subreddit"}
	}
	key := "public_json_hot_" + strings.ToLower(subreddit)
	if until, ok := c.redditBackoffActive(key); ok {
		return nil, model.SourceStatus{Source: "reddit", Provider: key, State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/r/" + subreddit + "/hot.json")
	if err != nil {
		status := sourceWithProvider("reddit", key, "error", err.Error())
		c.markRedditBackoff(key, status)
		return nil, status
	}
	q := u.Query()
	q.Set("limit", "15")
	q.Set("raw_json", "1")
	u.RawQuery = q.Encode()
	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit", key, "error", err.Error())
		c.markRedditBackoff(key, status)
		return nil, status
	}
	posts, status := c.doRedditListing(req, key, login)
	if status.State != "ready" {
		if c.scraperAPIKey != "" && (status.State == "blocked" || status.State == "error") {
			pageURL := strings.TrimRight(c.baseURL, "/") + "/r/" + subreddit + "/hot/"
			scrapePosts, scrapeStatus := c.scrapeRedditListingURL(ctx, pageURL, login, key+"_scraper", 90000, nil)
			filtered := filterRedditPostsForLogin(scrapePosts, login)
			if len(filtered) > 0 {
				c.markRedditBackoff(key, scrapeStatus)
				return filtered, sourceWithProvider("reddit", key, "ready", "")
			}
			if scrapeStatus.State != "" {
				c.markRedditBackoff(key, scrapeStatus)
				return nil, scrapeStatus
			}
		}
		c.markRedditBackoff(key, status)
		return nil, status
	}
	c.markRedditBackoff(key, status)
	if login == "" {
		return posts, sourceWithProvider("reddit", key, "ready", "")
	}
	filtered := filterRedditPostsForLogin(posts, login)
	if len(filtered) == 0 {
		return nil, sourceWithProvider("reddit", key, "unavailable", "no hot posts matched this streamer")
	}
	return filtered, sourceWithProvider("reddit", key, "ready", "")
}

func filterRedditPostsForLogin(posts []model.RedditPost, login string) []model.RedditPost {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return posts
	}
	out := make([]model.RedditPost, 0, len(posts))
	for _, post := range posts {
		if redditPostMatchesLogin(post, login) {
			out = append(out, post)
		}
	}
	return out
}

func redditPostMatchesLogin(post model.RedditPost, login string) bool {
	if strings.Contains(strings.ToLower(post.Title), login) {
		return true
	}
	if strings.Contains(strings.ToLower(post.FlairText), login) {
		return true
	}
	for _, tag := range post.StreamerTags {
		if strings.Contains(strings.ToLower(tag), login) {
			return true
		}
	}
	return false
}

func normalizeRedditProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "official", "public_json", "third_party", "scraper", "firecrawl", "off":
		p := strings.ToLower(strings.TrimSpace(value))
		if p == "firecrawl" {
			return "scraper"
		}
		return p
	default:
		return "auto"
	}
}

func (c *Client) fetchLSFOfficial(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := c.redditBackoffActive("official"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "official", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	token, status := c.redditBearer(ctx)
	if status.State != "ready" {
		return nil, status
	}
	u, err := c.redditSearchURL(c.oauthAPIURL, "", login, period, sort)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "official", "error", err.Error())
		c.markRedditBackoff("official", status)
		return nil, status
	}
	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "official", "error", err.Error())
		c.markRedditBackoff("official", status)
		return nil, status
	}
	req.Header.Set("Authorization", "Bearer "+token)
	posts, status := c.doRedditListing(req, "official", login)
	c.markRedditBackoff("official", status)
	return posts, status
}

func (c *Client) fetchLSFJSON(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := c.redditBackoffActive("public_json"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "public_json", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := c.redditSearchURL(c.baseURL, ".json", login, period, sort)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json", "error", err.Error())
		c.markRedditBackoff("public_json", status)
		return nil, status
	}

	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json", "error", err.Error())
		c.markRedditBackoff("public_json", status)
		return nil, status
	}
	posts, status := c.doRedditListing(req, "public_json", login)
	c.markRedditBackoff("public_json", status)
	return posts, status
}

func (c *Client) fetchLSFThirdParty(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if c.thirdPartyURL == "" {
		return nil, sourceWithProvider("reddit_lsf", "third_party", "unavailable", "third-party url not configured")
	}
	if until, ok := c.redditBackoffActive("third_party"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "third_party", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := url.Parse(c.thirdPartyURL)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "third_party", "error", err.Error())
		c.markRedditBackoff("third_party", status)
		return nil, status
	}
	q := u.Query()
	q.Set("subreddit", "LivestreamFail")
	q.Set("q", login)
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	q.Set("limit", "8")
	u.RawQuery = q.Encode()
	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "third_party", "error", err.Error())
		c.markRedditBackoff("third_party", status)
		return nil, status
	}
	if c.thirdPartyKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.thirdPartyKey)
	}
	posts, status := c.doRedditListing(req, "third_party", login)
	c.markRedditBackoff("third_party", status)
	return posts, status
}

func (c *Client) fetchLSFScraper(ctx context.Context, login, period, sort string, budget *browserFetchBudget) ([]model.RedditPost, model.SourceStatus) {
	if c.scraperAPIKey == "" {
		return nil, sourceWithProvider("reddit_lsf", "scraper", "unavailable", "scraper api key not configured")
	}
	if until, ok := c.redditBackoffActive("scraper"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "scraper", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	if c.lsfLowPriority {
		if oldURL := c.redditOldSearchURL(login, period, sort); oldURL != "" {
			posts, status := c.scrapeRedditListingURL(ctx, oldURL, login, "scraper", 45000, budget)
			c.markRedditBackoff("scraper", status)
			if len(posts) > 0 {
				return posts, status
			}
			if status.State == "" || status.State == "ready" {
				status = sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search did not contain posts for this streamer")
			}
			return nil, status
		}
		return nil, sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search url not configured")
	}
	var lastStatus model.SourceStatus
	for _, pageURL := range c.redditScraperSearchURLs(login, period, sort) {
		posts, status := c.scrapeRedditListingURL(ctx, pageURL, login, "scraper", 120000, budget)
		lastStatus = status
		if len(posts) > 0 {
			c.markRedditBackoff("scraper", status)
			return posts, status
		}
	}
	status := lastStatus
	if status.State == "" {
		status = sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search did not contain posts for this streamer")
	} else if status.State == "ready" {
		status = sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search did not contain posts for this streamer")
	}
	hotURL := strings.TrimRight(c.baseURL, "/") + "/r/LivestreamFail/hot/"
	hotPosts, hotStatus := c.scrapeRedditListingURL(ctx, hotURL, login, "scraper_hot", 90000, budget)
	filteredHot := filterRedditPostsForLogin(hotPosts, login)
	if len(filteredHot) > 0 {
		c.markRedditBackoff("scraper", hotStatus)
		return filteredHot, hotStatus
	}
	if len(hotPosts) > 0 {
		hotStatus = sourceWithProvider("reddit_lsf", "scraper_hot", "unavailable", "no recent hot posts matched this streamer")
	}
	if hotStatus.State != "" && hotStatus.State != "ready" {
		c.markRedditBackoff("scraper", hotStatus)
		return nil, hotStatus
	}
	c.markRedditBackoff("scraper", status)
	return nil, status
}

func redditScraperSiteProfile(pageURL string) string {
	lower := strings.ToLower(pageURL)
	if strings.Contains(lower, "old.reddit.com") {
		return "reddit_search"
	}
	return "social_public"
}

func (c *Client) scrapeRedditListingURL(ctx context.Context, pageURL, login, provider string, timeoutMs int, budget *browserFetchBudget) ([]model.RedditPost, model.SourceStatus) {
	if !budget.trySpend() {
		return nil, browserBudgetStatus("reddit_lsf", provider)
	}
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	body, _ := json.Marshal(map[string]any{
		"url":             pageURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"siteProfile":     redditScraperSiteProfile(pageURL),
		"maxAge":          600000,
		"timeout":         timeoutMs,
		"useProxy":        c.socialScrapeUseProxy,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.scraperAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.scraperAPIKey)
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.doScraperRequest(req)
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state := "error"
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
			state = "blocked"
		}
		return nil, sourceWithProvider("reddit_lsf", provider, state, fmt.Sprintf("status %d", resp.StatusCode))
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&out); err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	if !out.Success && out.Error != "" {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", out.Error)
	}
	htmlBody := out.Data.HTML
	if htmlBody == "" {
		htmlBody = out.Data.RawHTML
	}
	if htmlBody == "" {
		htmlBody = out.Data.Markdown
	}
	trimmed := strings.TrimSpace(htmlBody)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if posts, err := decodeRedditPosts([]byte(trimmed), c.baseURL, login, "scraper"); err == nil && len(posts) > 0 {
			return posts, sourceWithProvider("reddit_lsf", provider, "ready", "")
		}
	}
	posts := ParseHTMLListing(htmlBody, c.baseURL, login)
	status := sourceWithProvider("reddit_lsf", provider, "ready", "")
	if len(posts) == 0 {
		status = sourceWithProvider("reddit_lsf", provider, "unavailable", "scrape did not contain usable posts")
	}
	return posts, status
}

func (c *Client) doScraperRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.scrapeHTTP.Do(req)
	if err == nil || !scraper.IsTransientError(err) || req.Body == nil || req.GetBody == nil {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
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
	return c.scrapeHTTP.Do(retry)
}

func (c *Client) redditSearchURL(base, suffix, login, period, sort string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/r/LivestreamFail/search" + suffix)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", login)
	q.Set("restrict_sr", "1")
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	q.Set("limit", "8")
	if strings.HasSuffix(suffix, ".json") {
		q.Set("raw_json", "1")
	}
	u.RawQuery = q.Encode()
	return u, nil
}

func (c *Client) redditOldSearchURL(login, period, sort string) string {
	u, err := url.Parse("https://old.reddit.com/r/LivestreamFail/search")
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("q", login)
	q.Set("restrict_sr", "on")
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	q.Set("limit", "8")
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) redditScraperSearchURLs(login, period, sort string) []string {
	urls := make([]string, 0, 2)
	if u, err := c.redditSearchURL(c.baseURL, ".json", login, period, sort); err == nil {
		urls = append(urls, u.String())
	}
	if oldURL := c.redditOldSearchURL(login, period, sort); oldURL != "" {
		urls = append(urls, oldURL)
	}
	return urls
}

func (c *Client) newRedditGet(ctx context.Context, raw string) (*http.Request, error) {
	if strings.TrimSpace(c.userAgent) == "" {
		return nil, fmt.Errorf("user agent is required for reddit requests")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json,text/html;q=0.8")
	return req, nil
}

func (c *Client) doRedditListing(req *http.Request, provider, login string) ([]model.RedditPost, model.SourceStatus) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, sourceWithProvider("reddit_lsf", provider, "blocked", fmt.Sprintf("status %d", resp.StatusCode))
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		state := "error"
		if strings.Contains(strings.ToLower(string(snippet)), "blocked by network security") {
			state = "blocked"
		}
		return nil, sourceWithProvider("reddit_lsf", provider, state, "non-json response")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	posts, err := decodeRedditPosts(body, c.baseURL, login, provider)
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	return posts, sourceWithProvider("reddit_lsf", provider, "ready", "")
}

func decodeRedditPosts(body []byte, redditBaseURL, login, provider string) ([]model.RedditPost, error) {
	var wrapper struct {
		Items []model.RedditPost `json:"items"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&wrapper); err == nil && wrapper.Items != nil {
		for i := range wrapper.Items {
			if strings.HasPrefix(wrapper.Items[i].Permalink, "/") {
				wrapper.Items[i].Permalink = strings.TrimRight(redditBaseURL, "/") + wrapper.Items[i].Permalink
			}
			if wrapper.Items[i].URL == "" {
				wrapper.Items[i].URL = wrapper.Items[i].Permalink
			}
			if strings.TrimSpace(wrapper.Items[i].Provider) == "" {
				wrapper.Items[i].Provider = provider
			}
			enrichRedditTag(&wrapper.Items[i], login)
		}
		return wrapper.Items, nil
	}
	var listing struct {
		Data struct {
			Children []struct {
				Data struct {
					ID                string               `json:"id"`
					Title             string               `json:"title"`
					URL               string               `json:"url"`
					Permalink         string               `json:"permalink"`
					Thumbnail         string               `json:"thumbnail"`
					Author            string               `json:"author"`
					Subreddit         string               `json:"subreddit"`
					LinkFlairText     string               `json:"link_flair_text"`
					LinkFlairRichtext []redditRichTextPart `json:"link_flair_richtext"`
					Score             int                  `json:"score"`
					NumComments       int                  `json:"num_comments"`
					CreatedUTC        float64              `json:"created_utc"`
					Preview           struct {
						Images []struct {
							Source struct {
								URL string `json:"url"`
							} `json:"source"`
						} `json:"images"`
					} `json:"preview"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&listing); err != nil {
		return nil, err
	}

	posts := make([]model.RedditPost, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		item := child.Data
		permalink := item.Permalink
		if strings.HasPrefix(permalink, "/") {
			permalink = strings.TrimRight(redditBaseURL, "/") + permalink
		}
		thumbnail := ""
		if len(item.Preview.Images) > 0 {
			thumbnail = unescapeJSONURL(item.Preview.Images[0].Source.URL)
		}
		if thumbnail == "" {
			thumbnail = NormalizeRedditThumb(item.Thumbnail)
		}
		post := model.RedditPost{
			ID:         item.ID,
			Title:      item.Title,
			URL:        item.URL,
			Permalink:  permalink,
			Thumbnail:  thumbnail,
			Author:     item.Author,
			Score:      item.Score,
			Comments:   item.NumComments,
			CreatedUTC: int64(item.CreatedUTC),
			Subreddit:  item.Subreddit,
			FlairText:  redditFlairText(item.LinkFlairText, item.LinkFlairRichtext),
			Provider:   provider,
		}
		enrichRedditTag(&post, login)
		posts = append(posts, post)
	}
	return posts, nil
}

func (c *Client) fetchLSFHTML(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/r/LivestreamFail/search")
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}
	q := u.Query()
	q.Set("q", login)
	q.Set("restrict_sr", "1")
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	u.RawQuery = q.Encode()

	req, err := c.newRedditGet(ctx, u.String())
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "blocked", fmt.Sprintf("status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}
	posts := ParseHTMLListing(string(body), c.baseURL, login)
	if len(posts) == 0 {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "unavailable", "public listing did not contain usable posts")
	}
	return posts, sourceWithProvider("reddit_lsf_html", "html", "fallback", "json listing unavailable")
}

func (c *Client) redditBearer(ctx context.Context) (string, model.SourceStatus) {
	if c.accessToken != "" {
		return c.accessToken, sourceWithProvider("reddit_lsf", "official", "ready", "using configured access token")
	}
	if c.clientID == "" {
		return "", sourceWithProvider("reddit_lsf", "official", "unavailable", "reddit client id not configured")
	}
	if strings.TrimSpace(c.userAgent) == "" {
		return "", sourceWithProvider("reddit_lsf", "official", "error", "user agent is required for reddit requests")
	}
	c.mu.Lock()
	if c.token.AccessToken != "" && time.Until(c.token.ExpiresAt) > time.Minute {
		token := c.token.AccessToken
		c.mu.Unlock()
		return token, sourceWithProvider("reddit_lsf", "official", "ready", "using cached oauth token")
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", sourceWithProvider("reddit_lsf", "official", "error", err.Error())
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", sourceWithProvider("reddit_lsf", "official", "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", sourceWithProvider("reddit_lsf", "official", "blocked", fmt.Sprintf("token status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", sourceWithProvider("reddit_lsf", "official", "error", err.Error())
	}
	if out.AccessToken == "" {
		return "", sourceWithProvider("reddit_lsf", "official", "error", "oauth token response missing access_token")
	}
	expires := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.ExpiresIn <= 0 {
		expires = time.Now().Add(10 * time.Minute)
	}
	c.mu.Lock()
	c.token = tokenCache{AccessToken: out.AccessToken, ExpiresAt: expires}
	c.mu.Unlock()
	return out.AccessToken, sourceWithProvider("reddit_lsf", "official", "ready", "oauth token refreshed")
}

func (c *Client) redditBackoffActive(provider string) (time.Time, bool) {
	c.mu.Lock()
	until, ok := c.backoff[provider]
	active := ok && time.Now().Before(until)
	if !active {
		if ok {
			delete(c.backoff, provider)
		}
		c.mu.Unlock()
		return time.Time{}, false
	}
	c.mu.Unlock()
	return until, true
}

func (c *Client) markRedditBackoff(provider string, status model.SourceStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if redditLSFStatusInterrupted(status) {
		delete(c.backoff, provider)
		return
	}
	if status.State == "blocked" || status.State == "error" {
		c.backoff[provider] = time.Now().Add(45 * time.Second)
		return
	}
	delete(c.backoff, provider)
}

var redditPostRe = regexp.MustCompile(`(?is)<a[^>]+href="([^"]*/r/[^/]+/comments/[^"]+)"[^>]*>(.*?)</a>`)
var redditThingMarkerRe = regexp.MustCompile(`(?is)(?:\bdata-fullname="t3_[^"]+"|\bid-t3_[a-z0-9]+)`)
var redditTitleAnchorRe = regexp.MustCompile(`(?is)<a[^>]*\bclass="[^"]*\btitle\b[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
var redditCommentsAnchorRe = regexp.MustCompile(`(?is)<a[^>]*\bclass="[^"]*\bcomments\b[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
var redditCommentOnlyTitleRe = regexp.MustCompile(`(?i)^\d+\s+comments?$`)
var redditCommentCountFromTextRe = regexp.MustCompile(`(?i)^(\d+)\s+comments?$`)
var redditCommentMDRe = regexp.MustCompile(`(?is)<div[^>]*\bclass="[^"]*\bmd\b[^"]*"[^>]*>(.*?)</div>`)
var redditShredditPostRe = regexp.MustCompile(`(?is)<shreddit-post\b[^>]*>`)
var redditPermalinkAttrRe = regexp.MustCompile(`(?i)permalink="(/r/[^/]+/comments/[^"]+)"`)
var redditPostTitleAttrRe = regexp.MustCompile(`(?i)post-title="([^"]+)"`)
var redditScoreAttrRe = regexp.MustCompile(`(?i)\bscore="(\d+)"`)
var redditCommentCountAttrRe = regexp.MustCompile(`(?i)\bcomment-count="(\d+)"`)
var redditFlairTextAttrRe = regexp.MustCompile(`(?i)\bflair-text="([^"]+)"`)
var redditCreatedTimestampAttrRe = regexp.MustCompile(`(?i)\bcreated-timestamp="([^"]+)"`)
var redditThingScoreAttrRe = regexp.MustCompile(`(?i)\bdata-score="(-?\d+)"`)
var redditThingScoreTitleRe = regexp.MustCompile(`(?i)\bclass="[^"]*\bscore\b[^"]*"[^>]*\btitle="(-?\d+)\s+points?"`)
var redditThingScoreTextRe = regexp.MustCompile(`(?is)<div[^>]*\bclass="[^"]*\bscore\b[^"]*"[^>]*>\s*(-?\d+)\s*</div>`)
var redditTitleVoteSuffixRe = regexp.MustCompile(`(?i)\s+(\d+)\s+votes?\s*[•·]\s*(\d+)\s+comments?\s*$`)
var tagRe = regexp.MustCompile(`(?is)<[^>]+>`)

type redditRichTextPart struct {
	Text string `json:"t"`
}

func redditThingScore(chunk string) int {
	if n := redditAttrInt(chunk, redditThingScoreAttrRe); n != 0 {
		return n
	}
	if match := redditThingScoreTitleRe.FindStringSubmatch(chunk); len(match) >= 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil && n > 0 {
			return n
		}
	}
	if match := redditThingScoreTextRe.FindStringSubmatch(chunk); len(match) >= 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func redditFlairText(text string, rich []redditRichTextPart) string {
	if strings.TrimSpace(text) != "" {
		return strings.Join(strings.Fields(html.UnescapeString(text)), " ")
	}
	parts := make([]string, 0, len(rich))
	for _, item := range rich {
		if t := strings.TrimSpace(item.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(strings.Join(parts, " "))), " ")
}

func enrichRedditTag(post *model.RedditPost, login string) {
	if post == nil {
		return
	}
	if strings.TrimSpace(post.FlairText) != "" {
		post.StreamerTags = deriveStreamerTags(login, post.FlairText, true)
		return
	}
	post.StreamerTags = deriveStreamerTags(login, post.Title, false)
}

func deriveStreamerTags(login, source string, allowGeneric bool) []string {
	login = strings.ToLower(strings.TrimSpace(login))
	source = strings.TrimSpace(source)
	if source == "" {
		return []string{}
	}
	tags := []string{}
	seen := map[string]struct{}{}
	add := func(tag string) {
		tag = strings.Join(strings.Fields(strings.Trim(tag, " \t\r\n#[](){}:;,.|/")), " ")
		if tag == "" {
			return
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
	}
	for _, part := range regexp.MustCompile(`(?i)\s*(?:,|/|\||&|\band\b)\s*`).Split(source, -1) {
		clean := strings.TrimSpace(part)
		if clean == "" {
			continue
		}
		if login != "" {
			lower := strings.ToLower(clean)
			if strings.Contains(lower, login) || strings.Contains(login, lower) {
				add(clean)
				continue
			}
		}
		if strings.TrimSpace(source) == clean && login == "" && allowGeneric {
			add(clean)
		}
	}
	if len(tags) == 0 && login != "" && strings.Contains(strings.ToLower(source), login) {
		add(login)
	}
	if len(tags) == 0 && allowGeneric && strings.TrimSpace(source) != "" && strings.TrimSpace(source) == postSafeFlair(source) {
		add(source)
	}
	return tags
}

func postSafeFlair(source string) string {
	lower := strings.ToLower(strings.TrimSpace(source))
	switch lower {
	case "clip", "clips", "twitch", "lsf", "livestreamfail", "drama", "meta":
		return ""
	default:
		return strings.TrimSpace(source)
	}
}

func redditAttrFirst(tag string, re *regexp.Regexp) string {
	match := re.FindStringSubmatch(tag)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func redditAttrInt(tag string, re *regexp.Regexp) int {
	match := re.FindStringSubmatch(tag)
	if len(match) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(match[1]))
	return n
}

func redditCreatedUnix(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return float64(ts.Unix())
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return float64(ts.Unix())
	}
	return 0
}

func cleanRedditListingTitle(title, flair string, score, comments *int) string {
	title = strings.TrimSpace(html.UnescapeString(title))
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return ""
	}
	if m := redditTitleVoteSuffixRe.FindStringSubmatch(title); len(m) >= 3 {
		if *score == 0 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				*score = n
			}
		}
		if *comments == 0 {
			if n, err := strconv.Atoi(m[2]); err == nil {
				*comments = n
			}
		}
		title = strings.TrimSpace(redditTitleVoteSuffixRe.ReplaceAllString(title, ""))
	}
	flair = strings.TrimSpace(html.UnescapeString(flair))
	if flair != "" && strings.HasSuffix(title, flair) {
		title = strings.TrimSpace(strings.TrimSuffix(title, flair))
	}
	return title
}

type parsedRedditListingPost struct {
	title       string
	flair       string
	permalink   string
	externalURL string
	score       int
	comments    int
	createdUnix float64
}

func isRedditCommentOnlyTitle(title string) bool {
	return redditCommentOnlyTitleRe.MatchString(strings.TrimSpace(title))
}

func redditTitleFromPermalinkSlug(permalink string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(permalink), "/"), "/")
	for i, part := range parts {
		if strings.EqualFold(part, "comments") && i+2 < len(parts) {
			slug := strings.TrimSpace(parts[i+2])
			if slug == "" {
				return ""
			}
			return strings.Join(strings.Fields(strings.ReplaceAll(slug, "_", " ")), " ")
		}
	}
	return ""
}

func isRedditCommentsPath(raw string) bool {
	return strings.Contains(strings.ToLower(raw), "/comments/")
}

func normalizeRedditHref(redditBaseURL, raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		return strings.TrimRight(redditBaseURL, "/") + raw
	}
	return raw
}

func finalizeRedditListingTitle(title, flair, permalink string, score, comments *int) string {
	title = cleanRedditListingTitle(title, flair, score, comments)
	if title == "" || isRedditCommentOnlyTitle(title) {
		title = redditTitleFromPermalinkSlug(permalink)
	}
	if isRedditCommentOnlyTitle(title) {
		return ""
	}
	return title
}

func preferRedditListingPost(existing, incoming parsedRedditListingPost) parsedRedditListingPost {
	if isRedditCommentOnlyTitle(existing.title) && !isRedditCommentOnlyTitle(incoming.title) {
		return incoming
	}
	if !isRedditCommentOnlyTitle(existing.title) && isRedditCommentOnlyTitle(incoming.title) {
		return existing
	}
	if len(incoming.title) > len(existing.title) {
		existing.title = incoming.title
	}
	if existing.externalURL == "" && incoming.externalURL != "" {
		existing.externalURL = incoming.externalURL
	}
	if existing.permalink == "" && incoming.permalink != "" {
		existing.permalink = incoming.permalink
	}
	if existing.comments == 0 && incoming.comments > 0 {
		existing.comments = incoming.comments
	}
	if existing.score == 0 && incoming.score > 0 {
		existing.score = incoming.score
	}
	if existing.flair == "" && incoming.flair != "" {
		existing.flair = incoming.flair
	}
	return existing
}

// ParseHTMLListing extracts Reddit posts from an HTML listing page.
func ParseHTMLListing(body, redditBaseURL, login string) []model.RedditPost {
	byID := map[string]parsedRedditListingPost{}
	upsert := func(in parsedRedditListingPost) {
		in.permalink = normalizeRedditHref(redditBaseURL, in.permalink)
		in.externalURL = normalizeRedditHref(redditBaseURL, in.externalURL)
		scorePtr, commentsPtr := in.score, in.comments
		title := finalizeRedditListingTitle(in.title, in.flair, in.permalink, &scorePtr, &commentsPtr)
		if title == "" || len(title) < 4 {
			return
		}
		id := redditIDFromURL(in.permalink)
		if id == "" {
			id = redditIDFromURL(in.externalURL)
		}
		if id == "" {
			return
		}
		in.title = title
		in.score = scorePtr
		in.comments = commentsPtr
		if existing, ok := byID[id]; ok {
			byID[id] = preferRedditListingPost(existing, in)
			return
		}
		byID[id] = in
	}

	for _, chunk := range oldRedditThingChunks(body) {
		var titleHref, titleText, commentsHref, commentsText string
		if m := redditTitleAnchorRe.FindStringSubmatch(chunk); len(m) >= 3 {
			titleHref, titleText = m[1], strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(m[2], "")))
		}
		if m := redditCommentsAnchorRe.FindStringSubmatch(chunk); len(m) >= 3 {
			commentsHref, commentsText = m[1], strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(m[2], "")))
		}
		permalink := commentsHref
		if permalink == "" && isRedditCommentsPath(titleHref) {
			permalink = titleHref
		}
		externalURL := ""
		if titleHref != "" && !isRedditCommentsPath(titleHref) {
			externalURL = titleHref
		}
		comments := 0
		if m := redditCommentCountFromTextRe.FindStringSubmatch(commentsText); len(m) >= 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil {
				comments = n
			}
		}
		score := redditThingScore(chunk)
		upsert(parsedRedditListingPost{
			title:       titleText,
			permalink:   permalink,
			externalURL: externalURL,
			score:       score,
			comments:    comments,
		})
	}

	for _, tag := range redditShredditPostRe.FindAllString(body, 32) {
		permalink := redditAttrFirst(tag, redditPermalinkAttrRe)
		if permalink == "" {
			continue
		}
		upsert(parsedRedditListingPost{
			title:       redditAttrFirst(tag, redditPostTitleAttrRe),
			flair:       redditAttrFirst(tag, redditFlairTextAttrRe),
			permalink:   permalink,
			score:       redditAttrInt(tag, redditScoreAttrRe),
			comments:    redditAttrInt(tag, redditCommentCountAttrRe),
			createdUnix: redditCreatedUnix(redditAttrFirst(tag, redditCreatedTimestampAttrRe)),
		})
	}
	for _, match := range redditPostRe.FindAllStringSubmatch(body, 16) {
		title := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(match[2], "")))
		if isRedditCommentOnlyTitle(title) {
			continue
		}
		upsert(parsedRedditListingPost{
			title:     title,
			permalink: match[1],
		})
	}

	out := make([]model.RedditPost, 0, len(byID))
	for id, parsed := range byID {
		permalink := parsed.permalink
		if permalink == "" {
			permalink = parsed.externalURL
		}
		externalURL := parsed.externalURL
		if externalURL == permalink || isRedditCommentsPath(externalURL) {
			externalURL = ""
		}
		postURL := externalURL
		if postURL == "" {
			postURL = permalink
		}
		post := model.RedditPost{
			ID:         id,
			Title:      parsed.title,
			URL:        postURL,
			Permalink:  permalink,
			Subreddit:  "LivestreamFail",
			Score:      parsed.score,
			Comments:   parsed.comments,
			FlairText:  parsed.flair,
			CreatedUTC: int64(parsed.createdUnix),
		}
		enrichRedditTag(&post, login)
		out = append(out, post)
	}
	if len(out) > 28 {
		return out[:28]
	}
	return out
}

func oldRedditThingChunks(body string) []string {
	idxs := redditThingMarkerRe.FindAllStringIndex(body, -1)
	if len(idxs) == 0 {
		return nil
	}
	chunks := make([]string, 0, len(idxs))
	for i, loc := range idxs {
		start := loc[0]
		end := len(body)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		chunks = append(chunks, body[start:end])
	}
	return chunks
}

// ParseHTMLComments extracts top-level comment bodies from old.reddit HTML.
func ParseHTMLComments(body string) []string {
	out := make([]string, 0, 16)
	seen := map[string]struct{}{}
	for _, match := range redditCommentMDRe.FindAllStringSubmatch(body, 40) {
		if len(match) < 2 {
			continue
		}
		text := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(match[1], " ")))
		text = strings.Join(strings.Fields(text), " ")
		if text == "" || text == "[deleted]" || text == "[removed]" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
	}
	return out
}

func (c *Client) scrapeRedditPageHTML(ctx context.Context, pageURL, provider string, timeoutMs int, budget *browserFetchBudget) (string, model.SourceStatus) {
	if !budget.trySpend() {
		return "", browserBudgetStatus("reddit", provider)
	}
	if timeoutMs <= 0 {
		timeoutMs = 90000
	}
	body, _ := json.Marshal(map[string]any{
		"url":             pageURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"siteProfile":     redditScraperSiteProfile(pageURL),
		"maxAge":          600000,
		"timeout":         timeoutMs,
		"useProxy":        c.socialScrapeUseProxy,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.scraperAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", sourceWithProvider("reddit", provider, "error", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.scraperAPIKey)
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.doScraperRequest(req)
	if err != nil {
		return "", sourceWithProvider("reddit", provider, "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state := "error"
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
			state = "blocked"
		}
		return "", sourceWithProvider("reddit", provider, state, fmt.Sprintf("status %d", resp.StatusCode))
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&out); err != nil {
		return "", sourceWithProvider("reddit", provider, "error", err.Error())
	}
	if !out.Success && out.Error != "" {
		return "", sourceWithProvider("reddit", provider, "error", out.Error)
	}
	htmlBody := out.Data.HTML
	if htmlBody == "" {
		htmlBody = out.Data.RawHTML
	}
	if htmlBody == "" {
		htmlBody = out.Data.Markdown
	}
	if strings.TrimSpace(htmlBody) == "" {
		return "", sourceWithProvider("reddit", provider, "unavailable", "scrape did not contain usable html")
	}
	return htmlBody, sourceWithProvider("reddit", provider, "ready", "")
}

func commentJSONURL(permalink, baseURL string) (string, error) {
	pageURL := redditCommentsPageURL(permalink, baseURL)
	if pageURL == "" {
		return "", fmt.Errorf("empty permalink")
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimSuffix(u.Path, "/")
	u.Path = path + ".json"
	q := u.Query()
	q.Set("limit", "20")
	q.Set("depth", "1")
	q.Set("raw_json", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) fetchCommentBodiesJSON(ctx context.Context, permalink string) ([]string, error) {
	bases := []string{c.oldRedditURL, c.baseURL}
	var lastErr error
	for _, base := range bases {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		jsonURL, err := commentJSONURL(permalink, base)
		if err != nil {
			lastErr = err
			continue
		}
		req, err := c.newRedditGet(ctx, jsonURL)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("reddit comments status %d", resp.StatusCode)
			continue
		}
		texts, err := decodeCommentBodies(body)
		if err != nil {
			lastErr = err
			continue
		}
		if len(texts) > 0 {
			return texts, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("reddit comments unavailable")
	}
	return nil, lastErr
}

func (c *Client) fetchCommentBodies(ctx context.Context, permalink string, budget *browserFetchBudget) ([]string, error) {
	texts, jsonErr := c.fetchCommentBodiesJSON(ctx, permalink)
	if jsonErr == nil && len(texts) > 0 {
		return texts, nil
	}
	if c.scraperAPIKey == "" {
		return texts, jsonErr
	}
	pageURL := redditCommentsPageURL(permalink, c.oldRedditURL)
	htmlBody, status := c.scrapeRedditPageHTML(ctx, pageURL, "scraper_comments", 90000, budget)
	if status.State != "" && status.State != "ready" {
		if jsonErr != nil {
			return nil, jsonErr
		}
		return nil, fmt.Errorf("reddit comments scrape %s: %s", status.State, status.Message)
	}
	scraped := ParseHTMLComments(htmlBody)
	if len(scraped) > 0 {
		return scraped, nil
	}
	if jsonErr != nil {
		return nil, jsonErr
	}
	return nil, nil
}

func redditCommentsPageURL(permalink, oldRedditBase string) string {
	permalink = strings.TrimSpace(permalink)
	if permalink == "" {
		return ""
	}
	if strings.HasPrefix(permalink, "http://") || strings.HasPrefix(permalink, "https://") {
		u, err := url.Parse(permalink)
		if err != nil {
			return permalink
		}
		u.Host = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(u.Host), "www."), "old.")
		if oldRedditBase != "" {
			if base, err := url.Parse(oldRedditBase); err == nil && base.Host != "" {
				u.Scheme = base.Scheme
				u.Host = base.Host
			} else {
				u.Host = "old.reddit.com"
				u.Scheme = "https"
			}
		} else {
			u.Host = "old.reddit.com"
			u.Scheme = "https"
		}
		u.RawQuery = ""
		u.Fragment = ""
		path := strings.TrimSuffix(u.Path, "/")
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			return u.Scheme + "://" + u.Host + path + "/"
		}
		return u.Scheme + "://" + u.Host + strings.TrimSuffix(path, ".json") + "/"
	}
	base := strings.TrimRight(oldRedditBase, "/")
	if base == "" {
		base = "https://old.reddit.com"
	}
	return base + permalink
}

func redditIDFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part == "comments" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// NormalizeRedditThumb returns a usable preview URL from Reddit listing metadata.
func NormalizeRedditThumb(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	switch lower {
	case "self", "default", "nsfw", "spoiler", "image":
		return ""
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if strings.Contains(lower, "redditstatic.com") {
			return ""
		}
		return raw
	}
	if strings.HasPrefix(lower, "//") {
		return "https:" + raw
	}
	return ""
}
