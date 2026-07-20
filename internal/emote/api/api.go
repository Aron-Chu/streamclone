package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/flags"
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/seeder"
	"streamclone/internal/emote/store"
	emotesync "streamclone/internal/emote/sync"
	"streamclone/internal/emoteimage"
	"streamclone/internal/metrics"
)

const maxUploadBytes = 5 << 20

type Handler struct {
	st              *store.Store
	obj             *objstore.Client
	d               *dict.Dict
	seed            *seeder.Seeder
	render          *render.Queue
	log             *slog.Logger
	token           string
	eventSub        eventSubscriber
	ensure          func(context.Context, string, string, []seeder.Provider) (ensureResponse, int, error)
	seedMu          sync.Mutex
	seeding         map[string]struct{}
	loadedMu        sync.Mutex
	loadedProviders map[string]map[seeder.Provider]struct{}
}

type eventSubscriber interface {
	Register(ctx context.Context, login, twitchID, providerSetID string) error
	Unregister(login string)
}

func (h *Handler) SetEventSubscriber(sub eventSubscriber) {
	h.eventSub = sub
}

func New(st *store.Store, obj *objstore.Client, d *dict.Dict, seed *seeder.Seeder, log *slog.Logger, token string) *Handler {
	return NewWithRenderQueue(st, obj, d, seed, nil, log, token)
}

func NewWithRenderQueue(st *store.Store, obj *objstore.Client, d *dict.Dict, seed *seeder.Seeder, rq *render.Queue, log *slog.Logger, token string) *Handler {
	return &Handler{st: st, obj: obj, d: d, seed: seed, render: rq, log: log, token: token, seeding: make(map[string]struct{}), loadedProviders: make(map[string]map[seeder.Provider]struct{})}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/emotes/{id}/{scale}.webp", h.serveEmoteAsset)
	r.Get("/v1/channels/{login}/emotes", h.listChannelEmotes)
	r.Post("/v1/channels/{login}/emotes/ensure", h.ensureChannelEmotes)

	r.Group(func(r chi.Router) {
		r.Use(h.bearerAuth)
		r.Post("/v1/emotes", h.uploadEmote)
		r.Post("/v1/sets", h.createSet)
		r.Post("/v1/sets/{id}/items", h.addItem)
		r.Delete("/v1/sets/{id}/items/{emote_id}", h.removeItem)
		r.Put("/v1/channels/{twitch_id}/active-set", h.setActiveSet)
		r.Post("/v1/seed/twitch/{twitch_id}", h.seedChannel)
	})
}

// etagMatches compares If-None-Match against a bare object ETag (no quotes).
// Uses exact token match after stripping weak markers/quotes — never substring.
func etagMatches(ifNoneMatch, etag string) bool {
	etag = strings.Trim(strings.TrimSpace(etag), `"`)
	if etag == "" || strings.TrimSpace(ifNoneMatch) == "" {
		return false
	}
	for _, part := range strings.Split(ifNoneMatch, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		part = strings.TrimPrefix(part, "W/")
		part = strings.Trim(part, `"`)
		if part == etag {
			return true
		}
	}
	return false
}

func (h *Handler) serveEmoteAsset(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	scale := strings.TrimSpace(chi.URLParam(r, "scale"))
	if id == "" || scale == "" {
		http.NotFound(w, r)
		return
	}
	rc, info, err := h.obj.Open(r.Context(), id, scale)
	if err == nil {
		defer rc.Close()
		emote, emoteErr := h.st.GetEmote(r.Context(), id)
		provider := "custom"
		if emoteErr == nil && emote.Provider != "" {
			provider = emote.Provider
		}
		metrics.EmoteImageServed.WithLabelValues(provider, scale, "local").Inc()
		w.Header().Set("Content-Type", info.ContentType)
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		if info.Size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		}
		if etag := strings.Trim(info.ETag, `"`); etag != "" {
			w.Header().Set("ETag", `"`+etag+`"`)
			if etagMatches(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		if !info.LastModified.IsZero() {
			w.Header().Set("Last-Modified", info.LastModified.UTC().Format(http.TimeFormat))
		}
		_, _ = io.Copy(w, rc)
		return
	}

	emote, emoteErr := h.st.GetEmote(r.Context(), id)
	if emoteErr != nil {
		metrics.EmoteImageServed.WithLabelValues("unknown", scale, "missing").Inc()
		http.NotFound(w, r)
		return
	}
	cdnURL := emoteimage.ExtensionBrowserURL(emote.Provider, emote.ID, emote.ProviderEmoteID)
	if cdnURL == "" && emote.SourceURL != "" {
		cdnURL = emote.SourceURL
	}
	if cdnURL != "" {
		metrics.EmoteImageServed.WithLabelValues(emote.Provider, scale, "cdn").Inc()
		if h.render != nil && h.render.ShouldRenderOnUIRequest() {
			h.render.EnqueueAsync(r.Context(), render.Request{
				EmoteID:         emote.ID,
				Provider:        emote.Provider,
				ProviderEmoteID: emote.ProviderEmoteID,
				SourceURL:       emote.SourceURL,
				SourceHash:      emote.SourceHash,
				Reason:          render.ReasonUIRequest,
				Scale:           scale,
			})
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		http.Redirect(w, r, cdnURL, http.StatusFound)
		return
	}

	metrics.EmoteImageServed.WithLabelValues(emote.Provider, scale, "missing").Inc()
	http.NotFound(w, r)
}

func (h *Handler) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if strings.TrimPrefix(auth, "Bearer ") != h.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) uploadEmote(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer f.Close()

	ct := fh.Header.Get("Content-Type")
	if ct != "image/webp" && ct != "image/gif" && ct != "image/png" {
		http.Error(w, "unsupported content type", http.StatusBadRequest)
		return
	}
	if fh.Size > maxUploadBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "read file", http.StatusInternalServerError)
		return
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	e := store.Emote{
		Name:       name,
		MimeType:   ct,
		SourceHash: hash,
		Status:     0,
	}
	emoteID, err := h.st.UpsertEmote(r.Context(), e)
	if err != nil {
		h.log.Error("upsert emote", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	if err := h.obj.PutSrc(r.Context(), emoteID, data, ct); err != nil {
		h.log.Error("store src", "err", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	if h.render != nil {
		if _, err := h.render.Enqueue(r.Context(), render.Request{
			EmoteID:    emoteID,
			Provider:   "custom",
			Reason:     render.ReasonCustomUpload,
			SourceHash: hash,
			Scale:      "1x",
		}); err != nil {
			h.log.Error("insert job", "err", err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
	} else if _, err := h.st.InsertJob(r.Context(), emoteID, render.JobSourceKey(hash, []string{"1x", "2x", "3x", "4x"})); err != nil {
		h.log.Error("insert job", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": emoteID})
}

func (h *Handler) createSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, err := h.st.CreateEmoteSet(r.Context(), req.Name, req.OwnerID)
	if err != nil {
		h.log.Error("create set", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	setID := chi.URLParam(r, "id")
	var req struct {
		EmoteID string  `json:"emote_id"`
		Alias   *string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EmoteID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.st.AddEmoteToSet(r.Context(), setID, req.EmoteID, req.Alias); err != nil {
		h.log.Error("add item", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	emote, err := h.st.GetEmote(r.Context(), req.EmoteID)
	if err == nil {
		channel, cerr := h.getChannelForSet(r, setID)
		if cerr == nil && channel != nil {
			name := emote.Name
			if req.Alias != nil {
				name = *req.Alias
			}
			_ = h.d.AddEmote(r.Context(), channel.Login, name, emote.ID, flags.IsZeroWidth(emote.Flags))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type ensureResponse struct {
	State     string                   `json:"state"`
	Count     int                      `json:"count"`
	Pending   int                      `json:"pending"`
	Total     int                      `json:"total"`
	Percent   int                      `json:"percent"`
	Providers []ensureProviderResponse `json:"providers,omitempty"`
	Benchmark *ensureBenchmark         `json:"benchmark,omitempty"`
}

type ensureProviderResponse struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	Count    int    `json:"count"`
	Pending  int    `json:"pending"`
	Failed   int    `json:"failed"`
	Total    int    `json:"total"`
	Percent  int    `json:"percent"`
	Error    string `json:"error,omitempty"`
}

type ensureBenchmark struct {
	EnsureMs     int64                     `json:"ensureMs"`
	SeedMs       int64                     `json:"seedMs"`
	DictionaryMs int64                     `json:"dictionaryMs"`
	CacheHit     bool                      `json:"cacheHit"`
	Providers    []ensureProviderBenchmark `json:"providers"`
}

type ensureProviderBenchmark struct {
	Provider   string `json:"provider"`
	State      string `json:"state"`
	Count      int    `json:"count"`
	Pending    int    `json:"pending"`
	Failed     int    `json:"failed"`
	Total      int    `json:"total"`
	Percent    int    `json:"percent"`
	DurationMs int64  `json:"durationMs"`
}

func (h *Handler) ensureChannelEmotes(w http.ResponseWriter, r *http.Request) {
	login := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "login")))
	if login == "" {
		http.Error(w, "missing login", http.StatusBadRequest)
		return
	}
	var req struct {
		TwitchID  string   `json:"twitch_id"`
		Providers []string `json:"providers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.TwitchID = strings.TrimSpace(req.TwitchID)
	if req.TwitchID == "" {
		http.Error(w, "missing twitch_id", http.StatusBadRequest)
		return
	}
	providers, err := normalizeProviders(req.Providers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ensure := h.ensure
	if ensure == nil {
		ensure = h.ensureEmotes
	}
	started := time.Now()
	resp, status, err := ensure(r.Context(), login, req.TwitchID, providers)
	if err != nil {
		if h.log != nil {
			h.log.Error("ensure channel emotes", "login", login, "twitch_id", req.TwitchID, "err", err)
		}
		http.Error(w, "seed error: "+err.Error(), status)
		return
	}
	if resp.Benchmark == nil {
		resp.Benchmark = benchmarkFromResponse(resp, false)
	}
	resp.Benchmark.EnsureMs = time.Since(started).Milliseconds()
	writeJSON(w, status, resp)
}

func (h *Handler) ensureEmotes(ctx context.Context, login, twitchID string, providers []seeder.Provider) (ensureResponse, int, error) {
	started := time.Now()
	ready, pending, channelKnown, err := h.st.GetChannelEmoteSummary(ctx, login)
	if err != nil {
		return ensureResponse{}, http.StatusInternalServerError, err
	}
	providerSummary, err := h.st.GetChannelProviderEmoteSummary(ctx, login)
	if err != nil {
		return ensureResponse{}, http.StatusInternalServerError, err
	}
	providerLoads, err := h.st.GetChannelProviderLoads(ctx, login)
	if err != nil {
		return ensureResponse{}, http.StatusInternalServerError, err
	}
	providersToSeed := providersNeedingSeed(providerSummary, providerLoads, providers)
	activeSetLoaded, err := h.activeSetIncludesProviders(ctx, login, providers)
	if err != nil {
		return ensureResponse{}, http.StatusInternalServerError, err
	}
	if !activeSetLoaded {
		setExists, err := h.st.EmoteSetExistsByOwnerName(ctx, providerSetName(login, providers), twitchID)
		if err != nil {
			return ensureResponse{}, http.StatusInternalServerError, err
		}
		if !setExists {
			providersToSeed = providers
		}
	}
	if h.isSeeding(login) {
		resp := makeEnsureResponse("processing", ready, maxInt(pending, 1), providers)
		applyProviderSummary(&resp, providerSummary, providerLoads)
		resp.Benchmark = benchmarkFromResponse(resp, false)
		resp.Benchmark.EnsureMs = time.Since(started).Milliseconds()
		return resp, http.StatusAccepted, nil
	}
	if pending > 0 {
		resp := makeEnsureResponse("processing", ready, pending, providers)
		applyProviderSummary(&resp, providerSummary, providerLoads)
		resp.Benchmark = benchmarkFromResponse(resp, false)
		resp.Benchmark.EnsureMs = time.Since(started).Milliseconds()
		return resp, http.StatusAccepted, nil
	}
	refreshProviders, err := h.providersNeedingRefresh(ctx, login, twitchID, providers)
	if err != nil {
		if h.log != nil {
			h.log.Warn("provider refresh check failed", "login", login, "twitch_id", twitchID, "err", err)
		}
	} else {
		providersToSeed = mergeProviders(providersToSeed, refreshProviders)
	}
	if channelKnown && activeSetLoaded && len(providersToSeed) == 0 {
		if h.seed != nil && providerListIncludes(providers, seeder.ProviderSevenTV) {
			if _, err := h.seed.SyncSevenTVEmoteFlags(ctx, twitchID); err != nil && h.log != nil {
				h.log.Warn("sync 7tv zero-width flags", "login", login, "twitch_id", twitchID, "err", err)
			}
			h.registerSevenTVSubscription(ctx, login, twitchID)
		}
		dictStarted := time.Now()
		if err := h.rebuildChannelDictionary(ctx, login); err != nil {
			return ensureResponse{}, http.StatusInternalServerError, err
		}
		resp := makeEnsureResponse("ready", ready, pending, providers)
		if hasPartialProvider(resp.Providers) {
			resp.State = "processing"
		}
		applyProviderSummary(&resp, providerSummary, providerLoads)
		resp.Benchmark = benchmarkFromResponse(resp, true)
		resp.Benchmark.DictionaryMs = time.Since(dictStarted).Milliseconds()
		resp.Benchmark.EnsureMs = time.Since(started).Milliseconds()
		return resp, http.StatusOK, nil
	}
	seedStarted := time.Now()
	if h.startSeed(login, twitchID, providers, providersToSeed) {
		pending = 1
	}
	resp := makeEnsureResponse("processing", ready, maxInt(pending, 1), providers)
	applyProviderSummary(&resp, providerSummary, providerLoads)
	resp.Benchmark = benchmarkFromResponse(resp, false)
	resp.Benchmark.SeedMs = time.Since(seedStarted).Milliseconds()
	resp.Benchmark.EnsureMs = time.Since(started).Milliseconds()
	return resp, http.StatusAccepted, nil
}

func (h *Handler) repairProviderMetadata(ctx context.Context) {
	if h.st == nil {
		return
	}
	n, err := h.st.RepairProviderMetadataReady(ctx)
	if err != nil && h.log != nil {
		h.log.Warn("repair provider metadata ready", "err", err)
		return
	}
	if n > 0 && h.log != nil {
		h.log.Info("repaired provider metadata ready", "count", n)
	}
}

func providerListIncludes(providers []seeder.Provider, want seeder.Provider) bool {
	for _, provider := range providers {
		if provider == want {
			return true
		}
	}
	return false
}

func (h *Handler) providersNeedingRefresh(ctx context.Context, login, twitchID string, providers []seeder.Provider) ([]seeder.Provider, error) {
	if h.seed == nil || h.st == nil {
		return nil, nil
	}
	stale := make([]seeder.Provider, 0, len(providers))
	for _, provider := range providers {
		switch provider {
		case seeder.ProviderSevenTV:
			local, ok, err := h.st.GetChannelProviderSetSnapshot(ctx, login, string(provider))
			if err != nil {
				return nil, err
			}
			remote, err := h.seed.SevenTVSnapshot(ctx, twitchID)
			if err != nil {
				return nil, err
			}
			if providerSnapshotNeedsRefresh(ok, local.ProviderSetID, local.EmoteHash, local.Count, remote.SetID, remote.EmoteHash, remote.Count) {
				stale = append(stale, provider)
			}
		}
	}
	return stale, nil
}

func providerSnapshotNeedsRefresh(localFound bool, localSetID, localHash string, localCount int, remoteSetID, remoteHash string, remoteCount int) bool {
	return emotesync.ProviderSnapshotNeedsRefresh(localFound, localSetID, localHash, remoteSetID, remoteHash, localCount, remoteCount)
}

func hasPartialProvider(providers []ensureProviderResponse) bool {
	for _, provider := range providers {
		if provider.State == "partial" || provider.State == "processing" {
			return true
		}
	}
	return false
}

func (h *Handler) registerSevenTVSubscription(ctx context.Context, login, twitchID string) {
	if h.eventSub == nil || h.st == nil {
		return
	}
	setID, ok, err := h.st.GetChannelSevenTVProviderSetID(ctx, login)
	if err != nil || !ok {
		return
	}
	if err := h.eventSub.Register(ctx, login, twitchID, setID); err != nil && h.log != nil {
		h.log.Warn("7tv event subscription failed", "login", login, "set_id", setID, "err", err)
	}
}

func mergeProviders(base []seeder.Provider, extra []seeder.Provider) []seeder.Provider {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[seeder.Provider]struct{}, len(base)+len(extra))
	out := make([]seeder.Provider, 0, len(base)+len(extra))
	for _, provider := range base {
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	for _, provider := range extra {
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	return out
}

func normalizeProviders(raw []string) ([]seeder.Provider, error) {
	if len(raw) == 0 {
		return []seeder.Provider{seeder.ProviderSevenTV, seeder.ProviderTwitch, seeder.ProviderFFZ, seeder.ProviderBTTV}, nil
	}
	seen := make(map[seeder.Provider]struct{}, len(raw))
	providers := make([]seeder.Provider, 0, len(raw))
	for _, item := range raw {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "7tv", "seventv":
			if _, ok := seen[seeder.ProviderSevenTV]; !ok {
				seen[seeder.ProviderSevenTV] = struct{}{}
				providers = append(providers, seeder.ProviderSevenTV)
			}
		case "twitch", "official", "official_twitch":
			if _, ok := seen[seeder.ProviderTwitch]; !ok {
				seen[seeder.ProviderTwitch] = struct{}{}
				providers = append(providers, seeder.ProviderTwitch)
			}
		case "ffz", "frankerfacez":
			if _, ok := seen[seeder.ProviderFFZ]; !ok {
				seen[seeder.ProviderFFZ] = struct{}{}
				providers = append(providers, seeder.ProviderFFZ)
			}
		case "bttv", "betterttv":
			if _, ok := seen[seeder.ProviderBTTV]; !ok {
				seen[seeder.ProviderBTTV] = struct{}{}
				providers = append(providers, seeder.ProviderBTTV)
			}
		case "":
		default:
			return nil, fmt.Errorf("unsupported emote provider %q", item)
		}
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("missing emote providers")
	}
	return providers, nil
}

func makeEnsureResponse(state string, ready, pending int, providers []seeder.Provider) ensureResponse {
	total := ready + pending
	resp := ensureResponse{State: state, Count: ready, Pending: pending, Total: total, Percent: percentLoaded(ready, total)}
	for _, provider := range providers {
		resp.Providers = append(resp.Providers, ensureProviderResponse{
			Provider: string(provider),
			State:    state,
		})
	}
	return resp
}

func applyProviderSummary(resp *ensureResponse, summary map[string]store.ProviderEmoteSummary, loads map[string]store.ChannelProviderLoad) {
	if resp == nil || len(resp.Providers) == 0 {
		return
	}
	for i := range resp.Providers {
		item, ok := summary[resp.Providers[i].Provider]
		load, loadOK := loads[resp.Providers[i].Provider]
		if !ok {
			if loadOK {
				target := load.Count
				resp.Providers[i].Count = 0
				resp.Providers[i].Pending = target
				resp.Providers[i].Failed = 0
				resp.Providers[i].Total = target
				resp.Providers[i].Percent = percentLoaded(0, target)
				resp.Providers[i].State = providerProgressState(store.ProviderEmoteSummary{}, load, loadOK, load.State)
				resp.Providers[i].Error = load.Error
			}
			continue
		}
		total := providerProgressTotal(item, load, loadOK)
		state := providerProgressState(item, load, loadOK, resp.Providers[i].State)
		resp.Providers[i].State = state
		resp.Providers[i].Count = item.Ready
		resp.Providers[i].Pending = item.Pending
		resp.Providers[i].Failed = item.Failed
		resp.Providers[i].Total = total
		resp.Providers[i].Percent = percentLoaded(item.Ready, total)
		if loadOK && load.State == "failed" {
			resp.Providers[i].State = "failed"
			resp.Providers[i].Error = load.Error
		}
	}
	if hasPartialProvider(resp.Providers) {
		resp.State = "processing"
	}
}

func providerProgressTotal(item store.ProviderEmoteSummary, load store.ChannelProviderLoad, loadOK bool) int {
	observed := item.Ready + item.Pending + item.Failed
	if loadOK && load.ExpectedCount > observed {
		return load.ExpectedCount
	}
	if loadOK && load.Count > observed {
		return load.Count
	}
	return observed
}

func providerProgressState(item store.ProviderEmoteSummary, load store.ChannelProviderLoad, loadOK bool, fallback string) string {
	if loadOK && load.State == "partial" {
		return "partial"
	}
	if item.Failed > 0 && item.Ready == 0 && item.Pending == 0 {
		return "failed"
	}
	if item.Pending > 0 {
		return "processing"
	}
	total := providerProgressTotal(item, load, loadOK)
	if loadOK && load.ExpectedCount > 0 && item.Ready < load.ExpectedCount {
		return "partial"
	}
	if loadOK && load.Count > 0 && item.Ready < load.Count {
		return "processing"
	}
	if total > 0 && item.Ready >= total {
		return "ready"
	}
	if loadOK && load.State == "processing" {
		return "processing"
	}
	if loadOK && load.State == "failed" {
		return "failed"
	}
	return fallback
}

func benchmarkFromResponse(resp ensureResponse, cacheHit bool) *ensureBenchmark {
	bench := &ensureBenchmark{CacheHit: cacheHit}
	for _, provider := range resp.Providers {
		bench.Providers = append(bench.Providers, ensureProviderBenchmark{
			Provider: provider.Provider,
			State:    provider.State,
			Count:    provider.Count,
			Pending:  provider.Pending,
			Failed:   provider.Failed,
			Total:    provider.Total,
			Percent:  provider.Percent,
		})
	}
	return bench
}

func providersReady(summary map[string]store.ProviderEmoteSummary, providers []seeder.Provider) bool {
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		item, ok := summary[string(provider)]
		if !ok || item.Ready == 0 || item.Pending > 0 {
			return false
		}
	}
	return true
}

func providerMetadataLoaded(loads map[string]store.ChannelProviderLoad, providers []seeder.Provider) bool {
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		load, ok := loads[string(provider)]
		if !ok || load.State != "ready" {
			return false
		}
	}
	return true
}

func providersNeedingSeed(summary map[string]store.ProviderEmoteSummary, loads map[string]store.ChannelProviderLoad, providers []seeder.Provider) []seeder.Provider {
	missing := make([]seeder.Provider, 0, len(providers))
	for _, provider := range providers {
		key := string(provider)
		if load, ok := loads[key]; ok && load.State == "ready" && load.Count == 0 {
			continue
		}
		if item, ok := summary[key]; ok && item.Ready > 0 && item.Pending == 0 {
			continue
		}
		missing = append(missing, provider)
	}
	return missing
}

func providerSetName(login string, providers []seeder.Provider) string {
	parts := make([]string, 0, len(providers))
	for _, provider := range providers {
		parts = append(parts, string(provider))
	}
	return fmt.Sprintf("%s provider emotes (%s)", login, strings.Join(parts, "+"))
}

func (h *Handler) activeSetIncludesProviders(ctx context.Context, login string, providers []seeder.Provider) (bool, error) {
	name, ok, err := h.st.GetChannelActiveSetName(ctx, login)
	if err != nil || !ok {
		return false, err
	}
	name = strings.ToLower(name)
	for _, provider := range providers {
		if !strings.Contains(name, string(provider)) {
			return false, nil
		}
	}
	return true, nil
}

func percentLoaded(ready, total int) int {
	if total <= 0 {
		return 0
	}
	return (ready * 100) / total
}

func isDefaultSevenTV(providers []seeder.Provider) bool {
	return len(providers) == 1 && providers[0] == seeder.ProviderSevenTV
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *Handler) rebuildChannelDictionary(ctx context.Context, login string) error {
	if h.d == nil {
		return nil
	}
	emotes, err := h.st.GetChannelEmotes(ctx, login)
	if err != nil {
		return err
	}
	entries := make([]dict.EmoteEntry, 0, len(emotes))
	for _, e := range emotes {
		entries = append(entries, dict.EmoteEntry{
			Name:            e.Name,
			EmoteID:         e.EmoteID,
			ProviderEmoteID: e.ProviderEmoteID,
			ZeroWidth:       flags.IsZeroWidth(e.Flags),
			Provider:        e.Provider,
		})
	}
	return h.d.Rebuild(ctx, login, entries)
}

func (h *Handler) isSeeding(login string) bool {
	h.seedMu.Lock()
	defer h.seedMu.Unlock()
	_, ok := h.seeding[login]
	return ok
}

func (h *Handler) startSeed(login, twitchID string, setProviders, seedProviders []seeder.Provider) bool {
	if h.seed == nil {
		return false
	}
	if len(seedProviders) == 0 {
		return false
	}
	h.seedMu.Lock()
	if h.seeding == nil {
		h.seeding = make(map[string]struct{})
	}
	if _, ok := h.seeding[login]; ok {
		h.seedMu.Unlock()
		return false
	}
	h.seeding[login] = struct{}{}
	h.seedMu.Unlock()
	if h.log != nil {
		h.log.Info("seed channel emotes queued", "login", login, "twitch_id", twitchID, "providers", setProviders, "seed_providers", seedProviders)
	}

	go func() {
		defer func() {
			h.seedMu.Lock()
			delete(h.seeding, login)
			h.seedMu.Unlock()
		}()
		started := time.Now()
		if h.log != nil {
			h.log.Info("seed channel emotes started", "login", login, "twitch_id", twitchID, "providers", setProviders, "seed_providers", seedProviders)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		results, err := h.seed.SeedChannelProviderSubset(ctx, login, twitchID, setProviders, seedProviders)
		if err != nil && h.log != nil {
			h.log.Error("seed channel emotes", "login", login, "twitch_id", twitchID, "providers", setProviders, "seed_providers", seedProviders, "err", err)
		}
		for _, result := range results {
			if result.State != "failed" {
				h.markProviderLoaded(login, seeder.Provider(result.Provider))
			}
			if result.Provider == string(seeder.ProviderSevenTV) && result.State != "failed" {
				h.registerSevenTVSubscription(ctx, login, twitchID)
			}
		}
		h.repairProviderMetadata(ctx)
		if h.log != nil {
			h.log.Info("seed channel emotes finished", "login", login, "twitch_id", twitchID, "providers", setProviders, "seed_providers", seedProviders, "results", results, "elapsed_ms", time.Since(started).Milliseconds())
		}
	}()
	return true
}

func (h *Handler) markProviderLoaded(login string, provider seeder.Provider) {
	h.loadedMu.Lock()
	defer h.loadedMu.Unlock()
	if h.loadedProviders == nil {
		h.loadedProviders = make(map[string]map[seeder.Provider]struct{})
	}
	if h.loadedProviders[login] == nil {
		h.loadedProviders[login] = make(map[seeder.Provider]struct{})
	}
	h.loadedProviders[login][provider] = struct{}{}
}

func (h *Handler) providersLoaded(login string, providers []seeder.Provider) bool {
	h.loadedMu.Lock()
	defer h.loadedMu.Unlock()
	if len(providers) == 0 || h.loadedProviders == nil || h.loadedProviders[login] == nil {
		return false
	}
	for _, provider := range providers {
		if _, ok := h.loadedProviders[login][provider]; !ok {
			return false
		}
	}
	return true
}

func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) {
	setID := chi.URLParam(r, "id")
	emoteID := chi.URLParam(r, "emote_id")

	emote, err := h.st.GetEmote(r.Context(), emoteID)
	name := emoteID
	if err == nil {
		name = emote.Name
	}

	if err := h.st.RemoveEmoteFromSet(r.Context(), setID, emoteID); err != nil {
		h.log.Error("remove item", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	channel, cerr := h.getChannelForSet(r, setID)
	if cerr == nil && channel != nil {
		_ = h.d.RemoveEmote(r.Context(), channel.Login, name)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setActiveSet(w http.ResponseWriter, r *http.Request) {
	twitchID := chi.URLParam(r, "twitch_id")
	var req struct {
		SetID string `json:"set_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SetID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.st.SetActiveEmoteSet(r.Context(), twitchID, req.SetID); err != nil {
		h.log.Error("set active set", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	channel, err := h.st.GetChannel(r.Context(), twitchID)
	if err == nil {
		emotes, err := h.st.GetChannelEmotes(r.Context(), channel.Login)
		if err == nil {
			var entries []dict.EmoteEntry
			for _, e := range emotes {
				entries = append(entries, dict.EmoteEntry{
					Name:            e.Name,
					EmoteID:         e.EmoteID,
					ProviderEmoteID: e.ProviderEmoteID,
					ZeroWidth:       flags.IsZeroWidth(e.Flags),
					Provider:        e.Provider,
				})
			}
			_ = h.d.Rebuild(r.Context(), channel.Login, entries)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) seedChannel(w http.ResponseWriter, r *http.Request) {
	twitchID := chi.URLParam(r, "twitch_id")
	if err := h.seed.SeedChannel(r.Context(), twitchID); err != nil {
		h.log.Error("seed channel", "twitch_id", twitchID, "err", err)
		http.Error(w, "seed error: "+err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listChannelEmotes(w http.ResponseWriter, r *http.Request) {
	login := chi.URLParam(r, "login")
	emotes, err := h.st.GetChannelEmotes(r.Context(), login)
	if err != nil {
		h.log.Error("list emotes", "login", login, "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type item struct {
		Name            string `json:"name"`
		EmoteID         string `json:"emote_id"`
		ProviderEmoteID string `json:"provider_emote_id,omitempty"`
		URL             string `json:"url"`
		ZW              bool   `json:"zw"`
		Provider        string `json:"provider"`
	}
	result := make([]item, 0, len(emotes))
	for _, e := range emotes {
		result = append(result, item{
			Name:            e.Name,
			EmoteID:         e.EmoteID,
			ProviderEmoteID: e.ProviderEmoteID,
			URL:             h.d.BrowserURL(e.EmoteID, e.Provider, e.ProviderEmoteID, "1x"),
			ZW:              flags.IsZeroWidth(e.Flags),
			Provider:        e.Provider,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getChannelForSet(r *http.Request, setID string) (*store.Channel, error) {
	rows, err := h.st.GetChannelByActiveSet(r.Context(), setID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
