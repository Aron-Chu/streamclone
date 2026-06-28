package analytics

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/timeseries"
)

var loginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,24}$`)

type Handler struct {
	store            *Store
	collector        *Collector
	helix            *HelixClient
	syncService      *SyncService
	pulseBackfill    *PulseBackfillManager
	heatmapCache     *heatmap.Cache
	timeseries       timeseries.Writer
	rdb              *redis.Client
	rateLimiter      *PulseRateLimiter
	pulseHosted      PulseHostedConfig
	pulseRuntime     PulseRuntimeConfig
	corpusRuntime    CorpusRuntimeConfig
	emoteHistoryJobs EmoteHistoryJobConfig
	cdnPublicBase    string
	statsGroup       singleflight.Group
	statusGroup      singleflight.Group
	hubGroup         singleflight.Group
	storyboardCache  *vodStoryboardCache
	refreshStop      chan struct{}
	refreshOnce      sync.Once
}

func NewHandler(store *Store, collector *Collector, helix *HelixClient, syncService *SyncService) *Handler {
	return &Handler{store: store, collector: collector, helix: helix, syncService: syncService}
}

func (h *Handler) WithHeatmapCache(cache *heatmap.Cache) *Handler {
	h.heatmapCache = cache
	return h
}

func (h *Handler) WithTimeseries(writer timeseries.Writer) *Handler {
	h.timeseries = writer
	return h
}

func (h *Handler) WithPulseRuntime(cfg PulseRuntimeConfig) *Handler {
	h.pulseRuntime = cfg.withDefaults()
	return h
}

func (h *Handler) pulseRuntimeConfig() PulseRuntimeConfig {
	if h == nil {
		return DefaultPulseRuntimeConfig()
	}
	return h.pulseRuntime.withDefaults()
}

func (h *Handler) WithCorpusRuntime(cfg CorpusRuntimeConfig) *Handler {
	h.corpusRuntime = cfg.withDefaults()
	return h
}

func (h *Handler) WithEmoteHistoryJobs(cfg EmoteHistoryJobConfig) *Handler {
	h.emoteHistoryJobs = cfg
	return h
}

func (h *Handler) corpusRuntimeConfig() CorpusRuntimeConfig {
	if h == nil {
		return DefaultCorpusRuntimeConfig()
	}
	return h.corpusRuntime.withDefaults()
}

func (h *Handler) Routes(r chi.Router) {
	h.PublicRoutes(r)
	h.ExtensionRoutes(r)
	h.PulseRoutes(r)
	h.PortalRoutes(r)
	h.EmoteHistoryRoutes(r)
	h.CorpusRoutes(r)
	h.PublicEmoteMaterializationRoutes(r)
	r.Route("/v1/analytics", func(r chi.Router) {
		r.Get("/always-tracked", h.getAlwaysTracked)
		r.Post("/always-tracked", h.setAlwaysTracked)
		if h.pulseHosted.Hosted {
			r.With(h.pulseHostedAuthMiddleware).Post("/channels/{login}/watch", h.watchChannel)
		} else {
			r.Post("/channels/{login}/watch", h.watchChannel)
		}
		r.Get("/channels/{login}/streams", h.channelStreams)
		r.Get("/channels/{login}/streams/ranked", h.channelStreamsRanked)
		if h.pulseHosted.Hosted {
			r.Group(func(r chi.Router) {
				r.Use(h.pulseHostedAuthMiddleware)
				r.Use(h.pulseHostedStreamTimelineAuthMiddleware)
				r.Get("/channels/{login}/live", h.channelLive)
				r.Get("/streams/{streamID}", h.streamDetail)
				r.Get("/streams/{streamID}/replay-heatmap", h.replayHeatmap)
			})
		} else {
			r.Get("/channels/{login}/live", h.channelLive)
			r.Get("/streams/{streamID}", h.streamDetail)
			r.Get("/streams/{streamID}/replay-heatmap", h.replayHeatmap)
		}
		r.Get("/streams/{streamID}/summary", h.streamSummary)
		r.Post("/streams/{streamID}/prefetch-tracker", h.prefetchTracker)
		r.Post("/streams/{streamID}/sync", h.syncStream)
		r.Get("/streams/{streamID}/sync/status", h.syncStreamStatus)
		r.Get("/sync/active", h.listActiveSyncs)
		r.Get("/tracking/snapshot", h.trackingSnapshot)
		r.Get("/top100/readiness", h.top100Readiness)
		r.Get("/top-roster/readiness", h.top100Readiness)
		r.Get("/streams/{streamID}/games", h.getStreamGames)
		r.Delete("/streams/{streamID}/replay-heatmap/cache", h.invalidateHeatmapCache)
		r.Get("/vods/{vodId}/storyboard-thumb", h.vodStoryboardThumb)
		r.Get("/timeseries/status", h.timeseriesStatus)
	})
}

func (h *Handler) getAlwaysTracked(w http.ResponseWriter, r *http.Request) {
	logins := h.collector.GetAlwaysTracked()
	writeJSON(w, http.StatusOK, map[string][]string{"channels": logins})
}

func (h *Handler) setAlwaysTracked(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	var req struct {
		Channel string `json:"channel"`
		Track   bool   `json:"track"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	login, ok := validLogin(req.Channel)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	globalLimit := h.pulseRuntimeConfig().ProtectedGlobalLimit
	if req.Track {
		if err := h.store.AddAlwaysTrackedWithCap(r.Context(), login, globalLimit); err != nil {
			if isProtectedCapError(err) {
				writeProtectedCapReached(w)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		resp := h.collector.SetAlwaysTracked(r.Context(), login, true, true)
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp := h.collector.SetAlwaysTracked(r.Context(), login, false)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) watchChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	if !h.enforceWatchRateLimit(w, r) {
		return
	}
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	principalID := ""
	if p, ok := pulsePrincipalFromContext(r.Context()); ok {
		principalID = p.ID
	}
	resp := h.collector.WatchForPrincipal(r.Context(), login, principalID)
	if resp.Tracking {
		h.invalidateExtensionCoverageCache(r.Context(), login)
	}
	status := http.StatusOK
	if !resp.Tracking {
		status = http.StatusAccepted
	}
	writeJSON(w, status, resp)
}

func (h *Handler) channelLive(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeHostedStreamTimelineAccess(w, r) {
		return
	}
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	stream, err := h.store.LatestStreamByLogin(r.Context(), login)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, StreamDetailResponse{
				Channel:   login,
				State:     "not_collected",
				Rollups:   []MinuteRollup{},
				TopEmotes: []TopEmote{},
				Sources:   []SourceStatus{{Source: "analytics_db", State: "unavailable", Message: "No recent data"}},
				UpdatedAt: time.Now().UnixMilli(),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.writeStreamDetail(w, r, stream, http.StatusOK)
}

func (h *Handler) channelStreams(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	streams, err := h.store.StreamsByLogin(r.Context(), login, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, StreamsResponse{
		Channel:   login,
		Items:     streams,
		Sources:   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) channelStreamsRanked(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	sortKey := normalizeRankedSort(r.URL.Query().Get("sort"))
	period := normalizeRankedPeriod(r.URL.Query().Get("period"))
	streams, err := h.store.StreamsByLogin(r.Context(), login, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	streams = filterStreamsByPeriod(streams, period, time.Now().UTC())
	sortStreamsByRank(streams, sortKey)
	if len(streams) > limit {
		streams = streams[:limit]
	}
	writeJSON(w, http.StatusOK, RankedStreamsResponse{
		Channel:   login,
		Sort:      sortKey,
		Period:    period,
		Items:     streams,
		Sources:   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) streamDetail(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.writeMissingStreamDetail(w, r, streamID)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.writeStreamDetail(w, r, stream, http.StatusOK)
}

func (h *Handler) streamSummary(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if channel, ok := validLogin(r.URL.Query().Get("channel")); ok && stream.Login != "" && channel != normalizeLogin(stream.Login) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_channel_mismatch"})
		return
	}
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	timelineRollups := filterTimelineRollups(rollups)
	metrics := summarizeStreamMetrics(stream, timelineRollups)
	stored := h.storedArtifactsForStream(r.Context(), stream.StreamID)
	sources := mergeStoredSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}, stored)
	writeJSON(w, http.StatusOK, StreamSummaryResponse{
		Channel:         stream.Login,
		Stream:          stream,
		Metrics:         metrics,
		TopEmotes:       TopEmotesFromRollups(timelineRollups, 25),
		Sources:         sources,
		UpdatedAt:       time.Now().UnixMilli(),
		StoredArtifacts: ptrStoredArtifacts(stored),
	})
}

func (h *Handler) timeseriesStatus(w http.ResponseWriter, r *http.Request) {
	if h.timeseries == nil {
		writeJSON(w, http.StatusOK, timeseries.Status{Enabled: false, Configured: false, Backend: timeseries.DefaultBackend, State: "disabled"})
		return
	}
	writeJSON(w, http.StatusOK, h.timeseries.Status())
}

func (h *Handler) writeMissingStreamDetail(w http.ResponseWriter, r *http.Request, streamID string) {
	if h.syncService != nil {
		if status, statusErr := h.syncService.GetSyncStatus(r.Context(), streamID); statusErr == nil && status != nil && !status.Phase.IsTerminal() && !status.Stale {
			writeJSON(w, http.StatusOK, StreamDetailResponse{
				Channel:   "",
				State:     "syncing",
				SyncPhase: string(status.Phase),
				Rollups:   []MinuteRollup{},
				TopEmotes: []TopEmote{},
				Sources:   []SourceStatus{{Source: "analytics_db", State: "syncing", Message: status.Message}},
				UpdatedAt: time.Now().UnixMilli(),
			})
			return
		}
	}
	channel := ""
	if login, ok := validLogin(r.URL.Query().Get("channel")); ok {
		channel = login
		upsertErr := h.store.UpsertStreamPlaceholder(r.Context(), streamID, "", login, "", time.Time{})
		if upsertErr == nil {
			if stream, err := h.store.StreamByID(r.Context(), streamID); err == nil {
				h.writeStreamDetail(w, r, stream, http.StatusOK)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, StreamDetailResponse{
		Channel:   channel,
		State:     "not_collected",
		Rollups:   []MinuteRollup{},
		TopEmotes: []TopEmote{},
		Sources:   []SourceStatus{{Source: "analytics_db", State: "unavailable", Message: "Stream not synced yet"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) writeStreamDetail(w http.ResponseWriter, r *http.Request, stream *StreamRecord, status int) {
	if !h.authorizeHostedStreamTimelineAccess(w, r) {
		return
	}
	sparse := r == nil || r.URL.Query().Get("sparse") != "false"
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rollups = filterTimelineRollups(rollups)
	for i := range rollups {
		rollups[i] = normalizeRollup(rollups[i], 200)
	}
	topEmotes := TopEmotesFromRollups(rollups, 50)
	emoteKeys := make([]string, 0, len(topEmotes))
	for _, emote := range topEmotes {
		emoteKeys = append(emoteKeys, emote.Key)
	}
	state := "historical"
	if stream.EndedAt == nil {
		state = "live"
	}
	vodID := strings.TrimSpace(stream.VodID)
	vodSource := strings.TrimSpace(stream.VodSource)
	broadcasterID := NormalizeBroadcasterID(stream.BroadcasterID)
	if broadcasterID == "" && h.helix != nil && h.helix.Enabled() && stream.Login != "" {
		broadcasterID = h.helix.ResolveBroadcasterID(r.Context(), stream.Login, "")
	}
	if vodID == "" && h.helix != nil && h.helix.Enabled() && broadcasterID != "" {
		if resolved, _ := h.helix.VideoIDByStreamID(r.Context(), broadcasterID, stream.StreamID); resolved != "" {
			vodID = resolved
			vodSource = "helix_stream_match"
			_ = h.store.SetStreamVodID(r.Context(), stream.StreamID, vodID, "helix_stream_match")
		}
	}
	var responseRollups []MinuteRollup
	if sparse {
		responseRollups = slimRollupsForChart(rollups, emoteKeys)
	} else {
		startAt, endAt := normalizeStreamWindow(stream.StartedAt, stream.EndedAt)
		responseRollups = fillMissingRollups(rollups, startAt, endAt)
	}
	vodDurationSec := 0
	if vodID != "" && h.helix != nil && h.helix.Enabled() {
		if d, err := h.helix.VideoDurationSeconds(r.Context(), vodID); err == nil {
			vodDurationSec = d
		}
	}
	chatCoverage := chatCoverageSummary(rollups, stream, vodDurationSec)

	responseState := state
	var syncPhase string
	if h.syncService != nil {
		if syncStatus, syncErr := h.syncService.GetSyncStatus(r.Context(), stream.StreamID); syncErr == nil && syncStatus != nil && !syncStatus.Phase.IsTerminal() && !syncStatus.Stale {
			responseState = "syncing"
			syncPhase = string(syncStatus.Phase)
		}
	}

	stored := h.storedArtifactsForStream(r.Context(), stream.StreamID)
	sources := mergeStoredSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}, stored)

	var chatSourceMeta *StreamChatSourceMetadata
	if meta, err := h.store.GetStreamChatSourceMetadata(r.Context(), stream.StreamID); err == nil && meta != nil && meta.ChatSource != ChatSourceNone {
		chatSourceMeta = meta
		label, status := portalChatSourceLabel(*meta)
		if label != "" {
			sources = append(sources, SourceStatus{Source: meta.ChatSource, State: meta.SourceConfidence, Message: label + " — " + status})
		}
	}

	writeJSON(w, status, StreamDetailResponse{
		Channel:         stream.Login,
		State:           responseState,
		SyncPhase:       syncPhase,
		Stream:          stream,
		Rollups:         responseRollups,
		TopEmotes:       topEmotes,
		Sources:         sources,
		UpdatedAt:       time.Now().UnixMilli(),
		VodID:           vodID,
		VodSource:       vodSource,
		ChatCoveragePct: chatCoverage.CoveragePct,
		VodDurationSec:  vodDurationSec,
		ChatCoverage:    &chatCoverage,
		ChatSourceMeta:  chatSourceMeta,
		ViewerSource:    persistedViewerSource(stream, rollups),
		StoredArtifacts: ptrStoredArtifacts(stored),
	})
}

func summarizeStreamMetrics(stream *StreamRecord, rollups []MinuteRollup) StreamSummaryMetrics {
	if stream == nil {
		return StreamSummaryMetrics{SyncHealthState: "missing"}
	}
	var chat, emotes, seventv, viewerSamples int
	minutesWithData := 0
	firstViewer := 0
	lastViewer := 0
	for _, rollup := range rollups {
		if rollup.Missing {
			continue
		}
		if rollup.ChatCount > 0 || rollup.TotalEmoteCount > 0 || rollup.ViewerSamples > 0 {
			minutesWithData++
		}
		chat += rollup.ChatCount
		emotes += rollup.TotalEmoteCount
		seventv += rollup.SevenTVEmoteCount
		viewerSamples += rollup.ViewerSamples
		viewer := rollup.ViewerLatest
		if viewer == 0 {
			viewer = rollup.ViewerMax
		}
		if viewer == 0 {
			viewer = rollup.ViewerAvg
		}
		if viewer > 0 {
			if firstViewer == 0 {
				firstViewer = viewer
			}
			lastViewer = viewer
		}
	}
	minutes := math.Max(1, float64(minutesWithData))
	providerShare := 0.0
	if emotes > 0 {
		providerShare = clampPct(float64(seventv) / float64(emotes) * 100)
	}
	reactionScore := 0.0
	if chat+emotes > 0 {
		reactionScore = clampPct(float64(emotes+seventv) / float64(chat+emotes) * 100)
	}
	viewerMomentum := 0.0
	if firstViewer > 0 {
		viewerMomentum = float64(lastViewer-firstViewer) / float64(firstViewer) * 100
	}
	coverage := 0.0
	if !stream.StartedAt.IsZero() {
		end := stream.LastSeenAt
		if stream.EndedAt != nil {
			end = *stream.EndedAt
		}
		expected := math.Max(1, math.Ceil(end.Sub(stream.StartedAt).Minutes()))
		coverage = clampPct(float64(minutesWithData) / expected * 100)
	}
	state := "stats_only"
	if viewerSamples > 0 && chat > 0 {
		state = "synced"
	} else if viewerSamples > 0 {
		state = "viewer_only"
	} else if chat > 0 {
		state = "chat_only"
	}
	if coverage > 0 && coverage < 80 {
		state = "partial"
	}
	return StreamSummaryMetrics{
		ChatPerMin:        round2(float64(chat) / minutes),
		EmotesPerMin:      round2(float64(emotes) / minutes),
		SevenTVPerMin:     round2(float64(seventv) / minutes),
		ProviderSharePct:  round2(providerShare),
		ReactionScore:     round2(reactionScore),
		ViewerMomentum5M:  round2(viewerMomentum),
		DataCoveragePct:   round2(coverage),
		SyncHealthState:   state,
		MinutesWithData:   minutesWithData,
		ViewerSampleCount: viewerSamples,
	}
}

func normalizeRankedSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "peak", "peak_viewers":
		return "peak_viewers"
	case "avg", "avg_viewers":
		return "avg_viewers"
	case "chat", "chat_per_min":
		return "chat"
	case "emotes", "emotes_per_min":
		return "emotes"
	case "seventv", "seventv_share":
		return "seventv_share"
	case "synced":
		return "synced"
	default:
		return "recent"
	}
}

func normalizeRankedPeriod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "24h", "7d", "30d", "all":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "30d"
	}
}

func filterStreamsByPeriod(streams []StreamRecord, period string, now time.Time) []StreamRecord {
	if period == "all" {
		return streams
	}
	dur := 30 * 24 * time.Hour
	switch period {
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	}
	cutoff := now.Add(-dur)
	out := streams[:0]
	for _, stream := range streams {
		if stream.StartedAt.IsZero() || stream.StartedAt.After(cutoff) {
			out = append(out, stream)
		}
	}
	return out
}

func sortStreamsByRank(streams []StreamRecord, sortKey string) {
	sort.SliceStable(streams, func(i, j int) bool {
		a, b := streams[i], streams[j]
		switch sortKey {
		case "peak_viewers":
			if a.PeakViewers != b.PeakViewers {
				return a.PeakViewers > b.PeakViewers
			}
		case "avg_viewers":
			if a.AvgViewers != b.AvgViewers {
				return a.AvgViewers > b.AvgViewers
			}
		case "chat":
			if a.ChatMessages != b.ChatMessages {
				return a.ChatMessages > b.ChatMessages
			}
		case "emotes":
			if a.TotalEmoteUses != b.TotalEmoteUses {
				return a.TotalEmoteUses > b.TotalEmoteUses
			}
		case "seventv_share":
			aShare := ratio(a.SevenTVEmoteUses, a.TotalEmoteUses)
			bShare := ratio(b.SevenTVEmoteUses, b.TotalEmoteUses)
			if aShare != bShare {
				return aShare > bShare
			}
		case "synced":
			aSynced := a.ViewerSamples + int(a.ChatMessages)
			bSynced := b.ViewerSamples + int(b.ChatMessages)
			if aSynced != bSynced {
				return aSynced > bSynced
			}
		}
		return a.LastSeenAt.After(b.LastSeenAt)
	})
}

func ratio(num, denom int64) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func clampPct(value float64) float64 {
	return math.Max(0, math.Min(100, value))
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func validLogin(value string) (string, bool) {
	login := normalizeLogin(value)
	return login, loginRe.MatchString(login)
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func minuteBucketKey(t time.Time) string {
	return t.UTC().Truncate(time.Minute).Format("2006-01-02T15:04:05Z07:00")
}

func mergeMinuteRollups(prev, item MinuteRollup) MinuteRollup {
	switch {
	case item.ViewerSamples > prev.ViewerSamples:
		prev.ViewerAvg = item.ViewerAvg
		prev.ViewerMax = item.ViewerMax
		prev.ViewerLatest = item.ViewerLatest
		prev.ViewerSamples = item.ViewerSamples
	case item.ViewerSamples == prev.ViewerSamples && item.ViewerSamples > 0:
		if item.ViewerAvg > prev.ViewerAvg {
			prev.ViewerAvg = item.ViewerAvg
		}
		if item.ViewerMax > prev.ViewerMax {
			prev.ViewerMax = item.ViewerMax
		}
		if item.ViewerLatest > prev.ViewerLatest {
			prev.ViewerLatest = item.ViewerLatest
		}
	case prev.ViewerSamples == 0 && item.ViewerSamples > 0:
		if item.ViewerAvg > 0 {
			prev.ViewerAvg = item.ViewerAvg
		}
		if item.ViewerMax > prev.ViewerMax {
			prev.ViewerMax = item.ViewerMax
		}
		if item.ViewerLatest > 0 {
			prev.ViewerLatest = item.ViewerLatest
		}
		prev.ViewerSamples = item.ViewerSamples
	}
	if item.ChatCount > prev.ChatCount {
		prev.ChatCount = item.ChatCount
	}
	if item.TotalEmoteCount > prev.TotalEmoteCount {
		prev.TotalEmoteCount = item.TotalEmoteCount
	}
	if item.SevenTVEmoteCount > prev.SevenTVEmoteCount {
		prev.SevenTVEmoteCount = item.SevenTVEmoteCount
	}
	if prev.Emotes == nil {
		prev.Emotes = map[string]int{}
	}
	for key, count := range item.Emotes {
		if count > prev.Emotes[key] {
			prev.Emotes[key] = count
		}
	}
	if !item.Missing {
		prev.Missing = false
	}
	return prev
}

func consolidateRollupsByMinute(in []MinuteRollup) map[string]MinuteRollup {
	out := make(map[string]MinuteRollup, len(in))
	for _, item := range in {
		bucket := item.MinuteTS.UTC().Truncate(time.Minute)
		key := minuteBucketKey(bucket)
		item.MinuteTS = bucket
		if item.Emotes == nil {
			item.Emotes = map[string]int{}
		}
		prev, ok := out[key]
		if !ok {
			out[key] = item
			continue
		}
		out[key] = mergeMinuteRollups(prev, item)
	}
	return out
}

func normalizeStreamWindow(startedAt time.Time, endedAt *time.Time) (time.Time, *time.Time) {
	if endedAt == nil || startedAt.IsZero() {
		return startedAt, endedAt
	}
	if !endedAt.Before(startedAt) {
		return startedAt, endedAt
	}
	clamped := startedAt.UTC().Truncate(time.Minute).Add(time.Minute)
	return startedAt, &clamped
}

func fillMissingRollups(in []MinuteRollup, startedAt time.Time, endedAt *time.Time) []MinuteRollup {
	startedAt, endedAt = normalizeStreamWindow(startedAt, endedAt)
	if startedAt.IsZero() {
		if len(in) < 2 {
			return in
		}
		out := make([]MinuteRollup, 0, len(in))
		for i, item := range in {
			if i > 0 {
				prev := in[i-1].MinuteTS
				if item.MinuteTS.Sub(prev) > 24*time.Hour {
					prev = item.MinuteTS.Add(-24 * time.Hour)
				}
				for ts := prev.Add(time.Minute); ts.Before(item.MinuteTS); ts = ts.Add(time.Minute) {
					out = append(out, MinuteRollup{MinuteTS: ts, Emotes: map[string]int{}, Missing: true})
				}
			}
			out = append(out, item)
		}
		return out
	}

	startMin := startedAt.UTC().Truncate(time.Minute)

	var endMin time.Time
	if endedAt != nil {
		endMin = endedAt.UTC().Truncate(time.Minute)
	} else {
		endMin = time.Now().UTC().Truncate(time.Minute)
	}

	// Safety: prevent padding spans greater than 24 hours to avoid infinite loops / OOM
	if endMin.Sub(startMin) > 24*time.Hour {
		startMin = endMin.Add(-24 * time.Hour)
	}

	minuteSpan := int(endMin.Sub(startMin) / time.Minute)
	if minuteSpan < 0 {
		endMin = startMin
		minuteSpan = 0
	}

	out := make([]MinuteRollup, 0, minuteSpan+1)
	existing := consolidateRollupsByMinute(in)

	for ts := startMin; !ts.After(endMin); ts = ts.Add(time.Minute) {
		if item, ok := existing[minuteBucketKey(ts)]; ok {
			out = append(out, item)
		} else {
			out = append(out, MinuteRollup{
				MinuteTS: ts,
				Emotes:   map[string]int{},
				Missing:  true,
			})
		}
	}

	return out
}

func slimRollupsForChart(in []MinuteRollup, emoteKeys []string) []MinuteRollup {
	if len(in) == 0 {
		return in
	}
	keySet := make(map[string]struct{}, len(emoteKeys))
	for _, key := range emoteKeys {
		if key == "" {
			continue
		}
		keySet[key] = struct{}{}
	}
	out := make([]MinuteRollup, len(in))
	for i, item := range in {
		if len(keySet) == 0 || len(item.Emotes) == 0 {
			item.Emotes = nil
		} else {
			slim := make(map[string]int, len(keySet))
			for key := range keySet {
				if count, ok := item.Emotes[key]; ok && count > 0 {
					slim[key] = count
				}
			}
			if len(slim) == 0 {
				item.Emotes = nil
			} else {
				item.Emotes = slim
			}
		}
		out[i] = item
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) prefetchTracker(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	login, ok := validLogin(r.URL.Query().Get("channel"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if h.syncService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync_unavailable"})
		return
	}
	queued := h.syncService.PrefetchTracker(login, streamID)
	status := "skipped"
	if queued {
		status = "queued"
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": status})
}

func (h *Handler) syncStream(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	channelOpt := normalizeLogin(r.URL.Query().Get("channel"))
	viewersOnly := strings.EqualFold(r.URL.Query().Get("viewers_only"), "true") ||
		strings.EqualFold(r.URL.Query().Get("viewers_only"), "1") ||
		strings.EqualFold(r.URL.Query().Get("mode"), "viewers")
	forceChat := strings.EqualFold(r.URL.Query().Get("force_chat"), "true") ||
		strings.EqualFold(r.URL.Query().Get("force_chat"), "1")
	accepted, status, err := h.syncService.TryStartSync(r.Context(), streamID, channelOpt, viewersOnly, forceChat, strings.TrimSpace(r.URL.Query().Get("vod_id")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	code := http.StatusAccepted
	if !accepted {
		code = http.StatusOK
	}
	writeJSON(w, code, StartSyncResponse{Accepted: accepted, Status: status})
}

func (h *Handler) syncStreamStatus(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	status, err := h.syncService.GetSyncStatus(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusOK, map[string]string{"phase": "idle"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) getStreamGames(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	segments, err := h.store.GetGameSegments(r.Context(), streamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(segments) == 0 {
		if stream, streamErr := h.store.StreamByID(r.Context(), streamID); streamErr == nil {
			segments = fallbackGameSegmentsForStream(stream)
		}
	}
	if segments == nil {
		segments = []GameSegment{}
	}
	writeJSON(w, http.StatusOK, segments)
}
