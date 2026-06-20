package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/storygraph/clip"
	"streamclone/internal/storygraph/cluster"
	"streamclone/internal/storygraph/develop"
	"streamclone/internal/storygraph/evidenceurl"
	"streamclone/internal/storygraph/ingest"
	"streamclone/internal/storygraph/preview"
	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/score"
	"streamclone/internal/storygraph/store"
)

const pulseWireRankModel = score.RankModelVersion

// Options configures the storygraph HTTP handler.
type Options struct {
	Store             *store.Store
	Reliability       *reliability.Registry
	Redis             *redis.Client
	Logger            *slog.Logger
	Config            config.Config
	Enabled           bool
	SetupControlToken string
	IngestHealth      *ingest.Health
	SamplerHealth     *ingest.DirectorySamplerHealth
	WindowScoreHealth *ingest.WindowScoreHealth
	Workers           *ingest.Workers
}

// Handler serves Pulse Wire API routes.
type Handler struct {
	store             *store.Store
	rel               *reliability.Registry
	logger            *slog.Logger
	cfg               config.Config
	enabled           bool
	setupToken        string
	develop           *develop.Service
	clipper           *clip.Bridge
	ingestHealth      *ingest.Health
	samplerHealth     *ingest.DirectorySamplerHealth
	windowScoreHealth *ingest.WindowScoreHealth
	workers           *ingest.Workers
	avatars           *avatarEnricher
	preview           *preview.Hydrator
}

// New creates a storygraph API handler.
func New(opts Options) *Handler {
	return &Handler{
		store:             opts.Store,
		rel:               opts.Reliability,
		logger:            opts.Logger,
		cfg:               opts.Config,
		enabled:           opts.Enabled,
		setupToken:        opts.SetupControlToken,
		develop:           develop.New(opts.Store),
		clipper:           clip.New(opts.Config),
		ingestHealth:      opts.IngestHealth,
		samplerHealth:     opts.SamplerHealth,
		windowScoreHealth: opts.WindowScoreHealth,
		workers:           opts.Workers,
		avatars:           newAvatarEnricher(opts.Config, opts.Redis),
		preview:           preview.NewHydrator(opts.Logger),
	}
}

// Routes mounts chi routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if !h.enabled {
		r.Get("/v1/pulse-wire/*", h.disabled)
		r.Get("/v1/channels/{login}/spread", h.disabled)
		r.Post("/v1/channels/{login}/spread/backfill", h.disabled)
		return
	}
	r.Route("/v1/pulse-wire", func(r chi.Router) {
		r.Get("/feed", h.feed)
		r.Get("/trending-streamers", h.trendingStreamers)
		r.Get("/stories/{id}", h.story)
		r.Get("/rising", h.rising)
		r.Get("/rising-streamers", h.risingStreamers)
		r.Get("/streamers/{login}", h.streamerProfile)
		r.Get("/daily", h.dailyEdition)
		r.Get("/edition", h.edition)
		r.Get("/developing", h.developing)
		r.Get("/source-mix", h.sourceMix)
		r.Get("/source-health", h.sourceHealth)
		r.Get("/community", h.community)
		r.Get("/community/flairs", h.communityFlairs)
		r.Get("/clips/top", h.topClips)
		r.Get("/bans", h.bans)
		r.Get("/evidence/unlinked", h.unlinkedEvidence)
		r.Get("/watch-entries", h.watchEntries)
		r.Post("/watch-entries", h.addWatchEntry)
		r.Delete("/watch-entries/{id}", h.deleteWatchEntry)
		r.Get("/thumb", h.mediaThumb)
		r.Post("/reclassify", h.operator(h.reclassify))
		r.Post("/repair/thumbnails", h.operator(h.repairThumbnails))
		r.Post("/developing/{id}/confirm", h.operator(h.confirmDeveloping))
		r.Post("/stories/{id}/follow", h.follow)
		r.Delete("/stories/{id}/follow", h.unfollow)
		r.Post("/stories/{id}/clip", h.operator(h.createClip))
		r.Post("/stories/{id}/evidence", h.operator(h.addEvidence))
		r.Post("/stories/{id}/operator-action", h.operator(h.operatorStoryAction))
	})
	r.Get("/v1/channels/{login}/spread", h.channelSpread)
	r.Post("/v1/channels/{login}/spread/backfill", h.channelSpreadBackfill)
}

func (h *Handler) disabled(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error": "pulse_wire_disabled",
		"hint":  "Set PULSE_WIRE_ENABLED=true to enable Story Graph",
	})
}

// ParseWindow returns the lower bound for supported Pulse Wire ranking windows.
func ParseWindow(raw string) (time.Time, string, error) {
	return parseWindowAt(time.Now(), raw)
}

func parseWindowAt(now time.Time, raw string) (time.Time, string, error) {
	now = now.UTC()
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "24h":
		return now.Add(-24 * time.Hour), "24h", nil
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, "today", nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), "7d", nil
	default:
		return time.Time{}, "", fmt.Errorf("invalid pulse wire window %q", raw)
	}
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	category := r.URL.Query().Get("category")
	login := r.URL.Query().Get("login")
	if login == "" {
		login = r.URL.Query().Get("streamer")
	}
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	sort := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sort == "" {
		sort = "rank"
	}
	items, err := h.store.ListFeed(r.Context(), state, category, login, sort, windowLabel, since, 20, cursor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.avatars.enrichCards(r.Context(), items)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"cursor":    nextCursor(items),
		"window":    windowLabel,
		"since":     since,
		"sort":      sort,
		"rankModel": pulseWireRankModel,
	})
}

func (h *Handler) trendingStreamers(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListTrendingStreamers(r.Context(), since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func (h *Handler) community(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	sort := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sort != "new" {
		sort = "hot"
	}
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	switch category {
	case "drama", "funny", "bans", "records", "esports":
	default:
		category = ""
	}
	flair := strings.TrimSpace(r.URL.Query().Get("flair"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	if category == "bans" {
		if banItems, err := h.store.ListBanEvents(r.Context(), since, limit); err == nil && len(banItems) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"items":  banEventsToCommunityPosts(banItems),
				"window": windowLabel,
				"since":  since,
				"sort":   sort,
				"source": "ban_events",
			})
			return
		}
	}
	items, err := h.store.ListCommunityPosts(r.Context(), sort, category, flair, since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items = filterCommunityPosts(items, category, limit)
	resp := map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
		"sort":   sort,
	}
	if flair != "" {
		resp["flair"] = flair
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) communityFlairs(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListCommunityFlairs(r.Context(), since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func filterCommunityPosts(items []store.CommunityPost, category string, limit int) []store.CommunityPost {
	if len(items) == 0 {
		return []store.CommunityPost{}
	}
	out := make([]store.CommunityPost, 0, limit)
	for _, post := range items {
		resolved := resolveCommunityCategory(post)
		post.Category = resolved
		if category != "" && resolved != category {
			continue
		}
		out = append(out, post)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func resolveCommunityCategory(post store.CommunityPost) string {
	if post.Source == "streamerbans_post" {
		return "bans"
	}
	if cat := strings.TrimSpace(post.Category); cat != "" && cat != "news" {
		return cat
	}
	if cat := cluster.ClassifyCategory(post.Title, post.Flair); cat != "" {
		return cat
	}
	return "news"
}

func (h *Handler) topClips(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListTopClips(r.Context(), since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func (h *Handler) story(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	card, err := h.store.GetStory(r.Context(), id, "local")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if card == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.refreshStalePreviews(r.Context(), id, card.EvidenceGallery)
	if refreshed, err := h.store.GetStory(r.Context(), id, "local"); err == nil && refreshed != nil {
		card = refreshed
	}
	enrichEvidenceGalleryTitles(card)
	h.avatars.enrichCard(r.Context(), card)
	writeJSON(w, http.StatusOK, card)
}

func (h *Handler) rising(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	items, err := h.store.ListRising(r.Context(), since, 5)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"window": windowLabel,
		"since":  since,
	})
}

func (h *Handler) developing(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListDeveloping(r.Context(), 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.avatars.enrichCards(r.Context(), items)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) sourceMix(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	mix, err := h.store.SourceMix(r.Context(), since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	total := 0
	for _, n := range mix {
		total += n
	}
	entries := h.rel.All()
	writeJSON(w, http.StatusOK, map[string]any{
		"mix":         mix,
		"total":       total,
		"reliability": entries,
		"since":       since,
		"window":      windowLabel,
	})
}

func (h *Handler) sourceHealth(w http.ResponseWriter, r *http.Request) {
	lastEvidence, err := h.store.LastEvidenceBySource(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sources := h.configuredSourceModes(lastEvidence)
	if h.ingestHealth != nil {
		for name, status := range h.ingestHealth.Snapshot() {
			entry, _ := sources[name].(map[string]any)
			if entry == nil {
				entry = map[string]any{}
			}
			var evidenceAt *time.Time
			if ts, ok := lastEvidence[sourceTypeForHealth(name)]; ok {
				evidenceAt = &ts
			}
			entry["mode"] = sourceMode(status, sourceConfigured(name, h.cfg), evidenceAt)
			entry["healthy"] = status.Healthy
			entry["last_poll_at"] = status.LastPollAt
			entry["last_items"] = status.LastItems
			if status.LastOKAt != nil {
				entry["last_ok_at"] = status.LastOKAt
			}
			if status.LastErrAt != nil {
				entry["last_err_at"] = status.LastErrAt
			}
			if status.LastError != "" {
				entry["last_error"] = status.LastError
			}
			if evidenceAt != nil {
				entry["last_evidence_at"] = *evidenceAt
			}
			if details := sourceDetailPayload(status.Details); len(details) > 0 {
				entry["details"] = details
			}
			sources[name] = entry
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":            sources,
		"directorySampler":   h.directorySamplerHealth(r.Context()),
		"windowScoreCompute": h.windowScoreComputeHealth(),
	})
}

func sourceDetailPayload(statuses map[string]ingest.SourceDetailStatus) map[string]any {
	if len(statuses) == 0 {
		return nil
	}
	details := map[string]any{}
	for detailName, detailStatus := range statuses {
		detailEntry := map[string]any{
			"healthy":      detailStatus.Healthy,
			"last_poll_at": detailStatus.LastPollAt,
			"last_items":   detailStatus.LastItems,
		}
		if detailStatus.LastOKAt != nil {
			detailEntry["last_ok_at"] = detailStatus.LastOKAt
		}
		if detailStatus.LastErrAt != nil {
			detailEntry["last_err_at"] = detailStatus.LastErrAt
		}
		if detailStatus.LastError != "" {
			detailEntry["last_error"] = detailStatus.LastError
		}
		details[detailName] = detailEntry
	}
	return details
}

func (h *Handler) configuredSourceModes(lastEvidence map[string]time.Time) map[string]any {
	sources := map[string]any{}
	for _, name := range []string{"twitchclips", "reddit", "youtube", "streamerbans"} {
		mode := "off"
		if sourceConfigured(name, h.cfg) {
			mode = "active"
		}
		entry := map[string]any{
			"mode":       mode,
			"healthy":    mode == "active",
			"last_items": 0,
		}
		if ts, ok := lastEvidence[sourceTypeForHealth(name)]; ok {
			entry["last_evidence_at"] = ts
		}
		sources[name] = entry
	}
	sources["x"] = map[string]any{
		"mode":    modeForX(h.cfg),
		"healthy": modeForX(h.cfg) == "link_only",
		"hint":    "X appears through extracted links/oEmbed unless optional x-ingest is enabled.",
	}
	sources["tiktok"] = map[string]any{
		"mode":    "link_only",
		"healthy": true,
		"hint":    "TikTok links render as evidence previews; no direct discovery source is enabled.",
	}
	sources["instagram"] = map[string]any{
		"mode":    "link_only",
		"healthy": true,
		"hint":    "Instagram is link-only evidence.",
	}
	sources["kick"] = map[string]any{
		"mode":    "deferred",
		"healthy": false,
		"hint":    "Kick discovery is planned after Evidence Gallery proves useful.",
	}
	return sources
}

func sourceConfigured(name string, cfg config.Config) bool {
	switch name {
	case "reddit":
		return cfg.RedditCommercialOK
	case "youtube":
		return len(cfg.StorygraphYTKeywords) > 0
	case "streamerbans":
		return cfg.StreamerbansIngestEnabled
	case "twitchclips":
		return true
	default:
		return false
	}
}

func sourceMode(status ingest.SourceStatus, configured bool, lastEvidence *time.Time) string {
	if !configured {
		return "off"
	}
	if !status.Healthy && status.LastError != "" {
		if status.LastOKAt != nil || lastEvidence != nil {
			return "degraded"
		}
		return "error"
	}
	return "active"
}

func modeForX(cfg config.Config) string {
	if cfg.XUnofficialOK && cfg.XContentToken() != "" {
		return "active"
	}
	return "link_only"
}

func sourceTypeForHealth(sourceName string) string {
	switch sourceName {
	case "youtube":
		return "youtube_video"
	case "twitchclips":
		return "twitch_clip"
	case "streamerbans":
		return "streamerbans_post"
	default:
		return sourceName + "_thread"
	}
}

func (h *Handler) confirmDeveloping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" {
		action = "confirm"
	}
	if err := h.develop.Confirm(r.Context(), id, action); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "action": action})
}

func (h *Handler) follow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.store.Follow(r.Context(), id, "local"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "tracked"})
}

func (h *Handler) unfollow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.store.Unfollow(r.Context(), id, "local"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "untracked"})
}

func (h *Handler) watchEntries(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListWatchEntries(r.Context(), "local")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) addWatchEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := h.store.UpsertWatchEntry(r.Context(), "local", body.Kind, body.Value, body.Label)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "watched", "item": item})
}

func (h *Handler) deleteWatchEntry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.store.DeleteWatchEntry(r.Context(), "local", id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "watch entry not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unwatched"})
}

func (h *Handler) createClip(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	result, err := h.clipper.TriggerForStory(r.Context(), h.store, id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) addEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	card, err := h.store.GetStory(r.Context(), id, "local")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if card == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var body struct {
		URL      string `json:"url"`
		Note     string `json:"note"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	rawURL := strings.TrimSpace(body.URL)
	if rawURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid url",
			"hint":  "Paste a full https:// link from Reddit, YouTube, Twitch, X, or TikTok.",
		})
		return
	}
	link, ok := evidenceurl.Canonicalize(rawURL)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid url",
			"hint":  "Could not parse that URL. Use a public post or clip link with https://.",
		})
		return
	}
	if existing, linked, err := h.store.FindPreviewLinkByCanonical(r.Context(), id, link.CanonicalURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	} else if linked && existing != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "already_attached",
			"preview":   existing,
			"matchKind": existing.MatchKind,
			"matchExplanation": map[string]any{
				"matchedBy":  "manual",
				"sourceType": preview.SourceTypeForPlatform(link.Platform),
			},
		})
		return
	}
	operator := strings.TrimSpace(body.Operator)
	if operator == "" {
		operator = "operator"
	}
	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "manual evidence"
	}
	auditNote := note
	if operator != "" {
		auditNote = operator + " · " + note
	}
	hash := sha256.Sum256([]byte(link.CanonicalURL))
	externalID := hex.EncodeToString(hash[:])
	now := time.Now()
	metrics, _ := json.Marshal(map[string]float64{})
	socialID, err := h.store.UpsertSocialItem(r.Context(), store.SocialItem{
		Source:       "manual",
		Kind:         "link",
		ExternalID:   externalID,
		URL:          link.CanonicalURL,
		Author:       operator,
		CreatedAtSrc: &now,
		Text:         auditNote,
		Metrics:      metrics,
		ExpiresAt:    now.Add(time.Duration(h.cfg.SocialRetentionDays) * 24 * time.Hour),
	}, json.RawMessage(`{"sourceApi":"operator","requestId":"manual-evidence"}`), hash[:])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sourceType := preview.SourceTypeForPlatform(link.Platform)
	var evidenceID int64
	if existingEv, err := h.store.FindManualEvidenceByURL(r.Context(), id, link.CanonicalURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	} else if existingEv != nil {
		evidenceID = existingEv.ID
	} else {
		evidenceID, err = h.store.InsertEvidence(r.Context(), store.Evidence{
			ClusterID:  id,
			ItemID:     &socialID,
			SourceType: sourceType,
			SourceURL:  link.CanonicalURL,
			MatchConf:  0.95,
			Weight:     h.rel.Weight(sourceType),
			OccurredAt: &now,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	evID := evidenceID
	previewCard, alreadyLinked, err := h.preview.AttachURL(r.Context(), h.store, id, &evID, link.CanonicalURL, "manual", auditNote, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	status := "ok"
	if alreadyLinked {
		status = "already_attached"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  status,
		"preview": previewCard,
		"matchExplanation": map[string]any{
			"matchedBy":     "manual",
			"sourceType":    sourceType,
			"confidence":    0.95,
			"previewStatus": previewCard.PreviewStatus,
		},
	})
}

func (h *Handler) operatorStoryAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Action          string  `json:"action"`
		Note            string  `json:"note"`
		Operator        string  `json:"operator"`
		EntityID        int64   `json:"entityId"`
		MomentFPID      int64   `json:"momentFpId"`
		TargetClusterID int64   `json:"targetClusterId"`
		EvidenceIDs     []int64 `json:"evidenceIds"`
		Title           string  `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	operator := strings.TrimSpace(body.Operator)
	if operator == "" {
		operator = "operator"
	}
	var action *store.OperatorAction
	switch strings.ToLower(strings.TrimSpace(body.Action)) {
	case "confirm_streamer_entity":
		action, err = h.store.ConfirmStoryEntity(r.Context(), id, body.EntityID, operator, body.Note)
	case "confirm_origin_moment":
		action, err = h.store.ConfirmStoryOrigin(r.Context(), id, body.MomentFPID, operator, body.Note)
	case "merge_duplicate_story":
		action, err = h.store.MergeDuplicateStory(r.Context(), id, body.TargetClusterID, operator, body.Note)
	case "split_unrelated_evidence":
		action, err = h.store.SplitUnrelatedEvidence(r.Context(), id, body.EvidenceIDs, body.Title, operator, body.Note)
	default:
		action, err = h.store.MarkStory(r.Context(), id, body.Action, operator, body.Note)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	card, _ := h.store.GetStory(r.Context(), id, "local")
	if card != nil {
		h.avatars.enrichCards(r.Context(), []store.StoryCard{*card})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"action": action,
		"story":  card,
	})
}

func (h *Handler) refreshStalePreviews(ctx context.Context, clusterID int64, gallery []store.EvidencePreview) {
	if h.preview == nil || h.store == nil || len(gallery) == 0 {
		return
	}
	for _, item := range gallery {
		if item.PreviewStatus != "error" && item.PreviewStatus != "fallback" {
			continue
		}
		if !item.ExpiresAt.IsZero() && time.Now().Before(item.ExpiresAt) {
			continue
		}
		link, ok := evidenceurl.Canonicalize(item.CanonicalURL)
		if !ok {
			continue
		}
		if _, _, err := h.preview.AttachURL(ctx, h.store, clusterID, nil, link.CanonicalURL, item.MatchKind, item.Note, ""); err != nil {
			h.logger.Warn("preview refresh failed", "url", item.CanonicalURL, "err", err)
		}
	}
}

func enrichEvidenceGalleryTitles(card *store.StoryCard) {
	if card == nil || len(card.EvidenceGallery) == 0 {
		return
	}
	storyTitle := strings.TrimSpace(card.Cluster.Title)
	var previewTitles []string
	for i := range card.EvidenceGallery {
		title := strings.TrimSpace(card.EvidenceGallery[i].Title)
		if title != "" {
			previewTitles = append(previewTitles, title)
		}
		if card.EvidenceGallery[i].Platform != evidenceurl.PlatformTwitchClip {
			continue
		}
		if title != "" {
			continue
		}
		if storyTitle != "" {
			card.EvidenceGallery[i].Title = storyTitle
			previewTitles = append(previewTitles, storyTitle)
		}
	}
	if store.IsPlaceholderTitle(card.Cluster.Title) {
		if resolved := store.ResolveDisplayTitle(card.Cluster.Title, "", previewTitles, ""); resolved != "" {
			card.Cluster.Title = resolved
		}
	}
}

func (h *Handler) risingStreamers(w http.ResponseWriter, r *http.Request) {
	rawWindow := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("window")))
	if rawWindow == "" {
		rawWindow = "today"
	}
	since, windowLabel, err := ParseWindow(rawWindow)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	category := r.URL.Query().Get("category")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.RisingCandidates(r.Context(), windowLabel, category, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i := range items {
		series, err := h.store.ViewerSeriesForLogin(r.Context(), items[i].Login, since, 24)
		if err == nil {
			items[i].ViewerSeries = series
		}
		storyID, title, _ := h.store.TopStoryForLogin(r.Context(), items[i].Login, since)
		items[i].TopStoryID = storyID
		items[i].TopStoryTitle = title
		if h.avatars != nil {
			if url, err := h.avatars.avatarForLogin(r.Context(), items[i].Login); err == nil {
				items[i].AvatarURL = url
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"window":       windowLabel,
		"since":        since,
		"sampleStatus": h.directorySampleStatus(r.Context(), windowLabel),
		"rankModel":    pulseWireRankModel,
	})
}

func (h *Handler) streamerProfile(w http.ResponseWriter, r *http.Request) {
	login := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "login")))
	if login == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login"})
		return
	}
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	profile, err := h.store.StreamerProfile(r.Context(), login, since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if profile == nil {
		profile = &store.StreamerStatProfile{Login: login}
	}
	h.enrichProfileFromMetadata(r.Context(), profile)
	if profile.DisplayName == "" && profile.ViewersNow == 0 && len(profile.ViewerSeries) == 0 && len(profile.RecentStories) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.avatars != nil {
		if url, err := h.avatars.avatarForLogin(r.Context(), login); err == nil {
			profile.AvatarURL = url
		}
	}
	h.avatars.enrichCards(r.Context(), profile.RecentStories)
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":      profile,
		"window":       windowLabel,
		"since":        since,
		"sampleStatus": h.directorySampleStatus(r.Context(), windowLabel),
		"rankModel":    pulseWireRankModel,
	})
}

func (h *Handler) dailyEdition(w http.ResponseWriter, r *http.Request) {
	rawDate := strings.TrimSpace(r.URL.Query().Get("date"))
	day := time.Now()
	if rawDate != "" {
		parsed, err := time.Parse("2006-01-02", rawDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid_date",
				"hint":  "date must be YYYY-MM-DD",
			})
			return
		}
		day = parsed
	}
	edition, err := h.store.DailyEdition(r.Context(), day, 10)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if edition != nil {
		h.enrichDailyEdition(r.Context(), edition)
	}
	writeJSON(w, http.StatusOK, edition)
}

func (h *Handler) enrichDailyEdition(ctx context.Context, edition *store.DailyEdition) {
	if edition == nil || h.avatars == nil {
		return
	}
	for i := range edition.TopGainers {
		if url, err := h.avatars.avatarForLogin(ctx, edition.TopGainers[i].Login); err == nil {
			edition.TopGainers[i].AvatarURL = url
		}
	}
	for i := range edition.TopDroppers {
		if url, err := h.avatars.avatarForLogin(ctx, edition.TopDroppers[i].Login); err == nil {
			edition.TopDroppers[i].AvatarURL = url
		}
	}
	for i := range edition.NewEntrants {
		if url, err := h.avatars.avatarForLogin(ctx, edition.NewEntrants[i].Login); err == nil {
			edition.NewEntrants[i].AvatarURL = url
		}
	}
	h.avatars.enrichCards(ctx, edition.BansOfTheDay)
	h.avatars.enrichCards(ctx, edition.TopStories)
}

func (h *Handler) channelSpread(w http.ResponseWriter, r *http.Request) {
	login := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "login")))
	items, err := h.store.SpreadForLogin(r.Context(), login, 10)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	exclude := make(map[int64]struct{}, len(items))
	for _, card := range items {
		exclude[card.Cluster.ID] = struct{}{}
	}
	probable, err := h.store.SpreadProbableForLogin(r.Context(), login, exclude, 3)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.avatars.enrichCards(r.Context(), items)
	h.avatars.enrichCards(r.Context(), probable)

	ent, _ := h.store.EntityByLogin(r.Context(), login)
	unresolved, _ := h.store.CountUnresolvedMentionsForLogin(r.Context(), login)

	meta := map[string]any{
		"entityKnown":             ent != nil,
		"aliases":                 store.EntityDisplayAliases(ent),
		"unresolvedMentionCount":  unresolved,
		"backfill":                h.spreadBackfillMeta(login),
	}
	if h.workers != nil {
		if ts := h.workers.LastIngestAt(); ts != nil {
			meta["lastIngestAt"] = ts.UTC().Format(time.RFC3339)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"login":         login,
		"items":         items,
		"probableItems": wrapProbableSpreadCards(probable),
		"meta":          meta,
	})
}

func (h *Handler) channelSpreadBackfill(w http.ResponseWriter, r *http.Request) {
	login := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "login")))
	if login == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login"})
		return
	}
	if h.workers == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "spread backfill unavailable"})
		return
	}
	meta, err := h.workers.RequestSpreadBackfill(r.Context(), login)
	if errors.Is(err, ingest.ErrSpreadBackfillCooldown) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":    "spread_backfill_cooldown",
			"hint":     "Try again in a few minutes.",
			"state":    meta.State,
			"backfill": meta,
		})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"state":    meta.State,
		"backfill": meta,
	})
}

func (h *Handler) spreadBackfillMeta(login string) ingest.SpreadBackfillMeta {
	if h.workers == nil {
		return ingest.SpreadBackfillMeta{State: "idle"}
	}
	return h.workers.SpreadBackfillMeta(login)
}

func wrapProbableSpreadCards(cards []store.StoryCard) []map[string]any {
	if len(cards) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		raw, err := json.Marshal(card)
		if err != nil {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		payload["matchTier"] = "probable"
		out = append(out, payload)
	}
	return out
}

func (h *Handler) operator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(h.setupToken) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "operator token not configured"})
			return
		}
		provided := r.Header.Get("X-Streamclone-Setup-Token")
		if provided == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if provided != h.setupToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func nextCursor(items []store.StoryCard) int64 {
	if len(items) == 0 {
		return 0
	}
	return items[len(items)-1].Cluster.ID
}
