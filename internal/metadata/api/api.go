package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
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
)

const (
	defaultLimit  = 20
	maxLimit      = 100
	maxQueryLen   = 200
	maxScrapeBody = 8 * 1024 * 1024
)

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
	redditBaseURL         string
	redditOAuthAPIURL     string
	redditTokenURL        string
	redditProvider        string
	redditClientID        string
	redditClientSecret    string
	redditAccessToken     string
	redditHTMLFallback    bool
	redditThirdPartyURL   string
	redditThirdPartyKey   string
	redditLSFLowPriority  bool
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
	redditMu              sync.Mutex
	redditToken           redditToken
	redditBackoff         map[string]time.Time
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
		redditBaseURL:        "https://www.reddit.com",
		redditOAuthAPIURL:    "https://oauth.reddit.com",
		redditTokenURL:       "https://www.reddit.com/api/v1/access_token",
		redditProvider:       "auto",
		scraperAPIURL:        "http://scraper:8000/v2/scrape",
		youtubeProvider:      "auto",
		youtubeAPIBaseURL:    defaultYouTubeAPIBase,
		userAgent:            "streamclone/1.0",
		redditBackoff:        map[string]time.Time{},
		youtubeBackoff:       map[string]time.Time{},
		redditLSFLowPriority: true,
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

type redditToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

func (h *Handler) WithHelix(hx HelixClient) *Handler {
	h.hx = hx
	return h
}

func (h *Handler) WithExternalSources(twitchTrackerAPIURL, redditBaseURL, userAgent string) *Handler {
	if twitchTrackerAPIURL != "" {
		h.twitchTrackerAPIURL = strings.TrimRight(twitchTrackerAPIURL, "/")
	}
	if redditBaseURL != "" {
		h.redditBaseURL = strings.TrimRight(redditBaseURL, "/")
	}
	if userAgent != "" {
		h.userAgent = userAgent
	}
	return h
}

func (h *Handler) WithRedditOptions(opts RedditOptions) *Handler {
	if opts.Provider != "" {
		h.redditProvider = strings.ToLower(strings.TrimSpace(opts.Provider))
	}
	if opts.BaseURL != "" {
		h.redditBaseURL = strings.TrimRight(opts.BaseURL, "/")
	}
	if opts.OAuthAPIURL != "" {
		h.redditOAuthAPIURL = strings.TrimRight(opts.OAuthAPIURL, "/")
	}
	if opts.TokenURL != "" {
		h.redditTokenURL = strings.TrimRight(opts.TokenURL, "/")
	}
	h.redditClientID = opts.ClientID
	h.redditClientSecret = opts.ClientSecret
	h.redditAccessToken = opts.AccessToken
	h.redditHTMLFallback = opts.HTMLFallback
	if opts.ThirdPartyURL != "" {
		h.redditThirdPartyURL = strings.TrimRight(opts.ThirdPartyURL, "/")
	}
	h.redditThirdPartyKey = opts.ThirdPartyKey
	scraperURL := opts.ScraperURL
	if scraperURL == "" {
		scraperURL = opts.FirecrawlURL
	}
	if scraperURL != "" {
		h.scraperAPIURL = strings.TrimRight(scraperURL, "/")
	}
	scraperKey := opts.ScraperKey
	if scraperKey == "" {
		scraperKey = opts.FirecrawlKey
	}
	h.scraperAPIKey = scraperKey
	h.redditLSFLowPriority = opts.LSFLowPriority
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
		return h.g.TopStreams(r.Context(), limit, cursor)
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

func (h *Handler) fetchStreamPool(ctx context.Context, pool int) (gql.Page[gql.Stream], error) {
	if pool <= 0 {
		pool = 100
	}
	pageSize := 25
	out := gql.Page[gql.Stream]{Items: make([]gql.Stream, 0, min(pool, pageSize))}
	cursor := ""
	seen := map[string]struct{}{}
	for len(out.Items) < pool {
		page, err := h.g.TopStreams(ctx, pageSize, cursor)
		if err != nil {
			if len(out.Items) > 0 {
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
			if len(out.Items) >= pool {
				break
			}
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
		out.Cursor = cursor
	}
	return out, nil
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
		// Scraper/mock API hosts only implement summary + /v2/scrape — HTML pages live on TwitchTracker.
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
	return sourceWithProvider("reddit_lsf", "warmup", "unavailable", "fetching from Reddit; first load may take a couple of minutes")
}

func redditLSFPendingStatus() model.SourceStatus {
	return sourceWithProvider("reddit_lsf", "pending", "unavailable", "ready to search Reddit when Analytics is idle")
}

func (h *Handler) fetchRedditLSFCached(ctx context.Context, login, period, sort string, refresh bool) ([]model.RedditPost, []model.SourceStatus) {
	key := redditLSFCacheKey(login, period, sort)
	if _, warming := h.lsfWarmInFlight.Load(key); warming {
		return []model.RedditPost{}, []model.SourceStatus{redditLSFWarmingStatus()}
	}
	if v, ok := h.c.GetFresh(ctx, key); ok {
		var cached redditLSFCacheValue
		if json.Unmarshal(v, &cached) == nil {
			return cached.Items, cached.Sources
		}
	}
	if result, err := h.c.Get(ctx, key); err == nil {
		var cached redditLSFCacheValue
		if json.Unmarshal(result.Data, &cached) == nil {
			if refresh && result.Stale {
				h.kickRedditLSFWarm(login, period, sort)
				return []model.RedditPost{}, []model.SourceStatus{redditLSFWarmingStatus()}
			}
			return cached.Items, cached.Sources
		}
	}
	if refresh {
		h.kickRedditLSFWarm(login, period, sort)
		return []model.RedditPost{}, []model.SourceStatus{redditLSFWarmingStatus()}
	}
	return []model.RedditPost{}, []model.SourceStatus{redditLSFPendingStatus()}
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

func redditLSFRequestContext(parent context.Context) (context.Context, context.CancelFunc) {
	// LSF scraper runs can take several minutes across JSON + old.reddit search URLs.
	return context.WithTimeout(context.WithoutCancel(parent), 12*time.Minute)
}

func redditLSFStatusInterrupted(status model.SourceStatus) bool {
	msg := strings.ToLower(status.Message)
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
}

func normalizeRedditLSFStatus(status model.SourceStatus) model.SourceStatus {
	if status.State == "error" && redditLSFStatusInterrupted(status) {
		status.State = "unavailable"
		status.Message = "fetching from Reddit; first load may take a couple of minutes"
	}
	return status
}

func (h *Handler) fetchRedditLSF(ctx context.Context, login, period, sort string) ([]model.RedditPost, []model.SourceStatus) {
	if h.redditBaseURL == "" {
		return nil, []model.SourceStatus{sourceWithProvider("reddit_lsf", "none", "unavailable", "api url not configured")}
	}

	ctx, cancel := redditLSFRequestContext(ctx)
	defer cancel()

	provider := normalizeRedditProvider(h.redditProvider)
	providersToTry := h.redditLSFProvidersToTry(provider)

	var statuses []model.SourceStatus

	for _, p := range providersToTry {
		var posts []model.RedditPost
		var status model.SourceStatus
		tried := false

		switch p {
		case "official":
			if h.redditClientID != "" || h.redditAccessToken != "" {
				posts, status = h.fetchRedditLSFOfficial(ctx, login, period, sort)
				tried = true
			}
		case "public_json":
			posts, status = h.fetchRedditLSFJSON(ctx, login, period, sort)
			tried = true
		case "third_party":
			if h.redditThirdPartyURL != "" {
				posts, status = h.fetchRedditLSFThirdParty(ctx, login, period, sort)
				tried = true
			}
		case "scraper":
			if h.scraperAPIKey != "" {
				posts, status = h.fetchRedditLSFScraper(ctx, login, period, sort)
				tried = true
			}
		}

		if tried {
			status = normalizeRedditLSFStatus(status)
			statuses = append(statuses, status)
			if status.State == "ready" && len(posts) > 0 {
				return posts, statuses
			}
			if p == "public_json" && status.State == "ready" && len(posts) == 0 && !h.redditLSFLowPriority {
				if recent, recentStatus := h.fetchRedditLSFRecentHot(ctx, login); len(recent) > 0 {
					statuses = append(statuses, recentStatus)
					return recent, statuses
				} else if recentStatus.State != "" {
					statuses = append(statuses, recentStatus)
				}
			}
		}
	}

	if !h.redditLSFLowPriority && (provider == "off" || provider == "auto") && !redditStatusContainsProvider(statuses, "public_json_hot") && !redditStatusContainsProvider(statuses, "scraper_hot") {
		if recent, recentStatus := h.fetchRedditLSFRecentHot(ctx, login); len(recent) > 0 {
			statuses = append(statuses, recentStatus)
			return recent, statuses
		} else if recentStatus.State != "" && !redditStatusContainsProvider(statuses, recentStatus.Provider) {
			statuses = append(statuses, recentStatus)
		}
	}

	if h.redditHTMLFallback && provider != "scraper" && provider != "firecrawl" {
		htmlPosts, htmlStatus := h.fetchRedditLSFHTML(ctx, login, period, sort)
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

func redditStatusContainsProvider(statuses []model.SourceStatus, provider string) bool {
	for _, s := range statuses {
		if s.Provider == provider {
			return true
		}
	}
	return false
}

// redditLSFProvidersToTry picks fetch paths for LSF highlights.
// When REDDIT_PROVIDER=off (compose default), use a lightweight chain:
// public Reddit JSON first, optional scraper when Reddit blocks JSON, then HTML fallback.
func (h *Handler) redditLSFProvidersToTry(provider string) []string {
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
	if h.scraperAPIKey != "" {
		providers = append(providers, "scraper")
	}
	return providers
}

// fetchRedditLSFRecentHot scans the LSF hot feed and keeps posts that mention the streamer.
func (h *Handler) fetchRedditLSFRecentHot(ctx context.Context, login string) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := h.redditBackoffActive("public_json_hot"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "public_json_hot", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := url.Parse(strings.TrimRight(h.redditBaseURL, "/") + "/r/LivestreamFail/hot.json")
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json_hot", "error", err.Error())
		h.markRedditBackoff("public_json_hot", status)
		return nil, status
	}
	q := u.Query()
	q.Set("limit", "25")
	q.Set("raw_json", "1")
	u.RawQuery = q.Encode()

	req, err := h.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json_hot", "error", err.Error())
		h.markRedditBackoff("public_json_hot", status)
		return nil, status
	}
	posts, status := h.doRedditListing(req, "public_json_hot", login)
	if status.State == "ready" {
		h.markRedditBackoff("public_json_hot", status)
		filtered := filterRedditPostsForLogin(posts, login)
		if len(filtered) > 0 {
			return filtered, sourceWithProvider("reddit_lsf", "public_json_hot", "ready", "")
		}
		return nil, sourceWithProvider("reddit_lsf", "public_json_hot", "unavailable", "no recent hot posts matched this streamer")
	}
	if h.scraperAPIKey != "" && (status.State == "blocked" || status.State == "error") {
		hotURL := strings.TrimRight(h.redditBaseURL, "/") + "/r/LivestreamFail/hot/"
		scrapePosts, scrapeStatus := h.scrapeRedditListingURL(ctx, hotURL, login, "scraper_hot", 30000)
		filtered := filterRedditPostsForLogin(scrapePosts, login)
		if len(filtered) > 0 {
			return filtered, scrapeStatus
		}
		if len(scrapePosts) > 0 {
			scrapeStatus = sourceWithProvider("reddit_lsf", "scraper_hot", "unavailable", "no recent hot posts matched this streamer")
		}
		if scrapeStatus.State != "" {
			return nil, scrapeStatus
		}
	}
	h.markRedditBackoff("public_json_hot", status)
	return nil, status
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

func (h *Handler) fetchRedditLSFOfficial(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := h.redditBackoffActive("official"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "official", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	token, status := h.redditBearer(ctx)
	if status.State != "ready" {
		return nil, status
	}
	u, err := h.redditSearchURL(h.redditOAuthAPIURL, "", login, period, sort)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "official", "error", err.Error())
		h.markRedditBackoff("official", status)
		return nil, status
	}
	req, err := h.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "official", "error", err.Error())
		h.markRedditBackoff("official", status)
		return nil, status
	}
	req.Header.Set("Authorization", "Bearer "+token)
	posts, status := h.doRedditListing(req, "official", login)
	h.markRedditBackoff("official", status)
	return posts, status
}

func (h *Handler) fetchRedditLSFJSON(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if until, ok := h.redditBackoffActive("public_json"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "public_json", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := h.redditSearchURL(h.redditBaseURL, ".json", login, period, sort)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json", "error", err.Error())
		h.markRedditBackoff("public_json", status)
		return nil, status
	}

	req, err := h.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "public_json", "error", err.Error())
		h.markRedditBackoff("public_json", status)
		return nil, status
	}
	posts, status := h.doRedditListing(req, "public_json", login)
	h.markRedditBackoff("public_json", status)
	return posts, status
}

func (h *Handler) fetchRedditLSFThirdParty(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if h.redditThirdPartyURL == "" {
		return nil, sourceWithProvider("reddit_lsf", "third_party", "unavailable", "third-party url not configured")
	}
	if until, ok := h.redditBackoffActive("third_party"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "third_party", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	u, err := url.Parse(h.redditThirdPartyURL)
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "third_party", "error", err.Error())
		h.markRedditBackoff("third_party", status)
		return nil, status
	}
	q := u.Query()
	q.Set("subreddit", "LivestreamFail")
	q.Set("q", login)
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	q.Set("limit", "8")
	u.RawQuery = q.Encode()
	req, err := h.newRedditGet(ctx, u.String())
	if err != nil {
		status := sourceWithProvider("reddit_lsf", "third_party", "error", err.Error())
		h.markRedditBackoff("third_party", status)
		return nil, status
	}
	if h.redditThirdPartyKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.redditThirdPartyKey)
	}
	posts, status := h.doRedditListing(req, "third_party", login)
	h.markRedditBackoff("third_party", status)
	return posts, status
}

func (h *Handler) fetchRedditLSFScraper(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	if h.scraperAPIKey == "" {
		return nil, sourceWithProvider("reddit_lsf", "scraper", "unavailable", "scraper api key not configured")
	}
	if until, ok := h.redditBackoffActive("scraper"); ok {
		return nil, model.SourceStatus{Source: "reddit_lsf", Provider: "scraper", State: "blocked", Message: "provider in backoff", BackoffUntil: until.UnixMilli()}
	}
	if h.redditLSFLowPriority {
		if oldURL := h.redditOldSearchURL(login, period, sort); oldURL != "" {
			posts, status := h.scrapeRedditListingURL(ctx, oldURL, login, "scraper", 45000)
			h.markRedditBackoff("scraper", status)
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
	for _, pageURL := range h.redditScraperSearchURLs(login, period, sort) {
		posts, status := h.scrapeRedditListingURL(ctx, pageURL, login, "scraper", 120000)
		lastStatus = status
		if len(posts) > 0 {
			h.markRedditBackoff("scraper", status)
			return posts, status
		}
	}
	status := lastStatus
	if status.State == "" {
		status = sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search did not contain posts for this streamer")
	} else if status.State == "ready" {
		status = sourceWithProvider("reddit_lsf", "scraper", "unavailable", "search did not contain posts for this streamer")
	}
	hotURL := strings.TrimRight(h.redditBaseURL, "/") + "/r/LivestreamFail/hot/"
	hotPosts, hotStatus := h.scrapeRedditListingURL(ctx, hotURL, login, "scraper_hot", 90000)
	filteredHot := filterRedditPostsForLogin(hotPosts, login)
	if len(filteredHot) > 0 {
		h.markRedditBackoff("scraper", hotStatus)
		return filteredHot, hotStatus
	}
	if len(hotPosts) > 0 {
		hotStatus = sourceWithProvider("reddit_lsf", "scraper_hot", "unavailable", "no recent hot posts matched this streamer")
	}
	if hotStatus.State != "" && hotStatus.State != "ready" {
		h.markRedditBackoff("scraper", hotStatus)
		return nil, hotStatus
	}
	h.markRedditBackoff("scraper", status)
	return nil, status
}

func (h *Handler) scrapeRedditListingURL(ctx context.Context, pageURL, login, provider string, timeoutMs int) ([]model.RedditPost, model.SourceStatus) {
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	body, _ := json.Marshal(map[string]any{
		"url":             pageURL,
		"formats":         []string{"html"},
		"onlyMainContent": false,
		"maxAge":          300000,
		"timeout":         timeoutMs,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.scraperAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.scraperAPIKey)
	if h.userAgent != "" {
		req.Header.Set("User-Agent", h.userAgent)
	}
	resp, err := h.scrapeHTTP.Do(req)
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
		if posts, err := decodeRedditPosts([]byte(trimmed), h.redditBaseURL, login); err == nil && len(posts) > 0 {
			return posts, sourceWithProvider("reddit_lsf", provider, "ready", "")
		}
	}
	posts := parseRedditHTMLListing(htmlBody, h.redditBaseURL, login)
	status := sourceWithProvider("reddit_lsf", provider, "ready", "")
	if len(posts) == 0 {
		status = sourceWithProvider("reddit_lsf", provider, "unavailable", "scrape did not contain usable posts")
	}
	return posts, status
}

func (h *Handler) redditSearchURL(base, suffix, login, period, sort string) (*url.URL, error) {
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

func (h *Handler) redditOldSearchURL(login, period, sort string) string {
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

func (h *Handler) redditScraperSearchURLs(login, period, sort string) []string {
	urls := make([]string, 0, 2)
	if u, err := h.redditSearchURL(h.redditBaseURL, ".json", login, period, sort); err == nil {
		urls = append(urls, u.String())
	}
	if oldURL := h.redditOldSearchURL(login, period, sort); oldURL != "" {
		urls = append(urls, oldURL)
	}
	return urls
}

func (h *Handler) newRedditGet(ctx context.Context, raw string) (*http.Request, error) {
	if strings.TrimSpace(h.userAgent) == "" {
		return nil, fmt.Errorf("user agent is required for reddit requests")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set("Accept", "application/json,text/html;q=0.8")
	return req, nil
}

func (h *Handler) doRedditListing(req *http.Request, provider, login string) ([]model.RedditPost, model.SourceStatus) {
	resp, err := h.http.Do(req)
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
	posts, err := decodeRedditPosts(body, h.redditBaseURL, login)
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf", provider, "error", err.Error())
	}
	return posts, sourceWithProvider("reddit_lsf", provider, "ready", "")
}

func decodeRedditPosts(body []byte, redditBaseURL, login string) ([]model.RedditPost, error) {
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
		thumbnail := item.Thumbnail
		if thumbnail == "self" || thumbnail == "default" || thumbnail == "nsfw" {
			thumbnail = ""
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
		}
		enrichRedditTag(&post, login)
		posts = append(posts, post)
	}
	return posts, nil
}

func (h *Handler) fetchRedditLSFHTML(ctx context.Context, login, period, sort string) ([]model.RedditPost, model.SourceStatus) {
	u, err := url.Parse(strings.TrimRight(h.redditBaseURL, "/") + "/r/LivestreamFail/search")
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}
	q := u.Query()
	q.Set("q", login)
	q.Set("restrict_sr", "1")
	q.Set("sort", normalizeSort(sort))
	q.Set("t", redditTime(period))
	u.RawQuery = q.Encode()

	req, err := h.newRedditGet(ctx, u.String())
	if err != nil {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "error", err.Error())
	}

	resp, err := h.http.Do(req)
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
	posts := parseRedditHTMLListing(string(body), h.redditBaseURL, login)
	if len(posts) == 0 {
		return nil, sourceWithProvider("reddit_lsf_html", "html", "unavailable", "public listing did not contain usable posts")
	}
	return posts, sourceWithProvider("reddit_lsf_html", "html", "fallback", "json listing unavailable")
}

func (h *Handler) redditBearer(ctx context.Context) (string, model.SourceStatus) {
	if h.redditAccessToken != "" {
		return h.redditAccessToken, sourceWithProvider("reddit_lsf", "official", "ready", "using configured access token")
	}
	if h.redditClientID == "" {
		return "", sourceWithProvider("reddit_lsf", "official", "unavailable", "reddit client id not configured")
	}
	if strings.TrimSpace(h.userAgent) == "" {
		return "", sourceWithProvider("reddit_lsf", "official", "error", "user agent is required for reddit requests")
	}
	h.redditMu.Lock()
	if h.redditToken.AccessToken != "" && time.Until(h.redditToken.ExpiresAt) > time.Minute {
		token := h.redditToken.AccessToken
		h.redditMu.Unlock()
		return token, sourceWithProvider("reddit_lsf", "official", "ready", "using cached oauth token")
	}
	h.redditMu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.redditTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", sourceWithProvider("reddit_lsf", "official", "error", err.Error())
	}
	req.SetBasicAuth(h.redditClientID, h.redditClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", h.userAgent)
	resp, err := h.http.Do(req)
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
	h.redditMu.Lock()
	h.redditToken = redditToken{AccessToken: out.AccessToken, ExpiresAt: expires}
	h.redditMu.Unlock()
	return out.AccessToken, sourceWithProvider("reddit_lsf", "official", "ready", "oauth token refreshed")
}

func (h *Handler) redditBackoffActive(provider string) (time.Time, bool) {
	h.redditMu.Lock()
	until, ok := h.redditBackoff[provider]
	active := ok && time.Now().Before(until)
	if !active {
		if ok {
			delete(h.redditBackoff, provider)
		}
		h.redditMu.Unlock()
		return time.Time{}, false
	}
	h.redditMu.Unlock()
	return until, true
}

func (h *Handler) markRedditBackoff(provider string, status model.SourceStatus) {
	h.redditMu.Lock()
	defer h.redditMu.Unlock()
	if redditLSFStatusInterrupted(status) {
		delete(h.redditBackoff, provider)
		return
	}
	if status.State == "blocked" || status.State == "error" {
		h.redditBackoff[provider] = time.Now().Add(45 * time.Second)
		return
	}
	delete(h.redditBackoff, provider)
}

var redditPostRe = regexp.MustCompile(`(?is)<a[^>]+href="([^"]*/r/LivestreamFail/comments/[^"]+)"[^>]*>(.*?)</a>`)
var redditShredditPostRe = regexp.MustCompile(`(?is)<shreddit-post\b[^>]*>`)
var redditPermalinkAttrRe = regexp.MustCompile(`(?i)permalink="(/r/LivestreamFail/comments/[^"]+)"`)
var redditPostTitleAttrRe = regexp.MustCompile(`(?i)post-title="([^"]+)"`)
var tagRe = regexp.MustCompile(`(?is)<[^>]+>`)
var twitchTrackerRowRe = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
var twitchTrackerCellRe = regexp.MustCompile(`(?is)<td\b([^>]*)>(.*?)</td>`)
var twitchTrackerHrefRe = regexp.MustCompile(`(?is)<a[^>]+href="([^"]+)"`)
var twitchTrackerSpanRe = regexp.MustCompile(`(?is)<span[^>]*>(.*?)</span>`)
var twitchTrackerImgTitleRe = regexp.MustCompile(`(?is)data-original-title="([^"]+)"`)

type redditRichTextPart struct {
	Text string `json:"t"`
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

func parseRedditHTMLListing(body, redditBaseURL, login string) []model.RedditPost {
	out := make([]model.RedditPost, 0, 16)
	seen := map[string]struct{}{}
	appendPost := func(href, title string) {
		title = strings.TrimSpace(html.UnescapeString(title))
		title = strings.Join(strings.Fields(title), " ")
		if title == "" || len(title) < 4 {
			return
		}
		href = html.UnescapeString(href)
		if strings.HasPrefix(href, "/") {
			href = strings.TrimRight(redditBaseURL, "/") + href
		}
		id := redditIDFromURL(href)
		if id == "" {
			id = href
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		post := model.RedditPost{
			ID:        id,
			Title:     title,
			URL:       href,
			Permalink: href,
			Subreddit: "LivestreamFail",
		}
		enrichRedditTag(&post, login)
		out = append(out, post)
	}
	for _, tag := range redditShredditPostRe.FindAllString(body, 16) {
		permalinkMatch := redditPermalinkAttrRe.FindStringSubmatch(tag)
		titleMatch := redditPostTitleAttrRe.FindStringSubmatch(tag)
		if len(permalinkMatch) < 2 || len(titleMatch) < 2 {
			continue
		}
		appendPost(permalinkMatch[1], titleMatch[1])
	}
	for _, match := range redditPostRe.FindAllStringSubmatch(body, 16) {
		title := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(match[2], "")))
		appendPost(match[1], title)
	}
	if len(out) > 8 {
		return out[:8]
	}
	return out
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
