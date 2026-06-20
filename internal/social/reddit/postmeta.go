package reddit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// PostMeta is lightweight metadata from Reddit's public post JSON endpoint.
type PostMeta struct {
	Thumbnail   string
	Score       int
	Comments    int
	ExternalURL string
	SelfText    string
	IsSelf      bool
}

// FetchPostMeta loads thumbnail and engagement from a Reddit thread URL.
func FetchPostMeta(ctx context.Context, client *http.Client, userAgent, postURL string) (PostMeta, bool) {
	jsonURL := postJSONURL(postURL)
	if jsonURL == "" {
		return PostMeta{}, false
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jsonURL, nil)
	if err != nil {
		return PostMeta{}, false
	}
	if ua := strings.TrimSpace(userAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := client.Do(req)
	if err != nil {
		return PostMeta{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PostMeta{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return PostMeta{}, false
	}
	return parsePostMetaJSON(body)
}

func postJSONURL(postURL string) string {
	postURL = strings.TrimSpace(postURL)
	if postURL == "" {
		return ""
	}
	parsed, err := url.Parse(postURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.Contains(host, "reddit.com") {
		return ""
	}
	parsed.Scheme = "https"
	parsed.Host = "old.reddit.com"
	path := strings.TrimSuffix(parsed.Path, "/")
	if path == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		path += ".json"
	}
	parsed.Path = path
	q := parsed.Query()
	q.Set("raw_json", "1")
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func parsePostMetaJSON(body []byte) (PostMeta, bool) {
	var listing []struct {
		Data struct {
			Children []struct {
				Data struct {
					Thumbnail   string `json:"thumbnail"`
					URL         string `json:"url"`
					SelfText    string `json:"selftext"`
					IsSelf      bool   `json:"is_self"`
					Score       int    `json:"score"`
					NumComments int    `json:"num_comments"`
					Preview     struct {
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
	if err := json.Unmarshal(body, &listing); err != nil || len(listing) == 0 {
		return PostMeta{}, false
	}
	for _, child := range listing[0].Data.Children {
		data := child.Data
		thumb := ""
		if len(data.Preview.Images) > 0 {
			thumb = unescapeJSONURL(data.Preview.Images[0].Source.URL)
		}
		if thumb == "" {
			thumb = NormalizeRedditThumb(data.Thumbnail)
		}
		if thumb == "" && data.Score == 0 && data.NumComments == 0 && data.URL == "" {
			continue
		}
		external := strings.TrimSpace(data.URL)
		if isRedditPermalinkURL(external) {
			external = ""
		}
		return PostMeta{
			Thumbnail:   thumb,
			Score:       data.Score,
			Comments:    data.NumComments,
			ExternalURL: external,
			SelfText:    trimRedditSelfText(data.SelfText),
			IsSelf:      data.IsSelf,
		}, true
	}
	return PostMeta{}, false
}

func unescapeJSONURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "\\u0026", "&")
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	return raw
}

var ogImageRe = regexp.MustCompile(`(?is)<meta\s+[^>]*(?:property|name)=["'](?:og:image|twitter:image)["'][^>]*content=["']([^"']*)["']`)
var postMetaScoreJSONRe = regexp.MustCompile(`(?i)"score"\s*:\s*(-?\d+)`)
var postMetaCommentsJSONRe = regexp.MustCompile(`(?i)"num_comments"\s*:\s*(-?\d+)`)

func isRedditPermalinkURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	if !strings.Contains(strings.ToLower(parsed.Hostname()), "reddit.com") {
		return false
	}
	return strings.Contains(parsed.Path, "/comments/")
}

func trimRedditSelfText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[removed]" || raw == "[deleted]" {
		return ""
	}
	if len(raw) > 480 {
		raw = raw[:480] + "…"
	}
	return raw
}

// FetchPostMeta loads thumbnail and engagement, using scraper OpenGraph when Reddit JSON is blocked.
func (c *Client) FetchPostMeta(ctx context.Context, postURL string, budget *browserFetchBudget) (PostMeta, bool) {
	if meta, ok := FetchPostMeta(ctx, c.http, c.userAgent, postURL); ok {
		if meta.hasSignal() {
			return meta, true
		}
	}
	if c == nil || c.scraperAPIKey == "" {
		return PostMeta{}, false
	}
	if jsonURL := postJSONURL(postURL); jsonURL != "" {
		if body, status := c.scrapeRedditPageHTML(ctx, jsonURL, "scraper_postmeta_json", 45000, budget); status.State == "ready" {
			trimmed := strings.TrimSpace(body)
			if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
				if meta, ok := parsePostMetaJSON([]byte(trimmed)); ok && meta.hasSignal() {
					return meta, true
				}
			}
		}
	}
	htmlBody, status := c.scrapeRedditPageHTML(ctx, postURL, "scraper_postmeta", 45000, budget)
	if status.State != "" && status.State != "ready" {
		return PostMeta{}, false
	}
	meta := PostMeta{}
	if thumb := extractOGImage(htmlBody); thumb != "" {
		meta.Thumbnail = thumb
	}
	if meta.Thumbnail == "" {
		for _, post := range ParseHTMLListing(htmlBody, c.baseURL, "") {
			if thumb := NormalizeRedditThumb(post.Thumbnail); thumb != "" {
				meta.Thumbnail = thumb
				break
			}
		}
	}
	meta.Score, meta.Comments = extractPostEngagementFromHTML(htmlBody)
	if meta.hasSignal() {
		return meta, true
	}
	return PostMeta{}, false
}

func extractPostEngagementFromHTML(htmlBody string) (score, comments int) {
	if m := redditThingScoreAttrRe.FindStringSubmatch(htmlBody); len(m) >= 2 {
		score, _ = strconv.Atoi(m[1])
	}
	if m := redditCommentCountAttrRe.FindStringSubmatch(htmlBody); len(m) >= 2 {
		comments, _ = strconv.Atoi(m[1])
	}
	if score == 0 {
		if m := redditScoreAttrRe.FindStringSubmatch(htmlBody); len(m) >= 2 {
			score, _ = strconv.Atoi(m[1])
		}
	}
	if score == 0 {
		if m := postMetaScoreJSONRe.FindStringSubmatch(htmlBody); len(m) >= 2 {
			score, _ = strconv.Atoi(m[1])
		}
	}
	if comments == 0 {
		if m := postMetaCommentsJSONRe.FindStringSubmatch(htmlBody); len(m) >= 2 {
			comments, _ = strconv.Atoi(m[1])
		}
	}
	return score, comments
}

func (m PostMeta) hasSignal() bool {
	return strings.TrimSpace(m.Thumbnail) != "" || m.Score > 0 || m.Comments > 0
}

func extractOGImage(htmlBody string) string {
	if match := ogImageRe.FindStringSubmatch(htmlBody); len(match) >= 2 {
		return strings.TrimSpace(strings.ReplaceAll(match[1], "&amp;", "&"))
	}
	return ""
}
