package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"streamclone/internal/social"
)

func (s *Source) EnrichItemText(ctx context.Context, item *social.Item) {
	if item == nil || item.Source != "youtube" || strings.TrimSpace(s.cfg.YouTubeAPIKey) == "" {
		return
	}
	vid := strings.TrimSpace(item.ExternalID)
	if vid == "" {
		return
	}
	descriptions, err := s.fetchVideoDescriptions(ctx, []string{vid})
	if err != nil {
		return
	}
	if desc, ok := descriptions[vid]; ok && strings.TrimSpace(desc) != "" {
		item.Text = strings.TrimSpace(item.Text + " " + desc)
	}
}

func (s *Source) fetchVideoDescriptions(ctx context.Context, videoIDs []string) (map[string]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	u, _ := url.Parse(strings.TrimRight(s.cfg.YouTubeAPIBaseURL, "/") + "/videos")
	params := u.Query()
	params.Set("part", "snippet")
	params.Set("id", strings.Join(videoIDs, ","))
	params.Set("key", s.cfg.YouTubeAPIKey)
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube videos status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Description string `json:"description"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, item := range out.Items {
		if item.ID != "" {
			result[item.ID] = item.Snippet.Description
		}
	}
	return result, nil
}
