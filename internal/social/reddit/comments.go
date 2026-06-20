package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"streamclone/internal/social"
)

// Comments fetches top-level comment bodies for a Reddit post permalink.
func (s *Source) Comments(ctx context.Context, itemID string, q social.Query) (social.Page, error) {
	if s.client == nil {
		return social.Page{}, fmt.Errorf("reddit client not configured")
	}
	permalink := strings.TrimSpace(itemID)
	if permalink == "" {
		return social.Page{}, nil
	}
	if !strings.HasPrefix(permalink, "http") {
		permalink = strings.TrimRight(s.client.baseURL, "/") + permalink
	}
	u, err := url.Parse(permalink)
	if err != nil {
		return social.Page{}, err
	}
	if !strings.HasSuffix(strings.ToLower(u.Path), "/") {
		u.Path += "/"
	}
	u.Path += ".json"
	qv := u.Query()
	qv.Set("limit", "20")
	qv.Set("depth", "1")
	qv.Set("raw_json", "1")
	u.RawQuery = qv.Encode()

	req, err := s.client.newRedditGet(ctx, u.String())
	if err != nil {
		return social.Page{}, err
	}
	resp, err := s.client.http.Do(req)
	if err != nil {
		return social.Page{}, err
	}
	defer resp.Body.Close()

	var texts []string
	sourceAPI := "reddit_comments"
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
			return social.Page{}, fmt.Errorf("reddit comments status %d", resp.StatusCode)
		}
		if s.client.scraperAPIKey == "" {
			return social.Page{}, fmt.Errorf("reddit comments status %d", resp.StatusCode)
		}
		budget := newBrowserFetchBudget(q.Budget.MaxBrowserFetches)
		pageURL := redditCommentsPageURL(permalink, s.client.oldRedditURL)
		htmlBody, status := s.client.scrapeRedditPageHTML(ctx, pageURL, "scraper_comments", 90000, budget)
		if status.State != "ready" || strings.TrimSpace(htmlBody) == "" {
			msg := strings.TrimSpace(status.Message)
			if msg == "" {
				msg = fmt.Sprintf("reddit comments status %d", resp.StatusCode)
			}
			return social.Page{}, fmt.Errorf("%s", msg)
		}
		texts = ParseHTMLComments(htmlBody)
		sourceAPI = "reddit_comments_scraper"
	} else {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil {
			return social.Page{}, err
		}
		texts, err = decodeCommentBodies(body)
		if err != nil {
			return social.Page{}, err
		}
	}

	max := q.Budget.MaxItems
	if max <= 0 {
		max = 10
	}
	retention := time.Now().Add(time.Duration(s.cfg.SocialRetentionDays) * 24 * time.Hour)
	postID := redditIDFromURL(permalink)
	items := make([]social.Item, 0, minInt(len(texts), max))
	for i, text := range texts {
		if len(items) >= max {
			break
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		items = append(items, social.Item{
			Source:     "reddit",
			Kind:       "comment",
			ExternalID: fmt.Sprintf("%s-c%d", postID, i),
			URL:        permalink,
			Text:       text,
			Provenance: social.Provenance{FetchedAt: time.Now(), SourceAPI: sourceAPI},
			ExpiresAt:  retention,
		})
	}
	return social.Page{Items: items}, nil
}

func decodeCommentBodies(body []byte) ([]string, error) {
	var listing []struct {
		Data struct {
			Children []struct {
				Data struct {
					Body string `json:"body"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, err
	}
	if len(listing) < 2 {
		return nil, nil
	}
	var out []string
	for _, child := range listing[1].Data.Children {
		body := strings.TrimSpace(child.Data.Body)
		if body != "" && body != "[deleted]" && body != "[removed]" {
			out = append(out, body)
		}
	}
	return out, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ social.CommentFetcher = (*Source)(nil)
