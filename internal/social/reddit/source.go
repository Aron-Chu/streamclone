package reddit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/metadata/model"
	"streamclone/internal/social"
	"streamclone/internal/storygraph/reliability"
)

func init() {
	social.Register("reddit", func() (social.SocialSource, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		return NewSource(cfg), nil
	})
}

// Source adapts the LSF Reddit client to SocialSource.
type Source struct {
	client *Client
	cfg    config.Config
}

func NewSource(cfg config.Config) *Source {
	return &Source{
		cfg: cfg,
		client: New(Options{
			Provider:       cfg.RedditProvider,
			BaseURL:        cfg.RedditAPIURL,
			OAuthAPIURL:    cfg.RedditOAuthAPIURL,
			TokenURL:       cfg.RedditTokenURL,
			ClientID:       cfg.RedditClientID,
			ClientSecret:   cfg.RedditClientSecret,
			AccessToken:    cfg.RedditAccessToken,
			HTMLFallback:   cfg.RedditHTMLFallback,
			ThirdPartyURL:  cfg.RedditThirdPartyURL,
			ThirdPartyKey:  cfg.RedditThirdPartyKey,
			ScraperURL:     cfg.ScraperAPIURL,
			ScraperKey:     cfg.ScraperAPIKey,
			SocialScrapeUseProxy: cfg.SocialScrapeUseProxy,
			LSFLowPriority: cfg.RedditLSFLowPriority,
			UserAgent:      "streamclone/1.0",
		}),
	}
}

func (s *Source) Name() string { return "reddit" }

func (s *Source) Risk() reliability.Risk { return reliability.RiskPublicAPI }

func (s *Source) Capabilities() social.Caps {
	return social.Caps{Hydrate: true, Comments: true, RefreshMetrics: true, Backfill: true}
}

func (s *Source) Healthy(ctx context.Context) error {
	if !s.cfg.RedditCommercialOK && s.cfg.PulseWireEnabled {
		return fmt.Errorf("reddit commercial gate: set REDDIT_COMMERCIAL_OK=true")
	}
	if s.client == nil || s.client.baseURL == "" {
		return fmt.Errorf("reddit not configured")
	}
	return nil
}

// FetchPostMeta loads preview metadata for a Reddit thread URL.
func (s *Source) FetchPostMeta(ctx context.Context, postURL string) (PostMeta, bool) {
	if s.client == nil {
		return PostMeta{}, false
	}
	return s.client.FetchPostMeta(ctx, postURL, nil)
}

func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {
	login := ""
	if q.Entity.TwitchLogin != "" {
		login = q.Entity.TwitchLogin
	}
	var posts []model.RedditPost
	browserBudget := newBrowserFetchBudget(q.Budget.MaxBrowserFetches)
	if login == "" {
		var status model.SourceStatus
		posts, status = s.client.fetchLSFRecentHot(ctx, "", browserBudget)
		if err := redditFetchErr(status); err != nil && len(posts) == 0 {
			return social.Page{}, err
		}
		// Global trending uses LSF hot only. Extra subreddit scrapes compete with
		// Analytics' TwitchTracker/YouTube browser work on the shared scraper.
	} else {
		var statuses []model.SourceStatus
		posts, statuses = s.client.fetchLSF(ctx, login, "7d", "hot", browserBudget)
		if err := redditFetchErrs(posts, statuses); err != nil {
			return social.Page{}, err
		}
	}
	items := make([]social.Item, 0, len(posts))
	retention := time.Now().Add(time.Duration(s.cfg.SocialRetentionDays) * 24 * time.Hour)
	for _, p := range posts {
		if q.Budget.MaxItems > 0 && len(items) >= q.Budget.MaxItems {
			break
		}
		created := time.Unix(p.CreatedUTC, 0)
		if p.CreatedUTC <= 0 {
			// HTML/scraper listings often omit timestamps; treat as fresh ingest.
			created = time.Now()
		}
		if !q.Since.IsZero() && created.Before(q.Since) {
			continue
		}
		metrics := map[string]float64{"score": float64(p.Score), "comments": float64(p.Comments)}
		raw, _ := json.Marshal(p)
		sum := sha256.Sum256(raw)
		loginHint, displayHint := redditEntityHint(p)
		sourceURL := redditPostURL(p)
		text := strings.TrimSpace(p.Title)
		if cleaned := cleanRedditListingTitle(text, p.FlairText, &p.Score, &p.Comments); cleaned != "" {
			text = cleaned
		}
		if isRedditCommentOnlyTitle(text) || len(strings.TrimSpace(text)) < 4 {
			if slug := redditTitleFromPermalinkSlug(sourceURL); slug != "" {
				text = slug
			}
		}
		if strings.TrimSpace(p.URL) != "" && strings.TrimSpace(p.URL) != sourceURL {
			text = strings.TrimSpace(text + " " + strings.TrimSpace(p.URL))
		}
		item := social.Item{
			Source:            "reddit",
			Kind:              "post",
			ExternalID:        p.ID,
			URL:               sourceURL,
			Author:            p.Author,
			Text:              text,
			FlairText:         strings.TrimSpace(p.FlairText),
			CreatedAt:         created,
			Metrics:           metrics,
			EntityTwitchLogin: loginHint,
			EntityDisplayName: displayHint,
			Provenance:        social.Provenance{FetchedAt: time.Now(), SourceAPI: redditSourceAPI(p.Provider)},
			SnapshotSHA256:    sum[:],
			ExpiresAt:         retention,
		}
		if thumb := NormalizeRedditThumb(p.Thumbnail); thumb != "" {
			item.Media = []social.MediaRef{{Kind: "image", URL: thumb}}
		}
		items = append(items, item)
	}
	return social.Page{Items: items}, nil
}

// Backfill imports a bounded historical Reddit sweep.
func (s *Source) Backfill(ctx context.Context, q social.Query) (social.Page, error) {
	if q.Since.IsZero() {
		q.Since = time.Now().Add(-7 * 24 * time.Hour)
	}
	if q.Budget.MaxItems <= 0 {
		q.Budget.MaxItems = 24
	}
	return s.Search(ctx, q)
}

func boundedRedditSubreddits() []string {
	return []string{"Twitch", "esports", "LivestreamFail"}
}

func mergeRedditPosts(base, extra []model.RedditPost) []model.RedditPost {
	seen := map[string]struct{}{}
	out := make([]model.RedditPost, 0, len(base)+len(extra))
	for _, group := range [][]model.RedditPost{base, extra} {
		for _, post := range group {
			if post.ID == "" {
				continue
			}
			if _, ok := seen[post.ID]; ok {
				continue
			}
			seen[post.ID] = struct{}{}
			out = append(out, post)
		}
	}
	return out
}

func redditEntityHint(post model.RedditPost) (string, string) {
	for _, tag := range post.StreamerTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		return normalizeHandleGuess(tag), tag
	}
	if len(post.StreamerTags) == 0 && strings.TrimSpace(post.FlairText) != "" {
		return normalizeHandleGuess(post.FlairText), strings.TrimSpace(post.FlairText)
	}
	return "", ""
}

func redditPostURL(post model.RedditPost) string {
	permalink := strings.TrimSpace(post.Permalink)
	if permalink != "" {
		if strings.HasPrefix(permalink, "/") {
			return CanonicalPermalink("https://www.reddit.com" + permalink)
		}
		return CanonicalPermalink(permalink)
	}
	return CanonicalPermalink(strings.TrimSpace(post.URL))
}

func redditSourceAPI(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "official":
		return "reddit_oauth"
	case "public_json", "public_json_hot", "old_reddit_json", "":
		return "reddit_lsf"
	default:
		return "reddit_" + strings.ToLower(strings.TrimSpace(provider))
	}
}

func normalizeHandleGuess(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func redditFetchErr(status model.SourceStatus) error {
	switch status.State {
	case "error", "blocked":
		msg := strings.TrimSpace(status.Message)
		if msg == "" {
			return fmt.Errorf("reddit fetch %s", status.State)
		}
		return fmt.Errorf("reddit fetch %s: %s", status.State, msg)
	default:
		return nil
	}
}

func redditFetchErrs(posts []model.RedditPost, statuses []model.SourceStatus) error {
	if len(posts) > 0 {
		return nil
	}
	var last error
	for _, st := range statuses {
		if err := redditFetchErr(st); err != nil {
			last = err
		}
	}
	return last
}
