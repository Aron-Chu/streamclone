package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"streamclone/internal/metadata/cache"
	"streamclone/internal/metadata/gql"
	"streamclone/internal/metadata/model"
	"streamclone/internal/metrics"
	"streamclone/internal/social/reddit"
	"streamclone/internal/upstream"
)

const (
	defaultLimit         = 20
	maxLimit             = 500
	topStreamsPageSize   = 25
	maxQueryLen          = 200
	maxScrapeBody        = 8 * 1024 * 1024
)

var tagRe = regexp.MustCompile(`(?is)<[^>]+>`)
var twitchTrackerRowRe = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
var twitchTrackerCellRe = regexp.MustCompile(`(?is)<td\b([^>]*)>(.*?)</td>`)
var twitchTrackerHrefRe = regexp.MustCompile(`(?is)<a[^>]+href="([^"]+)"`)
var twitchTrackerSpanRe = regexp.MustCompile(`(?is)<span[^>]*>(.*?)</span>`)
var twitchTrackerImgTitleRe = regexp.MustCompile(`(?is)data-original-title="([^"]+)"`)

type GQLClient interface {
	TopStreams(ctx context.Context, limit int, cursor string) (gql.Page[gql.Stream], error)
	Categories(ctx context.Context, limit int, cursor string) (gql.Page[gql.Category], error)
	CategoryStreams(ctx context.Context, categoryID string, limit int, cursor string) (gql.Page[gql.Stream], error)
	Search(ctx context.Context, query string, limit int) (gql.SearchResult, error)
	Channel(ctx context.Context, login string) (gql.Channel, error)
	ChannelAbout(ctx context.Context, login string) (gql.ChannelAbout, error)
}

type HelixClient interface {
	Enabled() bool
	TopLiveStreams(ctx context.Context, limit int, after string) (gql.Page[gql.Stream], error)
	ChannelDetails(ctx context.Context, login string) (model.ChannelDetails, error)
	ChatBadges(ctx context.Context, broadcasterID string) (model.ChatBadgeCatalog, error)
	Clips(ctx context.Context, broadcasterID string, query model.ClipQuery) (model.ClipsResponse, error)
	ArchivedStreamHistory(ctx context.Context, login string, limit int) ([]model.StreamStat, error)
}

type Handler struct {
	c                     *cache.Cache
	g                     GQLClient
	hx                    HelixClient
	follows               FollowStore
	http                  *http.Client
	scrapeHTTP            *http.Client
	twitchTrackerAPIURL   string
	reddit                *reddit.Client
	scraperAPIURL         string
	scraperAPIKey         string
	streamcloneProfile    string
	devTokenImportEnabled bool
	oauthClientID         string
	oauthClientSecret     string
	clipperServiceURL     string
	youtubeProvider       string
	youtubeAPIKey         string
	youtubeAPIBaseURL     string
	userAgent             string
	sf                    singleflight.Group
	scraperReadyMu        sync.RWMutex
	scraperReadyAt        time.Time
	scraperReadyCached    bool
	lsfWarmInFlight       sync.Map
	youtubeMu             sync.Mutex
	youtubeBackoff        map[string]time.Time
}

func New(c *cache.Cache, g GQLClient) *Handler {
	return &Handler{
		c:                    c,
		g:                    g,
		http:                 &http.Client{Timeout: 8 * time.Second},
		scrapeHTTP:           &http.Client{Timeout: 210 * time.Second},
		twitchTrackerAPIURL:  "https://twitchtracker.com/api",
		scraperAPIURL:        "http://scraper:8000/v2/scrape",
		youtubeProvider:      "auto",
		youtubeAPIBaseURL:    defaultYouTubeAPIBase,
		userAgent:            "streamclone/1.0",
		youtubeBackoff:       map[string]time.Time{},
		reddit:               reddit.New(reddit.Options{}),
	}
}

type RedditOptions struct {
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
	ScraperURL     string
	ScraperKey     string
	LSFLowPriority bool
	FirecrawlURL   string // deprecated alias for ScraperURL
	FirecrawlKey   string // deprecated alias for ScraperKey
}

func (h *Handler) WithHelix(hx HelixClient) *Handler {
	h.hx = hx
	return h
}

func (h *Handler) WithExternalSources(twitchTrackerAPIURL, redditBaseURL, userAgent string) *Handler {
	if twitchTrackerAPIURL != "" {
		h.twitchTrackerAPIURL = strings.TrimRight(twitchTrackerAPIURL, "/")
	}
	if userAgent != "" {
		h.userAgent = userAgent
	}
	if redditBaseURL != "" && h.reddit != nil {
		opts := reddit.Options{BaseURL: redditBaseURL, UserAgent: userAgent}
		h.reddit = reddit.New(opts)
	}
	return h
}

func (h *Handler) WithRedditOptions(opts RedditOptions) *Handler {
	scraperURL := opts.ScraperURL
	if scraperURL == "" {
		scraperURL = opts.FirecrawlURL
	}
	scraperKey := opts.ScraperKey
	if scraperKey == "" {
		scraperKey = opts.FirecrawlKey
	}
	if scraperURL != "" {
		h.scraperAPIURL = strings.TrimRight(scraperURL, "/")
	}
	h.scraperAPIKey = scraperKey
	h.reddit = reddit.New(reddit.Options{
		Provider:       opts.Provider,
		BaseURL:        opts.BaseURL,
		OAuthAPIURL:    opts.OAuthAPIURL,
		TokenURL:       opts.TokenURL,
		ClientID:       opts.ClientID,
		ClientSecret:   opts.ClientSecret,
		AccessToken:    opts.AccessToken,
		HTMLFallback:   opts.HTMLFallback,
		ThirdPartyURL:  opts.ThirdPartyURL,
		ThirdPartyKey:  opts.ThirdPartyKey,
		ScraperURL:     scraperURL,
		ScraperKey:     scraperKey,
		LSFLowPriority: opts.LSFLowPriority,
		UserAgent:      h.userAgent,
	})
	return h
}

func (h *Handler) Mount(r *chi.Mux) {
	r.Get("/v1/setup/welcome", h.setupWelcome)
	r.Get("/v1/setup/diagnostics", h.setupDiagnostics)
	r.Get("/v1/ops/network", h.opsNetwork)
	r.Get("/v1/followed", h.followedList)
	r.Get("/v1/streams", h.streams)
	r.Get("/v1/streams/random", h.randomStream)
	r.Get("/v1/categories", h.categories)
	r.Get("/v1/categories/{id}/streams", h.categoryStreams)
	r.Get("/v1/search", h.search)
	r.Route("/v1/channels/{login}", func(r chi.Router) {
		r.Get("/", h.channel)
		r.Get("/details", h.channelDetails)
		r.Get("/badges", h.channelBadges)
		r.Get("/clips", h.channelClips)
		r.Get("/lsf", h.channelLSF)
		r.Get("/youtube", h.channelYouTube)
		r.Get("/insights", h.channelInsights)
		r.Get("/streams/history", h.channelStreamHistory)
		r.Post("/follow", h.followChannel)
		r.Delete("/follow", h.unfollowChannel)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func parseLimit(r *http.Request) int {
	v, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if v <= 0 {
		return defaultLimit
	}
	if v > maxLimit {
		return maxLimit
	}
	return v
}

func parsePool(r *http.Request) int {
	v, _ := strconv.Atoi(r.URL.Query().Get("pool"))
	if v <= 0 {
		return 100
	}
	if v > 20000 {
		return 20000
	}
	return v
}

func parsePeriod(value string) (string, *time.Time, *time.Time) {
	now := time.Now().UTC()
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "24h", "day":
		start := now.Add(-24 * time.Hour)
		return "24h", &start, &now
	case "30d", "month":
		start := now.AddDate(0, 0, -30)
		return "30d", &start, &now
	case "365d", "year":
		start := now.AddDate(-1, 0, 0)
		return "365d", &start, &now
	case "all", "":
		return "all", nil, nil
	default:
		start := now.AddDate(0, 0, -7)
		return "7d", &start, &now
	}
}

func redditTime(period string) string {
	switch period {
	case "24h":
		return "day"
	case "30d":
		return "month"
	case "365d":
		return "year"
	case "all":
		return "all"
	default:
		return "week"
	}
}

func normalizeSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hot", "new":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "top"
	}
}

type sfVal struct{ data []byte }

func (h *Handler) fetchAndCache(ctx context.Context, key string, fetch func() (any, error)) ([]byte, bool, error) {
	if v, ok := h.c.GetFresh(ctx, key); ok {
		metrics.CacheRequests.WithLabelValues("fresh").Inc()
		return v, false, nil
	}
	metrics.CacheRequests.WithLabelValues("miss").Inc()
	v, err, _ := h.sf.Do(key, func() (any, error) {
		op := cacheKeyOperation(key)
		started := time.Now()
		data, fetchErr := fetch()
		if fetchErr != nil {
			metrics.UpstreamRequests.WithLabelValues(op, "error").Inc()
			metrics.UpstreamRequestDuration.WithLabelValues(op, "error").Observe(time.Since(started).Seconds())
			return nil, fetchErr
		}
		metrics.UpstreamRequests.WithLabelValues(op, "ok").Inc()
		metrics.UpstreamRequestDuration.WithLabelValues(op, "ok").Observe(time.Since(started).Seconds())
		b, merr := json.Marshal(data)
		if merr != nil {
			return nil, merr
		}
		_ = h.c.Set(context.WithoutCancel(ctx), key, b)
		return sfVal{data: b}, nil
	})
	if err != nil {
		result, cerr := h.c.Get(ctx, key)
		if cerr == nil {
			if result.Stale {
				metrics.CacheRequests.WithLabelValues("stale").Inc()
			}
			return result.Data, result.Stale, nil
		}
		return nil, false, err
	}
	return v.(sfVal).data, false, nil
}

func cacheKeyOperation(key string) string {
	switch {
	case strings.HasPrefix(key, "meta:streams:top:"):
		return "metadata_top_streams"
	case strings.HasPrefix(key, "meta:streams:randompool:"):
		return "metadata_random_pool"
	case strings.HasPrefix(key, "meta:category:"):
		return "metadata_category_streams"
	case strings.HasPrefix(key, "meta:categories:"):
		return "metadata_categories"
	case strings.HasPrefix(key, "meta:search:"):
		return "metadata_search"
	case strings.HasPrefix(key, "meta:channel:"):
		return "metadata_channel"
	default:
		return "metadata"
	}
}

func respond(w http.ResponseWriter, data []byte, stale bool, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if stale {
		w.Header().Set("X-Cache", "stale")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *Handler) streams(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r)
	cursor := r.URL.Query().Get("cursor")
	key := "meta:streams:top:" + strconv.Itoa(limit) + ":" + cursor
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchTopStreams(r.Context(), limit, cursor)
	})
	respond(w, data, stale, err)
}

func (h *Handler) randomStream(w http.ResponseWriter, r *http.Request) {
	pool := parsePool(r)
	key := "meta:streams:randompool:" + strconv.Itoa(pool)
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchStreamPool(r.Context(), pool)
	})
	if err != nil {
		respond(w, nil, false, err)
		return
	}

	var page gql.Page[gql.Stream]
	if err := json.Unmarshal(data, &page); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if len(page.Items) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no live streams available"})
		return
	}
	idx, err := randomIndex(len(page.Items))
	if err != nil {
		idx = int(time.Now().UnixNano() % int64(len(page.Items)))
	}
	if stale {
		w.Header().Set("X-Cache", "stale")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stream":    page.Items[idx],
		"poolSize":  len(page.Items),
		"stale":     stale,
		"updatedAt": time.Now().UnixMilli(),
	})
}

func (h *Handler) fetchTopStreams(ctx context.Context, limit int, startCursor string) (gql.Page[gql.Stream], error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit <= topStreamsPageSize {
		return h.g.TopStreams(ctx, limit, startCursor)
	}
	if h.hx != nil && h.hx.Enabled() {
		return h.hx.TopLiveStreams(ctx, limit, startCursor)
	}
	return h.fetchTopStreamsGQL(ctx, limit, startCursor)
}

func (h *Handler) fetchTopStreamsGQL(ctx context.Context, limit int, startCursor string) (gql.Page[gql.Stream], error) {
	pageSize := topStreamsPageSize
	out := gql.Page[gql.Stream]{Items: make([]gql.Stream, 0, min(limit, pageSize))}
	cursor := startCursor
	seen := map[string]struct{}{}
	for len(out.Items) < limit {
		page, err := h.g.TopStreams(ctx, pageSize, cursor)
		if err != nil {
			if len(out.Items) > 0 {
				if topStreamsGQLFailed(err) {
					return h.backfillTopStreamsFromHelix(ctx, out, limit, seen)
				}
				return out, nil
			}
			return gql.Page[gql.Stream]{}, err
		}
		if len(page.Items) == 0 {
			break
		}
		for _, stream := range page.Items {
			key := stream.Login
			if key == "" {
				key = stream.ID
			}
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out.Items = append(out.Items, stream)
			if len(out.Items) >= limit {
				break
			}
		}
		if page.Cursor == "" || page.Cursor == cursor {
			if len(out.Items) < limit {
				return h.backfillTopStreamsFromHelix(ctx, out, limit, seen)
			}
			break
		}
		cursor = page.Cursor
		out.Cursor = cursor
	}
	return out, nil
}

func (h *Handler) backfillTopStreamsFromHelix(ctx context.Context, partial gql.Page[gql.Stream], limit int, seen map[string]struct{}) (gql.Page[gql.Stream], error) {
	if h.hx == nil || !h.hx.Enabled() || len(partial.Items) >= limit {
		return partial, nil
	}
	helixPage, err := h.hx.TopLiveStreams(ctx, limit, "")
	if err != nil {
		return partial, nil
	}
	for _, stream := range helixPage.Items {
		key := stream.Login
		if key == "" {
			key = stream.ID
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		partial.Items = append(partial.Items, stream)
		if len(partial.Items) >= limit {
			break
		}
	}
	if partial.Cursor == "" {
		partial.Cursor = helixPage.Cursor
	}
	return partial, nil
}

func topStreamsGQLFailed(err error) bool {
	return errors.Is(err, upstream.ErrUpstreamSchema)
}

func (h *Handler) fetchStreamPool(ctx context.Context, pool int) (gql.Page[gql.Stream], error) {
	if pool <= 0 {
		pool = 100
	}
	return h.fetchTopStreams(ctx, pool, "")
}

func randomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("invalid random bound %d", n)
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r)
	cursor := r.URL.Query().Get("cursor")
	key := "meta:categories:" + strconv.Itoa(limit) + ":" + cursor
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.g.Categories(r.Context(), limit, cursor)
	})
	respond(w, data, stale, err)
}

func (h *Handler) categoryStreams(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit := parseLimit(r)
	cursor := r.URL.Query().Get("cursor")
	key := "meta:category:" + id + ":streams:" + strconv.Itoa(limit) + ":" + cursor
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.g.CategoryStreams(r.Context(), id, limit, cursor)
	})
	respond(w, data, stale, err)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	if len(q) > maxQueryLen {
		q = q[:maxQueryLen]
	}
	limit := parseLimit(r)
	key := "meta:search:" + q + ":" + strconv.Itoa(limit)
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.g.Search(r.Context(), q, limit)
	})
	respond(w, data, stale, err)
}

func (h *Handler) channel(w http.ResponseWriter, r *http.Request) {
	login := chi.URLParam(r, "login")
	key := "meta:channelid:" + login
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.g.Channel(r.Context(), login)
	})
	respond(w, data, stale, err)
}

func (h *Handler) channelDetails(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	key := "meta:channeldetails:" + login
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchChannelDetails(r.Context(), login)
	})
	respond(w, data, stale, err)
}

func (h *Handler) channelBadges(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	key := "meta:channelbadges:" + login
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchChannelBadges(r.Context(), login)
	})
	respond(w, data, stale, err)
}

func (h *Handler) channelClips(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	limit := parseLimit(r)
	period, startedAt, endedAt := parsePeriod(r.URL.Query().Get("period"))
	if startStr := r.URL.Query().Get("startedAt"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			startedAt = &t
			period = "custom_" + startStr
		}
	}
	if endStr := r.URL.Query().Get("endedAt"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endedAt = &t
			period = period + "_" + endStr
		}
	}
	cursor := r.URL.Query().Get("cursor")
	key := "meta:channelclips:" + login + ":" + period + ":" + strconv.Itoa(limit) + ":" + cursor
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchChannelClips(r.Context(), login, model.ClipQuery{
			Limit:     limit,
			Period:    period,
			Cursor:    cursor,
			StartedAt: startedAt,
			EndedAt:   endedAt,
		})
	})
	respond(w, data, stale, err)
}

func (h *Handler) channelLSF(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	period, _, _ := parsePeriod(r.URL.Query().Get("period"))
	sort := normalizeSort(r.URL.Query().Get("sort"))
	h.writeRedditLSFResponse(w, r.Context(), login, period, sort, parseLSFRefresh(r))
}

func (h *Handler) channelInsights(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	period, _, _ := parsePeriod(r.URL.Query().Get("period"))
	clipPeriod, _, _ := parsePeriod(r.URL.Query().Get("clipPeriod"))
	lsfPeriod, _, _ := parsePeriod(r.URL.Query().Get("lsfPeriod"))
	if r.URL.Query().Get("clipPeriod") == "" {
		clipPeriod = period
	}
	if r.URL.Query().Get("lsfPeriod") == "" {
		lsfPeriod = period
	}
	sort := normalizeSort(r.URL.Query().Get("lsfSort"))
	lsfRefresh := parseLSFRefresh(r)
	key := "meta:channelinsights:" + login + ":" + period + ":" + clipPeriod + ":" + lsfPeriod + ":" + sort
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		return h.fetchChannelInsights(r.Context(), login, period, clipPeriod, lsfPeriod, sort)
	})
	if err == nil {
		data = h.patchInsightsLSF(r.Context(), data, login, lsfPeriod, sort, lsfRefresh)
	}
	respond(w, data, stale, err)
}

func parseLSFRefresh(r *http.Request) bool {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lsfRefresh")))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("refresh")))
	}
	return v == "1" || v == "true" || v == "yes"
}

func (h *Handler) channelStreamHistory(w http.ResponseWriter, r *http.Request) {
	login := normalizeLogin(chi.URLParam(r, "login"))
	period, _, _ := parsePeriod(r.URL.Query().Get("period"))
	if period == "" {
		period = "30d"
	}
	key := "meta:channelstreamhistory:" + login + ":" + period
	data, stale, err := h.fetchAndCache(r.Context(), key, func() (any, error) {
		history, status := h.fetchHelixStreamHistory(r.Context(), login, period, "helix archive unavailable")
		if history == nil {
			history = []model.StreamStat{}
		}
		return map[string]any{
			"channel":   login,
			"period":    period,
			"items":     history,
			"sources":   []model.SourceStatus{status},
			"updatedAt": time.Now().UnixMilli(),
		}, nil
	})
	respond(w, data, stale, err)
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func source(source, state, message string) model.SourceStatus {
	return model.SourceStatus{Source: source, State: state, Message: message}
}

func sourceWithProvider(sourceName, provider, state, message string) model.SourceStatus {
	return model.SourceStatus{Source: sourceName, Provider: provider, State: state, Message: message}
}

func fromGQLChannel(ch gql.Channel, sources []model.SourceStatus) model.ChannelDetails {
	return model.ChannelDetails{
		ID:           ch.ID,
		Login:        ch.Login,
		DisplayName:  ch.DisplayName,
		Description:  ch.Description,
		ProfileImage: ch.ProfileImage,
		CreatedAt:    ch.CreatedAt,
		IsLive:       ch.IsLive,
		StreamID:     ch.StreamID,
		StreamTitle:  ch.StreamTitle,
		Category:     ch.Category,
		Viewers:      ch.Viewers,
		ThumbnailURL: ch.ThumbnailURL,
		StartedAt:    ch.StartedAt,
		UpdatedAt:    time.Now().UnixMilli(),
		Sources:      sources,
	}
}

func attachGQLAbout(details *model.ChannelDetails, about gql.ChannelAbout) {
	details.AboutPanels = make([]model.AboutPanel, 0, len(about.Panels))
	for _, panel := range about.Panels {
		details.AboutPanels = append(details.AboutPanels, model.AboutPanel{
			ID:          panel.ID,
			Type:        panel.Type,
			Title:       panel.Title,
			Description: panel.Description,
			ImageURL:    panel.ImageURL,
			LinkURL:     panel.LinkURL,
		})
	}
	details.SocialLinks = make([]model.SocialLink, 0, len(about.SocialLinks))
	for _, link := range about.SocialLinks {
		details.SocialLinks = append(details.SocialLinks, model.SocialLink{ID: link.ID, Title: link.Title, URL: link.URL})
	}
}

func (h *Handler) addChannelAboutPanels(ctx context.Context, login string, details model.ChannelDetails) model.ChannelDetails {
	about, err := h.g.ChannelAbout(ctx, login)
	if err != nil {
		details.Sources = append(details.Sources, source("twitch_gql_about_panels", "unavailable", err.Error()))
		return details
	}
	attachGQLAbout(&details, about)
	state := "ready"
	msg := ""
	if len(details.AboutPanels) == 0 && len(details.SocialLinks) == 0 {
		state = "unavailable"
		msg = "about query returned no panels"
	}
	details.Sources = append(details.Sources, source("twitch_gql_about_panels", state, msg))
	return details
}

func (h *Handler) fetchChannelDetails(ctx context.Context, login string) (model.ChannelDetails, error) {
	var sources []model.SourceStatus
	if h.hx != nil && h.hx.Enabled() {
		details, err := h.hx.ChannelDetails(ctx, login)
		if err == nil {
			details.Sources = append(details.Sources, source("twitch_helix", "ready", ""))
			return h.addChannelAboutPanels(ctx, login, details), nil
		}
		sources = append(sources, source("twitch_helix", "error", err.Error()))
	} else {
		sources = append(sources, source("twitch_helix", "unavailable", "missing server credentials"))
	}

	ch, err := h.g.Channel(ctx, login)
	if err != nil {
		sources = append(sources, source("twitch_gql", "error", err.Error()))
		return model.ChannelDetails{}, err
	}
	sources = append(sources, source("twitch_gql", "fallback", ""))
	return h.addChannelAboutPanels(ctx, login, fromGQLChannel(ch, sources)), nil
}

func (h *Handler) fetchChannelBadges(ctx context.Context, login string) (model.ChatBadgeCatalog, error) {
	resp := model.ChatBadgeCatalog{
		Channel:   login,
		Badges:    map[string]model.ChatBadge{},
		UpdatedAt: time.Now().UnixMilli(),
	}
	if h.hx == nil || !h.hx.Enabled() {
		resp.Sources = append(resp.Sources, source("twitch_helix_badges", "unavailable", "helix credentials not configured"))
		return resp, nil
	}
	details, err := h.hx.ChannelDetails(ctx, login)
	if err != nil {
		resp.Sources = append(resp.Sources, source("twitch_helix_badges", "error", err.Error()))
		return resp, nil
	}
	catalog, err := h.hx.ChatBadges(ctx, details.ID)
	if err != nil {
		resp.Sources = append(resp.Sources, source("twitch_helix_badges", "error", err.Error()))
		return resp, nil
	}
	resp.Badges = catalog.Badges
	resp.Sources = append(resp.Sources, source("twitch_helix_badges", "ready", ""))
	resp.UpdatedAt = catalog.UpdatedAt
	return resp, nil
}

func (h *Handler) fetchChannelClips(ctx context.Context, login string, query model.ClipQuery) (model.ClipsResponse, error) {
	if query.Period == "" {
		query.Period = "all"
	}
	resp := model.ClipsResponse{Period: query.Period, UpdatedAt: time.Now().UnixMilli()}
	if h.hx == nil || !h.hx.Enabled() {
		resp.Sources = append(resp.Sources, source("twitch_helix_clips", "unavailable", "missing server credentials"))
		return resp, nil
	}

	details, err := h.hx.ChannelDetails(ctx, login)
	if err != nil {
		resp.Sources = append(resp.Sources, source("twitch_helix_clips", "error", err.Error()))
		return resp, nil
	}
	clips, err := h.hx.Clips(ctx, details.ID, query)
	if err != nil {
		resp.Sources = append(resp.Sources, source("twitch_helix_clips", "error", err.Error()))
		return resp, nil
	}
	resp.Items = clips.Items
	resp.Cursor = clips.Cursor
	resp.Sources = append(resp.Sources, source("twitch_helix_clips", "ready", ""))
	return resp, nil
}

func (h *Handler) fetchChannelInsights(ctx context.Context, login, period, clipPeriod, lsfPeriod, lsfSort string) (model.InsightsResponse, error) {
	_, clipStart, clipEnd := parsePeriod(clipPeriod)
	resp := model.InsightsResponse{
		Channel:    login,
		Period:     period,
		ClipPeriod: clipPeriod,
		LSFPeriod:  lsfPeriod,
		UpdatedAt:  time.Now().UnixMilli(),
	}

	stats, statsSource := h.fetchTwitchTrackerSummary(ctx, login)
	resp.Stats = stats
	history, historySource := h.fetchTwitchTrackerStreamHistory(ctx, login, period)
	resp.StreamHistory = buildStreamHistory(history, period)
	resp.StatsTimeline = buildStatsTimeline(stats, resp.StreamHistory)
	if len(resp.StreamHistory) == 0 && historySource.State == "ready" {
		historySource = sourceWithProvider("stream_history", historySource.Provider, "unavailable", "no TwitchTracker streams matched the selected period")
	}
	resp.Sources = append(resp.Sources, statsSource, historySource)

	posts, redditSources := h.fetchRedditLSFCached(ctx, login, lsfPeriod, lsfSort, false)
	if posts == nil {
		posts = []model.RedditPost{}
	}
	resp.LSF = posts
	resp.Sources = append(resp.Sources, redditSources...)

	clips, _ := h.fetchChannelClips(ctx, login, model.ClipQuery{Limit: 8, Period: clipPeriod, StartedAt: clipStart, EndedAt: clipEnd})
	resp.Clips = clips.Items
	resp.Sources = append(resp.Sources, clips.Sources...)
	resp.StatsDerived = buildStatsDerived(stats, resp.Clips, resp.LSF, resp.StreamHistory)

	return resp, nil
}

func (h *Handler) fetchTwitchTrackerSummary(ctx context.Context, login string) (*model.TwitchTrackerSummary, model.SourceStatus) {
	if h.twitchTrackerAPIURL == "" {
		return nil, source("twitchtracker", "unavailable", "api url not configured")
	}

	endpoint := fmt.Sprintf("%s/channels/summary/%s", strings.TrimRight(h.twitchTrackerAPIURL, "/"), url.PathEscape(login))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, source("twitchtracker", "error", err.Error())
	}
	if h.userAgent != "" {
		req.Header.Set("User-Agent", h.userAgent)
	}

	resp, err := h.http.Do(req)
	if err != nil {
		return nil, source("twitchtracker", "error", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, source("twitchtracker", "error", fmt.Sprintf("status %d", resp.StatusCode))
	}

	var summary model.TwitchTrackerSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, source("twitchtracker", "error", err.Error())
	}
	return &summary, source("twitchtracker", "ready", "")
}

func (h *Handler) fetchTwitchTrackerStreamHistory(ctx context.Context, login, period string) ([]model.StreamStat, model.SourceStatus) {
	// Helix VOD archives are fast and reliable; TwitchTracker HTML is often Cloudflare-blocked.
	if helixHistory, helixStatus, ok := h.tryHelixStreamHistory(ctx, login, period); ok {
		if enriched, enrichProvider, enrichMsg := h.enrichStreamHistoryFromTwitchTracker(ctx, login, helixHistory); enrichMsg != "" {
			msg := "parsed Twitch archive VODs via Helix"
			if enrichProvider != "" {
				msg += "; enriched avg/peak from TwitchTracker (" + enrichMsg + ")"
			}
			return enriched, sourceWithProvider("stream_history", "helix", "ready", msg)
		}
		return helixHistory, helixStatus
	}

	baseURL := h.twitchTrackerWebBaseURL()
	if baseURL == "" {
		return h.fetchHelixStreamHistory(ctx, login, period, "twitchtracker base url not configured")
	}
	pageURL := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(login) + "/streams"
	body, provider, err := h.fetchTwitchTrackerPage(ctx, pageURL)
	if err != nil {
		return nil, sourceWithProvider("stream_history", provider, twitchTrackerStreamHistoryState(err), err.Error())
	}
	history := parseTwitchTrackerStreamsTable(body)
	if len(history) == 0 {
		return nil, sourceWithProvider("stream_history", provider, "unavailable", "twitchtracker streams page did not contain parseable rows")
	}
	filtered := buildStreamHistory(history, period)
	message := "parsed TwitchTracker streams table"
	if provider == "scraper" {
		message += " via browser scraper"
	}
	return filtered, sourceWithProvider("stream_history", provider, "ready", message)
}

func (h *Handler) enrichStreamHistoryFromTwitchTracker(ctx context.Context, login string, helixHistory []model.StreamStat) ([]model.StreamStat, string, string) {
	baseURL := h.twitchTrackerWebBaseURL()
	if baseURL == "" || len(helixHistory) == 0 {
		return helixHistory, "", ""
	}
	enrichCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	pageURL := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(login) + "/streams"
	body, provider, err := h.fetchTwitchTrackerPage(enrichCtx, pageURL)
	if err != nil {
		return helixHistory, "", ""
	}
	trackerRows := parseTwitchTrackerStreamsTable(body)
	if len(trackerRows) == 0 {
		return helixHistory, "", ""
	}
	byID := make(map[string]model.StreamStat, len(trackerRows))
	for _, row := range trackerRows {
		byID[row.ID] = row
	}
	enriched := make([]model.StreamStat, len(helixHistory))
	copy(enriched, helixHistory)
	merged := 0
	for i, row := range enriched {
		tracker, ok := byID[row.ID]
		if !ok {
			continue
		}
		if tracker.AvgViewers > 0 {
			enriched[i].AvgViewers = tracker.AvgViewers
		}
		if tracker.PeakViewers > 0 {
			enriched[i].PeakViewers = tracker.PeakViewers
		}
		if tracker.HoursWatched > 0 {
			enriched[i].HoursWatched = tracker.HoursWatched
		}
		if tracker.DurationMinutes > 0 && enriched[i].DurationMinutes == 0 {
			enriched[i].DurationMinutes = tracker.DurationMinutes
		}
		merged++
	}
	if merged == 0 {
		return helixHistory, "", ""
	}
	return enriched, provider, fmt.Sprintf("%d streams matched", merged)
}

func (h *Handler) tryHelixStreamHistory(ctx context.Context, login, period string) ([]model.StreamStat, model.SourceStatus, bool) {
	if h.hx == nil || !h.hx.Enabled() {
		return nil, model.SourceStatus{}, false
	}
	limit := streamHistoryLimit(period)
	if limit <= 0 {
		limit = 80
	}
	raw, err := h.hx.ArchivedStreamHistory(ctx, login, limit*2)
	if err != nil || len(raw) == 0 {
		return nil, model.SourceStatus{}, false
	}
	history := buildStreamHistory(raw, period)
	if len(history) == 0 {
		return nil, model.SourceStatus{}, false
	}
	return history, sourceWithProvider("stream_history", "helix", "ready", "parsed Twitch archive VODs via Helix (avg/peak viewers require TwitchTracker sync)"), true
}

func (h *Handler) fetchHelixStreamHistory(ctx context.Context, login, period, reason string) ([]model.StreamStat, model.SourceStatus) {
	if history, status, ok := h.tryHelixStreamHistory(ctx, login, period); ok {
		return history, status
	}
	return nil, sourceWithProvider("stream_history", "helix", "unavailable", reason)
}

func (h *Handler) twitchTrackerWebBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(h.twitchTrackerAPIURL), "/")
	base = strings.TrimSuffix(base, "/api")
	if base != "" && !strings.Contains(strings.ToLower(base), "twitchtracker.com") {
		// Scraper/mock API hosts only implement summary + /v2/scrape â€” HTML pages live on TwitchTracker.
		return "https://twitchtracker.com"
	}
	return base
}

func (h *Handler) fetchTwitchTrackerPage(ctx context.Context, rawURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "html", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if h.userAgent != "" {
		req.Header.Set("User-Agent", h.userAgent)
	}
	resp, err := h.http.Do(req)
	if err == nil {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			htmlBody := string(body)
			if !looksLikeCloudflareChallenge(htmlBody) {
				return htmlBody, "html", nil
			}
			err = fmt.Errorf("cloudflare challenge page")
		} else if readErr != nil {
			err = readErr
		} else {
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
	}
	if h.scraperAPIKey == "" {
		if err == nil {
			err = fmt.Errorf("scraper api key not configured")
		} else if strings.Contains(strings.ToLower(err.Error()), "cloudflare") {
			err = fmt.Errorf("%s; scraper api key not configured", err.Error())
		}
		return "", "html", err
	}
	htmlBody, scraperErr := h.fetchTwitchTrackerPageScraper(ctx, rawURL)
	if scraperErr != nil {
		return "", "scraper", scraperErr
	}
	return htmlBody, "scraper", nil
}

func (h *Handler) fetchTwitchTrackerPageScraper(ctx context.Context, rawURL string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"url":             rawURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"maxAge":          300000,
		"timeout":         30000,
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
		return "", fmt.Errorf("scraper response missing html")
	}
	return htmlBody, nil
}

func twitchTrackerStreamHistoryState(err error) string {
	if err == nil {
		return "ready"
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "scraper api key not configured"):
		return "unavailable"
	case strings.Contains(message, "cloudflare"), strings.Contains(message, "status 401"), strings.Contains(message, "status 403"), strings.Contains(message, "status 429"):
		return "blocked"
	default:
		return "error"
	}
}

func looksLikeCloudflareChallenge(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "just a moment") || strings.Contains(lower, "performing security verification") || strings.Contains(lower, "cf_chl_opt")
}

func redditLSFCacheKey(login, period, sort string) string {
	return "meta:channellsf:" + login + ":" + period + ":" + sort
}

type redditLSFCacheValue struct {
	Items     []model.RedditPost   `json:"items"`
	Sources   []model.SourceStatus `json:"sources"`
	Period    string               `json:"period"`
	Sort      string               `json:"sort"`
	UpdatedAt int64                `json:"updatedAt"`
}

func (h *Handler) kickRedditLSFWarm(login, period, sort string) {
	key := redditLSFCacheKey(login, period, sort)
	if _, loaded := h.lsfWarmInFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	go func() {
		defer h.lsfWarmInFlight.Delete(key)
		ctx := context.Background()
		_, _, _ = h.fetchAndCache(ctx, key, func() (any, error) {
			posts, sources := h.fetchRedditLSF(ctx, login, period, sort)
			if posts == nil {
				posts = []model.RedditPost{}
			}
			return redditLSFCacheValue{
				Items:     posts,
				Sources:   sources,
				Period:    period,
				Sort:      sort,
				UpdatedAt: time.Now().UnixMilli(),
			}, nil
		})
	}()
}

func redditLSFWarmingStatus() model.SourceStatus {
	return reddit.WarmingStatus()
}

func redditLSFPendingStatus() model.SourceStatus {
	return reddit.PendingStatus()
}

func (h *Handler) fetchRedditLSFCached(ctx context.Context, login, period, sort string, refresh bool) ([]model.RedditPost, []model.SourceStatus) {
	key := redditLSFCacheKey(login, period, sort)
	if v, ok := h.c.GetFresh(ctx, key); ok {
		var cached redditLSFCacheValue
		if json.Unmarshal(v, &cached) == nil {
			return cached.Items, cached.Sources
		}
	}
	var staleCached *redditLSFCacheValue
	if result, err := h.c.Get(ctx, key); err == nil {
		var cached redditLSFCacheValue
		if json.Unmarshal(result.Data, &cached) == nil {
			if result.Stale {
				staleCached = &cached
				if !refresh {
					return cached.Items, cached.Sources
				}
			} else {
				return cached.Items, cached.Sources
			}
		}
	}
	if _, warming := h.lsfWarmInFlight.Load(key); warming {
		if staleCached != nil {
			return staleCached.Items, []model.SourceStatus{redditLSFWarmingStatus()}
		}
		return []model.RedditPost{}, []model.SourceStatus{redditLSFWarmingStatus()}
	}
	if staleCached != nil && refresh {
		h.kickRedditLSFWarm(login, period, sort)
		return staleCached.Items, []model.SourceStatus{redditLSFWarmingStatus()}
	}
	h.kickRedditLSFWarm(login, period, sort)
	return []model.RedditPost{}, []model.SourceStatus{redditLSFWarmingStatus()}
}

func replaceRedditLSFSources(sources, reddit []model.SourceStatus) []model.SourceStatus {
	out := make([]model.SourceStatus, 0, len(sources)+len(reddit))
	for _, s := range sources {
		if !strings.HasPrefix(s.Source, "reddit_lsf") {
			out = append(out, s)
		}
	}
	return append(out, reddit...)
}

func (h *Handler) patchInsightsLSF(ctx context.Context, raw []byte, login, lsfPeriod, sort string, refresh bool) []byte {
	var resp model.InsightsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return raw
	}
	posts, sources := h.fetchRedditLSFCached(ctx, login, lsfPeriod, sort, refresh)
	if posts == nil {
		posts = []model.RedditPost{}
	}
	resp.LSF = posts
	resp.Sources = replaceRedditLSFSources(resp.Sources, sources)
	updated, err := json.Marshal(resp)
	if err != nil {
		return raw
	}
	return updated
}

func (h *Handler) writeRedditLSFResponse(w http.ResponseWriter, ctx context.Context, login, period, sort string, refresh bool) {
	key := redditLSFCacheKey(login, period, sort)
	posts, sources := h.fetchRedditLSFCached(ctx, login, period, sort, refresh)
	if posts == nil {
		posts = []model.RedditPost{}
	}
	stale := false
	if result, err := h.c.Get(ctx, key); err == nil {
		stale = result.Stale
	}
	payload, err := json.Marshal(map[string]any{
		"items":     posts,
		"sources":   sources,
		"period":    period,
		"sort":      sort,
		"updatedAt": time.Now().UnixMilli(),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respond(w, payload, stale, nil)
}

func (h *Handler) fetchRedditLSF(ctx context.Context, login, period, sort string) ([]model.RedditPost, []model.SourceStatus) {
	if h.reddit == nil {
		return nil, []model.SourceStatus{sourceWithProvider("reddit_lsf", "none", "unavailable", "reddit client not configured")}
	}
	return h.reddit.FetchLSF(ctx, login, period, sort)
}

func buildStatsTimeline(stats *model.TwitchTrackerSummary, history []model.StreamStat) []model.StatsTimelinePoint {
	if len(history) > 0 {
		out := make([]model.StatsTimelinePoint, 0, len(history))
		for i := len(history) - 1; i >= 0; i-- {
			row := history[i]
			label := "Stream"
			if row.StartedAt != "" {
				if startedAt, err := time.Parse(time.RFC3339, row.StartedAt); err == nil {
					label = startedAt.Format("Jan 2")
				}
			}
			out = append(out, model.StatsTimelinePoint{
				Label:        label,
				AvgViewers:   row.AvgViewers,
				PeakViewers:  row.PeakViewers,
				HoursWatched: row.HoursWatched,
			})
		}
		return out
	}
	if stats == nil || (stats.AvgViewers == 0 && stats.MaxViewers == 0) {
		return nil
	}
	return []model.StatsTimelinePoint{
		{Label: "Average", AvgViewers: stats.AvgViewers},
		{Label: "Peak", AvgViewers: stats.MaxViewers, PeakViewers: stats.MaxViewers},
	}
}

func parseTwitchTrackerStreamsTable(body string) []model.StreamStat {
	tableIndex := strings.Index(strings.ToLower(body), `id="streams"`)
	if tableIndex >= 0 {
		body = body[tableIndex:]
	}
	rows := twitchTrackerRowRe.FindAllStringSubmatch(body, -1)
	out := make([]model.StreamStat, 0, len(rows))
	for _, row := range rows {
		cells := twitchTrackerCellRe.FindAllStringSubmatch(row[1], -1)
		if len(cells) < 7 {
			continue
		}
		id := twitchTrackerStreamID(cells[0][2])
		if id == "" {
			continue
		}
		durationMinutes := twitchTrackerCellOrder(cells[1][1])
		avgViewers := twitchTrackerCellValue(cells[2][2])
		peakViewers := twitchTrackerCellValue(cells[3][2])
		title := compactHTMLText(cells[6][2])
		games := twitchTrackerCellGames(cells[minInt(7, len(cells)-1)][2])
		startedAt, endedAt := twitchTrackerTimes(cells[0][1], durationMinutes)
		category := ""
		if len(games) > 0 {
			category = games[0]
		}
		if title == "" {
			title = "Stream " + strings.TrimSpace(twitchTrackerCellDisplayTime(cells[0][2]))
		}
		out = append(out, model.StreamStat{
			ID:              id,
			Title:           title,
			Category:        category,
			StartedAt:       startedAt,
			EndedAt:         endedAt,
			DurationMinutes: durationMinutes,
			AvgViewers:      avgViewers,
			PeakViewers:     peakViewers,
			HoursWatched:    twitchTrackerHoursWatched(avgViewers, durationMinutes),
		})
	}
	return out
}

func buildStreamHistory(history []model.StreamStat, period string) []model.StreamStat {
	if len(history) == 0 {
		return nil
	}
	_, startedAfter, _ := parsePeriod(period)
	limit := streamHistoryLimit(period)
	out := make([]model.StreamStat, 0, len(history))
	for _, row := range history {
		if startedAfter != nil && row.StartedAt != "" {
			startedAt, err := time.Parse(time.RFC3339, row.StartedAt)
			if err == nil && startedAt.Before(*startedAfter) {
				continue
			}
		}
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func streamHistoryLimit(period string) int {
	switch period {
	case "24h":
		return 8
	case "30d":
		return 40
	case "365d", "all":
		return 80
	default:
		return 16
	}
}

func twitchTrackerStreamID(cellHTML string) string {
	match := twitchTrackerHrefRe.FindStringSubmatch(cellHTML)
	if len(match) < 2 {
		return ""
	}
	u, err := url.Parse(html.UnescapeString(match[1]))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func twitchTrackerCellOrder(attrs string) int {
	value := htmlAttr(attrs, "data-order")
	count, _ := strconv.Atoi(strings.TrimSpace(value))
	return count
}

func twitchTrackerCellValue(cellHTML string) int {
	match := twitchTrackerSpanRe.FindStringSubmatch(cellHTML)
	value := cellHTML
	if len(match) >= 2 {
		value = match[1]
	}
	text := compactHTMLText(value)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, text)
	count, _ := strconv.Atoi(digits)
	return count
}

func twitchTrackerCellDisplayTime(cellHTML string) string {
	match := twitchTrackerSpanRe.FindStringSubmatch(cellHTML)
	if len(match) < 2 {
		return compactHTMLText(cellHTML)
	}
	return compactHTMLText(match[1])
}

func twitchTrackerCellGames(cellHTML string) []string {
	matches := twitchTrackerImgTitleRe.FindAllStringSubmatch(cellHTML, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		name := strings.TrimSpace(html.UnescapeString(match[1]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func twitchTrackerTimes(attrs string, durationMinutes int) (string, string) {
	value := htmlAttr(attrs, "data-order")
	if value == "" {
		return "", ""
	}
	startedAt, err := time.ParseInLocation("2006-01-02 15:04", value, time.UTC)
	if err != nil {
		return "", ""
	}
	endedAt := startedAt.Add(time.Duration(durationMinutes) * time.Minute)
	return startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339)
}

func twitchTrackerHoursWatched(avgViewers, durationMinutes int) int {
	if avgViewers <= 0 || durationMinutes <= 0 {
		return 0
	}
	return (avgViewers*durationMinutes + 30) / 60
}

func compactHTMLText(value string) string {
	text := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(value, " ")))
	return strings.Join(strings.Fields(text), " ")
}

func htmlAttr(attrs, name string) string {
	needle := strings.ToLower(name) + `="`
	lower := strings.ToLower(attrs)
	start := strings.Index(lower, needle)
	if start < 0 {
		return ""
	}
	start += len(needle)
	end := strings.Index(attrs[start:], `"`)
	if end < 0 {
		return ""
	}
	return html.UnescapeString(attrs[start : start+end])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildStatsDerived(stats *model.TwitchTrackerSummary, clips []model.ClipCard, posts []model.RedditPost, history []model.StreamStat) *model.StatsDerived {
	if stats == nil && len(clips) == 0 && len(posts) == 0 && len(history) == 0 {
		return nil
	}
	out := &model.StatsDerived{
		ClipsLoaded:          len(clips),
		LSFPostsLoaded:       len(posts),
		HasRealStreamHistory: len(history) > 0,
	}
	if stats == nil {
		return out
	}
	out.HoursStreamed = round2(float64(stats.MinutesStreamed) / 60)
	if out.HoursStreamed > 0 {
		out.ViewerHoursPerStreamHour = round2(float64(stats.HoursWatched) / out.HoursStreamed)
		out.FollowersPerStreamHour = round2(float64(stats.Followers) / out.HoursStreamed)
	}
	if stats.AvgViewers > 0 {
		out.PeakToAverageRatio = round2(float64(stats.MaxViewers) / float64(stats.AvgViewers))
	}
	return out
}

func round2(value float64) float64 {
	scaled := value * 100
	if scaled >= 0 {
		return float64(int(scaled+0.5)) / 100
	}
	return float64(int(scaled-0.5)) / 100
}
