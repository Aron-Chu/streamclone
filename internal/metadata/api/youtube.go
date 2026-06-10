package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/metadata/gql"
	"streamclone/internal/metadata/model"
)

const defaultYouTubeAPIBase = "https://www.googleapis.com/youtube/v3"

var (
	youtubeVideoIDRe     = regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
	youtubeTitleSimpleRe = regexp.MustCompile(`"title":\{"simpleText":"([^"]+)"\}`)
	youtubeTitleRunsRe   = regexp.MustCompile(`"title":\{"runs":\[\{"text":"([^"]+)"\}`)
	youtubeSubTextRe     = regexp.MustCompile(`"subscriberCountText":\{"simpleText":"([^"]+)"\}`)
)

type YouTubeOptions struct {
	Provider string
	APIKey   string
	APIURL   string
}

func (h *Handler) WithYouTubeOptions(opts YouTubeOptions) *Handler {
	if opts.Provider != "" {
		h.youtubeProvider = strings.ToLower(strings.TrimSpace(opts.Provider))
	}
	h.youtubeAPIKey = opts.APIKey
	if opts.APIURL != "" {
		h.youtubeAPIBaseURL = strings.TrimRight(opts.APIURL, "/")
	}
	if h.youtubeAPIBaseURL == "" {
		h.youtubeAPIBaseURL = defaultYouTubeAPIBase
	}
	if h.youtubeBackoff == nil {
		h.youtubeBackoff = map[string]time.Time{}
	}
	return h
}

func (h *Handler) channelYouTube(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	limit := parseLimit(r)
	if limit > 25 {
		limit = 25
	}
	handle := strings.TrimSpace(r.URL.Query().Get("handle"))
	youtubeURL := strings.TrimSpace(r.URL.Query().Get("youtubeUrl"))
	key := fmt.Sprintf("meta:channelyoutube:%s:%s:%s:%d", login, handle, youtubeURL, limit)
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchChannelYouTube(r.Context(), login, handle, youtubeURL, limit)
	})
	respond(w, data, stale, err)
}

func (h *Handler) fetchChannelYouTube(ctx context.Context, login, handle, youtubeURL string, limit int) (model.YouTubeResponse, error) {
	resp := model.YouTubeResponse{
		Channel:   login,
		UpdatedAt: time.Now().UnixMilli(),
	}
	ref, refSource, err := h.resolveYouTubeRef(ctx, login, handle, youtubeURL)
	if err != nil {
		resp.Sources = append(resp.Sources, sourceWithProvider("youtube", "resolve", "unavailable", err.Error()))
		return resp, nil
	}
	if refSource != "" {
		resp.Sources = append(resp.Sources, sourceWithProvider("youtube", "resolve", "ready", refSource))
	}

	info, sources := h.fetchYouTubeInfo(ctx, ref, limit)
	resp.Sources = append(resp.Sources, sources...)
	if info != nil {
		resp.YouTube = info
	}
	return resp, nil
}

type youtubeRef struct {
	Kind  string // handle, channel_id, username, custom
	Value string
}

func (h *Handler) resolveYouTubeRef(ctx context.Context, login, handle, youtubeURL string) (youtubeRef, string, error) {
	if youtubeURL != "" {
		ref, ok := parseYouTubeURL(youtubeURL)
		if !ok {
			return youtubeRef{}, "", fmt.Errorf("invalid youtubeUrl")
		}
		return ref, "from youtubeUrl query", nil
	}
	if handle != "" {
		handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
		if handle == "" {
			return youtubeRef{}, "", fmt.Errorf("empty handle")
		}
		return youtubeRef{Kind: "handle", Value: handle}, "from handle query", nil
	}
	about, err := h.g.ChannelAbout(ctx, login)
	if err != nil {
		return youtubeRef{}, "", fmt.Errorf("no youtube link in twitch about panels: %v", err)
	}
	if ref, ok := findYouTubeLink(about); ok {
		return ref, "from twitch about social links", nil
	}
	if h.youtubeAPIKey != "" {
		if ref, ok := h.searchYouTubeChannelByLogin(ctx, login); ok {
			return ref, "from youtube search by twitch login", nil
		}
	}
	return youtubeRef{}, "", fmt.Errorf("no youtube link found for channel")
}

func (h *Handler) searchYouTubeChannelByLogin(ctx context.Context, login string) (youtubeRef, bool) {
	searchURL, _ := url.Parse(h.youtubeAPIBaseURL + "/search")
	q := searchURL.Query()
	q.Set("part", "snippet")
	q.Set("type", "channel")
	q.Set("maxResults", "1")
	q.Set("q", login)
	q.Set("key", h.youtubeAPIKey)
	searchURL.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return youtubeRef{}, false
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return youtubeRef{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return youtubeRef{}, false
	}
	var out struct {
		Items []struct {
			ID struct {
				ChannelID string `json:"channelId"`
			} `json:"id"`
			Snippet struct {
				CustomURL string `json:"customUrl"`
				Title     string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&out); err != nil {
		return youtubeRef{}, false
	}
	if len(out.Items) == 0 || out.Items[0].ID.ChannelID == "" {
		return youtubeRef{}, false
	}
	item := out.Items[0]
	if handle := strings.TrimPrefix(strings.TrimSpace(item.Snippet.CustomURL), "@"); handle != "" {
		return youtubeRef{Kind: "handle", Value: handle}, true
	}
	return youtubeRef{Kind: "channel_id", Value: item.ID.ChannelID}, true
}

func findYouTubeLink(about gql.ChannelAbout) (youtubeRef, bool) {
	for _, link := range about.SocialLinks {
		ref, ok := parseYouTubeURL(link.URL)
		if ok {
			return ref, true
		}
	}
	for _, panel := range about.Panels {
		if ref, ok := parseYouTubeURL(panel.LinkURL); ok {
			return ref, true
		}
	}
	return youtubeRef{}, false
}

func parseYouTubeURL(raw string) (youtubeRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return youtubeRef{}, false
	}
	if !strings.Contains(strings.ToLower(raw), "youtube.com") && !strings.Contains(strings.ToLower(raw), "youtu.be") {
		return youtubeRef{}, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return youtubeRef{}, false
	}
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return youtubeRef{}, false
	}
	switch parts[0] {
	case "channel":
		if len(parts) >= 2 && parts[1] != "" {
			return youtubeRef{Kind: "channel_id", Value: parts[1]}, true
		}
	case "c":
		if len(parts) >= 2 && parts[1] != "" {
			return youtubeRef{Kind: "custom", Value: parts[1]}, true
		}
	case "user":
		if len(parts) >= 2 && parts[1] != "" {
			return youtubeRef{Kind: "username", Value: parts[1]}, true
		}
	case "@":
		fallthrough
	default:
		if strings.HasPrefix(parts[0], "@") {
			return youtubeRef{Kind: "handle", Value: strings.TrimPrefix(parts[0], "@")}, true
		}
		if parts[0] != "" && parts[0] != "watch" && parts[0] != "shorts" {
			return youtubeRef{Kind: "handle", Value: parts[0]}, true
		}
	}
	return youtubeRef{}, false
}

func normalizeYouTubeProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "api", "scrape", "off":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func (h *Handler) fetchYouTubeInfo(ctx context.Context, ref youtubeRef, limit int) (*model.YouTubeChannelInfo, []model.SourceStatus) {
	provider := normalizeYouTubeProvider(h.youtubeProvider)
	if provider == "off" {
		return nil, []model.SourceStatus{sourceWithProvider("youtube", "off", "unavailable", "youtube provider disabled")}
	}

	var providersToTry []string
	switch provider {
	case "api":
		providersToTry = []string{"api"}
	case "scrape":
		providersToTry = []string{"scrape"}
	default:
		providersToTry = []string{"api", "scrape"}
	}

	var statuses []model.SourceStatus
	for _, p := range providersToTry {
		var info *model.YouTubeChannelInfo
		var status model.SourceStatus
		tried := false

		switch p {
		case "api":
			if h.youtubeAPIKey != "" {
				info, status = h.fetchYouTubeAPI(ctx, ref, limit)
				tried = true
			}
		case "scrape":
			if h.scraperAPIURL != "" && h.scraperAPIKey != "" {
				info, status = h.fetchYouTubeScrape(ctx, ref, limit)
				tried = true
			}
		}

		if tried {
			statuses = append(statuses, status)
			if status.State == "ready" && info != nil {
				return info, statuses
			}
		}
	}

	if len(statuses) == 0 {
		statuses = append(statuses, sourceWithProvider("youtube", "none", "unavailable", "no youtube providers configured (set YOUTUBE_API_KEY or SCRAPER_API_URL)"))
	}
	return nil, statuses
}

func (h *Handler) fetchYouTubeAPI(ctx context.Context, ref youtubeRef, limit int) (*model.YouTubeChannelInfo, model.SourceStatus) {
	if until, ok := h.youtubeBackoffActive("api"); ok {
		return nil, model.SourceStatus{Source: "youtube", Provider: "api", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	channelID, err := h.resolveYouTubeChannelID(ctx, ref)
	if err != nil {
		status := sourceWithProvider("youtube", "api", "error", err.Error())
		h.markYouTubeBackoff("api", status)
		return nil, status
	}

	channelURL, _ := url.Parse(h.youtubeAPIBaseURL + "/channels")
	q := channelURL.Query()
	q.Set("part", "snippet,statistics")
	q.Set("id", channelID)
	q.Set("key", h.youtubeAPIKey)
	channelURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelURL.String(), nil)
	if err != nil {
		status := sourceWithProvider("youtube", "api", "error", err.Error())
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	resp, err := h.http.Do(req)
	if err != nil {
		status := sourceWithProvider("youtube", "api", "error", err.Error())
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		status := sourceWithProvider("youtube", "api", "error", err.Error())
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	if resp.StatusCode != http.StatusOK {
		state := "error"
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			state = "blocked"
		}
		status := sourceWithProvider("youtube", "api", state, fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		h.markYouTubeBackoff("api", status)
		return nil, status
	}

	var channelOut struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				CustomURL   string `json:"customUrl"`
				Description string `json:"description"`
				Thumbnails  struct {
					Default struct {
						URL string `json:"url"`
					} `json:"default"`
				} `json:"thumbnails"`
			} `json:"snippet"`
			Statistics struct {
				SubscriberCount   string `json:"subscriberCount"`
				HiddenSubscriber  bool   `json:"hiddenSubscriberCount"`
				ViewCount         string `json:"viewCount"`
				VideoCount        string `json:"videoCount"`
			} `json:"statistics"`
		} `json:"items"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &channelOut); err != nil {
		status := sourceWithProvider("youtube", "api", "error", err.Error())
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	if channelOut.Error != nil && channelOut.Error.Message != "" {
		status := sourceWithProvider("youtube", "api", "error", channelOut.Error.Message)
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	if len(channelOut.Items) == 0 {
		status := sourceWithProvider("youtube", "api", "unavailable", "channel not found")
		h.markYouTubeBackoff("api", status)
		return nil, status
	}
	item := channelOut.Items[0]
	info := &model.YouTubeChannelInfo{
		ChannelID:       item.ID,
		Title:           item.Snippet.Title,
		CustomURL:       strings.TrimPrefix(item.Snippet.CustomURL, "@"),
		ProfileImageURL: item.Snippet.Thumbnails.Default.URL,
		LatestVideos:    []model.YouTubeVideo{},
	}
	if info.CustomURL != "" {
		info.Handle = info.CustomURL
	}
	if item.Statistics.HiddenSubscriber {
		info.SubscriberHidden = true
	} else if subs, err := strconv.ParseInt(item.Statistics.SubscriberCount, 10, 64); err == nil {
		info.SubscriberCount = &subs
	}
	if videos, err := strconv.ParseInt(item.Statistics.VideoCount, 10, 64); err == nil {
		info.VideoCount = &videos
	}

	videos, videoStatus := h.fetchYouTubeAPIVideos(ctx, channelID, limit)
	if videoStatus.State == "ready" {
		info.LatestVideos = videos
	}

	status := sourceWithProvider("youtube", "api", "ready", "")
	if len(info.LatestVideos) == 0 && videoStatus.State != "ready" {
		status.Message = "channel loaded; " + videoStatus.Message
	}
	h.markYouTubeBackoff("api", status)
	return info, status
}

func (h *Handler) resolveYouTubeChannelID(ctx context.Context, ref youtubeRef) (string, error) {
	if ref.Kind == "channel_id" {
		return ref.Value, nil
	}
	listURL, _ := url.Parse(h.youtubeAPIBaseURL + "/channels")
	q := listURL.Query()
	q.Set("part", "id")
	q.Set("key", h.youtubeAPIKey)
	switch ref.Kind {
	case "handle":
		q.Set("forHandle", ref.Value)
	case "username":
		q.Set("forUsername", ref.Value)
	default:
		q.Set("forHandle", ref.Value)
	}
	listURL.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("channel lookup status %d", resp.StatusCode)
	}
	var out struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("%s", out.Error.Message)
	}
	if len(out.Items) == 0 {
		return "", fmt.Errorf("channel not found for %s", ref.Value)
	}
	return out.Items[0].ID, nil
}

func (h *Handler) fetchYouTubeAPIVideos(ctx context.Context, channelID string, limit int) ([]model.YouTubeVideo, model.SourceStatus) {
	if limit <= 0 {
		limit = 8
	}
	searchURL, _ := url.Parse(h.youtubeAPIBaseURL + "/search")
	q := searchURL.Query()
	q.Set("part", "snippet")
	q.Set("channelId", channelID)
	q.Set("order", "date")
	q.Set("type", "video")
	q.Set("maxResults", strconv.Itoa(limit))
	q.Set("key", h.youtubeAPIKey)
	searchURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, sourceWithProvider("youtube_videos", "api", "error", err.Error())
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, sourceWithProvider("youtube_videos", "api", "error", err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, sourceWithProvider("youtube_videos", "api", "error", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, sourceWithProvider("youtube_videos", "api", "error", fmt.Sprintf("status %d", resp.StatusCode))
	}
	var out struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				PublishedAt string `json:"publishedAt"`
				Thumbnails  struct {
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, sourceWithProvider("youtube_videos", "api", "error", err.Error())
	}
	videos := make([]model.YouTubeVideo, 0, len(out.Items))
	for _, item := range out.Items {
		vid := item.ID.VideoID
		if vid == "" {
			continue
		}
		videos = append(videos, model.YouTubeVideo{
			ID:           vid,
			Title:        item.Snippet.Title,
			URL:          "https://www.youtube.com/watch?v=" + vid,
			ThumbnailURL: item.Snippet.Thumbnails.Medium.URL,
			PublishedAt:  item.Snippet.PublishedAt,
		})
	}
	return videos, sourceWithProvider("youtube_videos", "api", "ready", "")
}

func (h *Handler) fetchYouTubeScrape(ctx context.Context, ref youtubeRef, limit int) (*model.YouTubeChannelInfo, model.SourceStatus) {
	if until, ok := h.youtubeBackoffActive("scrape"); ok {
		return nil, model.SourceStatus{Source: "youtube", Provider: "scrape", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	pageURL := youTubePageURL(ref)
	htmlBody, err := h.fetchYouTubeScraperPage(ctx, pageURL)
	if err != nil {
		status := sourceWithProvider("youtube", "scrape", youtubeScrapeState(err), err.Error())
		h.markYouTubeBackoff("scrape", status)
		return nil, status
	}
	info := parseYouTubeChannelHTML(htmlBody, ref, limit)
	status := sourceWithProvider("youtube", "scrape", "ready", "")
	if info == nil || (info.Title == "" && len(info.LatestVideos) == 0) {
		status = sourceWithProvider("youtube", "scrape", "unavailable", "scrape did not contain usable channel data")
		h.markYouTubeBackoff("scrape", status)
		return info, status
	}
	h.markYouTubeBackoff("scrape", status)
	return info, status
}

func youTubePageURL(ref youtubeRef) string {
	switch ref.Kind {
	case "channel_id":
		return "https://www.youtube.com/channel/" + ref.Value + "/videos"
	case "handle", "custom":
		handle := ref.Value
		if !strings.HasPrefix(handle, "@") {
			handle = "@" + handle
		}
		return "https://www.youtube.com/" + handle + "/videos"
	case "username":
		return "https://www.youtube.com/user/" + ref.Value + "/videos"
	default:
		return "https://www.youtube.com/@" + ref.Value + "/videos"
	}
}

func (h *Handler) fetchYouTubeScraperPage(ctx context.Context, pageURL string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"url":             pageURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"siteProfile":     "social_public",
		"maxAge":          300000,
		"timeout":         45000,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.scraperAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.scraperAPIKey)
	if h.userAgent != "" {
		req.Header.Set("User-Agent", h.userAgent)
	}
	resp, err := h.scrapeHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxScrapeBody)).Decode(&out); err != nil {
		return "", err
	}
	if !out.Success && out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	htmlBody := out.Data.HTML
	if htmlBody == "" {
		htmlBody = out.Data.RawHTML
	}
	if htmlBody == "" {
		htmlBody = out.Data.Markdown
	}
	if strings.TrimSpace(htmlBody) == "" {
		return "", fmt.Errorf("scrape response missing html")
	}
	return htmlBody, nil
}

func parseYouTubeChannelHTML(body string, ref youtubeRef, limit int) *model.YouTubeChannelInfo {
	info := &model.YouTubeChannelInfo{
		Handle:       ref.Value,
		LatestVideos: []model.YouTubeVideo{},
	}
	if ref.Kind == "channel_id" {
		info.ChannelID = ref.Value
	}
	if match := youtubeSubTextRe.FindStringSubmatch(body); len(match) > 1 {
		if subs := parseYouTubeCountText(match[1]); subs >= 0 {
			info.SubscriberCount = &subs
		}
	}
	seen := map[string]struct{}{}
	videoIDs := youtubeVideoIDRe.FindAllStringSubmatch(body, limit*3)
	titles := collectYouTubeTitles(body, limit*3)
	titleIdx := 0
	for _, match := range videoIDs {
		if len(match) < 2 {
			continue
		}
		id := match[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		video := model.YouTubeVideo{
			ID:  id,
			URL: "https://www.youtube.com/watch?v=" + id,
		}
		for titleIdx < len(titles) {
			title := titles[titleIdx]
			titleIdx++
			if title != "" && !strings.HasPrefix(title, "http") {
				video.Title = title
				break
			}
		}
		info.LatestVideos = append(info.LatestVideos, video)
		if limit > 0 && len(info.LatestVideos) >= limit {
			break
		}
	}
	if len(info.LatestVideos) > 0 {
		count := int64(len(info.LatestVideos))
		info.VideoCount = &count
	}
	return info
}

func collectYouTubeTitles(body string, limit int) []string {
	out := make([]string, 0, limit)
	for _, match := range youtubeTitleSimpleRe.FindAllStringSubmatch(body, limit) {
		if len(match) > 1 && match[1] != "" {
			out = append(out, match[1])
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, match := range youtubeTitleRunsRe.FindAllStringSubmatch(body, limit) {
		if len(match) > 1 && match[1] != "" {
			out = append(out, match[1])
		}
	}
	return out
}

func parseYouTubeCountText(text string) int64 {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimSuffix(text, " subscribers")
	text = strings.TrimSuffix(text, " subscriber")
	text = strings.ReplaceAll(text, ",", "")
	text = strings.TrimSpace(text)
	mult := int64(1)
	if strings.HasSuffix(text, "k") {
		mult = 1000
		text = strings.TrimSuffix(text, "k")
	} else if strings.HasSuffix(text, "m") {
		mult = 1000000
		text = strings.TrimSuffix(text, "m")
	} else if strings.HasSuffix(text, "b") {
		mult = 1000000000
		text = strings.TrimSuffix(text, "b")
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return -1
	}
	return int64(f * float64(mult))
}

func youtubeScrapeState(err error) string {
	if err == nil {
		return "ready"
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "not configured"):
		return "unavailable"
	case strings.Contains(message, "status 401"), strings.Contains(message, "status 403"), strings.Contains(message, "status 429"):
		return "blocked"
	default:
		return "error"
	}
}

func (h *Handler) youtubeBackoffActive(provider string) (time.Time, bool) {
	h.youtubeMu.Lock()
	defer h.youtubeMu.Unlock()
	until, ok := h.youtubeBackoff[provider]
	if !ok || time.Now().After(until) {
		if ok {
			delete(h.youtubeBackoff, provider)
		}
		return time.Time{}, false
	}
	return until, true
}

func (h *Handler) markYouTubeBackoff(provider string, status model.SourceStatus) {
	h.youtubeMu.Lock()
	defer h.youtubeMu.Unlock()
	if status.State == "blocked" || status.State == "error" {
		h.youtubeBackoff[provider] = time.Now().Add(45 * time.Second)
		return
	}
	delete(h.youtubeBackoff, provider)
}
