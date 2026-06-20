package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"streamclone/internal/social/scraper"
)

type scrapedVideo struct {
	VideoID      string
	Title        string
	ChannelTitle string
}

var youtubeLinkIDPattern = regexp.MustCompile(`(?i)(?:watch\?v=|shorts/)([A-Za-z0-9_-]{11})`)
var errYouTubeBrowserBudgetExhausted = fmt.Errorf("youtube browser fetch budget exhausted")

func (s *Source) searchKeywordScrape(ctx context.Context, keyword string, maxResults int, since time.Time, maxBrowserFetches int, browserFetches *int) ([]scrapedVideo, int, error) {
	if maxResults <= 0 {
		maxResults = 4
	}
	if maxBrowserFetches < 0 {
		return nil, 0, errYouTubeBrowserBudgetExhausted
	}
	query := url.QueryEscape(strings.TrimSpace(keyword) + " clip")
	pageURL := "https://www.youtube.com/results?search_query=" + query
	client := scraper.New(scraper.Config{
		URL:       s.cfg.ScraperAPIURL,
		Key:       s.cfg.ScraperAPIKey,
		UserAgent: "streamclone/1.0",
	})
	var lastErr error
	delays := []time.Duration{0, 500 * time.Millisecond, 1500 * time.Millisecond}
	for _, delay := range delays {
		if maxBrowserFetches > 0 && browserFetches != nil && *browserFetches >= maxBrowserFetches {
			if lastErr != nil {
				return nil, 0, lastErr
			}
			return nil, 0, errYouTubeBrowserBudgetExhausted
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, 0, ctx.Err()
			case <-timer.C:
			}
		}
		if browserFetches != nil {
			*browserFetches++
		}
		htmlBody, err := client.FetchHTMLWithOptions(ctx, pageURL, scraper.FetchOptions{
			SiteProfile: "social_public",
			UseProxy:    s.cfg.SocialScrapeUseProxy,
			TimeoutMs:   90000,
		})
		if err != nil {
			lastErr = err
			continue
		}
		videos, err := parseYouTubeSearchHTMLLimit(htmlBody, maxResults)
		if err != nil {
			lastErr = err
			continue
		}
		if !since.IsZero() {
			filtered := make([]scrapedVideo, 0, len(videos))
			for _, v := range videos {
				filtered = append(filtered, v)
				if len(filtered) >= maxResults {
					break
				}
			}
			videos = filtered
		}
		return videos, 200, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("youtube scrape failed")
	}
	return nil, 0, lastErr
}

func parseYouTubeSearchHTML(html string) ([]scrapedVideo, error) {
	return parseYouTubeSearchHTMLLimit(html, 24)
}

func parseYouTubeSearchHTMLLimit(html string, maxResults int) ([]scrapedVideo, error) {
	raw, err := extractYTInitialData(html)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]scrapedVideo, 0, maxResults)
	collectVideoRenderers(root, seen, &out, maxResults)
	if len(out) == 0 {
		collectVideoLinks(html, seen, &out, maxResults)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no videos in ytInitialData")
	}
	return out, nil
}

func collectVideoLinks(html string, seen map[string]struct{}, out *[]scrapedVideo, max int) {
	if len(*out) >= max {
		return
	}
	for _, match := range youtubeLinkIDPattern.FindAllStringSubmatch(html, -1) {
		if len(*out) >= max {
			return
		}
		if len(match) < 2 {
			continue
		}
		vid := strings.TrimSpace(match[1])
		if vid == "" {
			continue
		}
		if _, ok := seen[vid]; ok {
			continue
		}
		seen[vid] = struct{}{}
		*out = append(*out, scrapedVideo{
			VideoID: vid,
			Title:   "YouTube video " + vid,
		})
	}
}

func extractYTInitialData(html string) ([]byte, error) {
	marker := "var ytInitialData = "
	idx := strings.Index(html, marker)
	if idx < 0 {
		marker = "ytInitialData = "
		idx = strings.Index(html, marker)
	}
	if idx < 0 {
		return nil, fmt.Errorf("ytInitialData not found")
	}
	start := idx + len(marker)
	end := balancedJSONEnd(html, start)
	if end <= start {
		return nil, fmt.Errorf("ytInitialData parse bounds")
	}
	return []byte(html[start:end]), nil
}

func balancedJSONEnd(s string, start int) int {
	if start >= len(s) || s[start] != '{' {
		return -1
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func collectVideoRenderers(node any, seen map[string]struct{}, out *[]scrapedVideo, max int) {
	if len(*out) >= max {
		return
	}
	switch v := node.(type) {
	case map[string]any:
		if vr, ok := v["videoRenderer"].(map[string]any); ok {
			appendVideoRenderer(vr, seen, out, max)
		}
		for _, child := range v {
			collectVideoRenderers(child, seen, out, max)
		}
	case []any:
		for _, child := range v {
			collectVideoRenderers(child, seen, out, max)
		}
	}
}

func appendVideoRenderer(vr map[string]any, seen map[string]struct{}, out *[]scrapedVideo, max int) {
	if len(*out) >= max {
		return
	}
	vid, _ := vr["videoId"].(string)
	vid = strings.TrimSpace(vid)
	if vid == "" {
		return
	}
	if _, ok := seen[vid]; ok {
		return
	}
	title := textRuns(vr["title"])
	if title == "" {
		return
	}
	channel := textRuns(vr["ownerText"])
	if channel == "" {
		channel = textRuns(vr["shortBylineText"])
	}
	seen[vid] = struct{}{}
	*out = append(*out, scrapedVideo{
		VideoID:      vid,
		Title:        title,
		ChannelTitle: channel,
	})
}

func textRuns(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	runs, ok := m["runs"].([]any)
	if !ok {
		if simple, ok := m["simpleText"].(string); ok {
			return strings.TrimSpace(simple)
		}
		return ""
	}
	var b strings.Builder
	for _, run := range runs {
		rm, ok := run.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := rm["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return strings.TrimSpace(b.String())
}
