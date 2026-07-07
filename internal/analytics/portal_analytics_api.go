package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	portalAnalyticsCachePrefix   = "sp:portal:analytics:"
	portalAnalyticsSummaryTTL    = 45 * time.Second
	portalAnalyticsGamesLiveTTL  = 90 * time.Second
	portalAnalyticsGamesEndedTTL = 10 * time.Minute
	portalAnalyticsRLPrefix      = "sp:rl:portal:summary:"
	portalAnalyticsRLPerMin      = 60
)

// PortalStreamDetail is a user-safe stream shell without minute rollups or raw chat.
type PortalStreamDetail struct {
	Channel          string                    `json:"channel"`
	State            string                    `json:"state"`
	Stream           *PortalStreamRecord       `json:"stream,omitempty"`
	Sources          []SourceStatus            `json:"sources"`
	UpdatedAt        int64                     `json:"updatedAt"`
	VodID            string                    `json:"vodId,omitempty"`
	SyncPhase        string                    `json:"syncPhase,omitempty"`
	ChatCoveragePct  float64                   `json:"chatCoveragePct,omitempty"`
	ChatSourceMeta   *StreamChatSourceMetadata `json:"chatSource,omitempty"`
	AnalyticsQuality string                    `json:"analyticsQuality,omitempty"`
	ViewerSource     string                    `json:"viewerSource,omitempty"`
	DataSourceBadges []PortalDataSourceBadge   `json:"dataSourceBadges,omitempty"`
	StoredArtifacts  *StoredArtifactsSummary   `json:"storedArtifacts,omitempty"`
}

type PortalStreamRecord struct {
	StreamID       string     `json:"streamId"`
	Login          string     `json:"login"`
	DisplayName    string     `json:"displayName,omitempty"`
	Title          string     `json:"title,omitempty"`
	Category       string     `json:"category,omitempty"`
	GamesSummary   string     `json:"gamesSummary,omitempty"`
	StartedAt      time.Time  `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
	CurrentViewers int        `json:"currentViewers,omitempty"`
	PeakViewers    int        `json:"peakViewers,omitempty"`
	ViewerSamples  int        `json:"viewerSamples,omitempty"`
	ChatMessages   int64      `json:"chatMessages,omitempty"`
	VodID          string     `json:"vodId,omitempty"`
}

type PortalDataSourceBadge struct {
	Source string `json:"source"`
	State  string `json:"state"`
	Label  string `json:"label,omitempty"`
}

type PortalSyncStatus struct {
	Phase     string    `json:"phase"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	Stale     bool      `json:"stale,omitempty"`
}

// PortalMinuteTopEmote is a bounded per-minute emote preview (no raw emote maps or ids).
type PortalMinuteTopEmote struct {
	Name     string `json:"name"`
	Provider string `json:"provider,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Count    int    `json:"count"`
}

// PortalMinutePoint is a sanitized per-minute series point (no emote maps or raw chat).
type PortalMinutePoint struct {
	OffsetSeconds     int                    `json:"offsetSeconds"`
	ViewerAvg         int                    `json:"viewerAvg,omitempty"`
	ViewerMax         int                    `json:"viewerMax,omitempty"`
	ViewerLatest      int                    `json:"viewerLatest,omitempty"`
	ChatCount         int                    `json:"chatCount,omitempty"`
	TotalEmoteCount   int                    `json:"totalEmoteCount,omitempty"`
	SevenTVEmoteCount int                    `json:"seventvEmoteCount,omitempty"`
	TopEmotes         []PortalMinuteTopEmote `json:"topEmotes,omitempty"`
	Missing           bool                   `json:"missing,omitempty"`
}

// PortalStreamMinutesResponse exposes portal-safe minute rollups for chart rails.
type PortalStreamMinutesResponse struct {
	StreamID                   string              `json:"streamId"`
	Channel                    string              `json:"channel"`
	StartedAt                  time.Time           `json:"startedAt"`
	CoverageStartOffsetSeconds int                 `json:"coverageStartOffsetSeconds,omitempty"`
	Minutes                    []PortalMinutePoint `json:"minutes"`
	ProvisionalPeakMinutes     []PortalMinutePoint `json:"provisionalPeakMinutes,omitempty"`
	UpdatedAt                  int64               `json:"updatedAt"`
}

func portalStreamRecordFrom(rec *StreamRecord) *PortalStreamRecord {
	if rec == nil {
		return nil
	}
	return &PortalStreamRecord{
		StreamID:       rec.StreamID,
		Login:          rec.Login,
		DisplayName:    rec.DisplayName,
		Title:          rec.Title,
		Category:       rec.Category,
		StartedAt:      rec.StartedAt,
		EndedAt:        rec.EndedAt,
		CurrentViewers: rec.CurrentViewers,
		PeakViewers:    rec.PeakViewers,
		ViewerSamples:  rec.ViewerSamples,
		ChatMessages:   rec.ChatMessages,
		VodID:          rec.VodID,
	}
}

func (h *Handler) portalLiveStreamRecord(ctx context.Context, stream *StreamRecord) *PortalStreamRecord {
	rec := portalStreamRecordFrom(stream)
	if rec == nil || h == nil {
		return rec
	}
	if segments, err := h.resolveStreamGameSegments(ctx, stream.StreamID); err == nil {
		rec.GamesSummary = gamesSummaryFromSegments(segments, stream.Category)
		if rec.Category == "" {
			rec.Category = dominantCategoryFromSegments(segments, stream.Category)
		}
	}
	return rec
}

func portalQualityFromMetrics(m StreamSummaryMetrics) string {
	switch m.SyncHealthState {
	case "ready", "ok", "":
		if m.DataCoveragePct >= 80 {
			return "full_pulse"
		}
		if m.MinutesWithData > 0 {
			return "partial_pulse"
		}
		return "warming"
	case "syncing":
		return "syncing"
	default:
		return "limited"
	}
}

func portalBadgeLabel(source string) string {
	parts := strings.Split(strings.ReplaceAll(source, "_", " "), " ")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, " ")
}

func portalBadgesFromSources(sources []SourceStatus) []PortalDataSourceBadge {
	out := make([]PortalDataSourceBadge, 0, len(sources))
	for _, s := range sources {
		label := portalBadgeLabel(s.Source)
		out = append(out, PortalDataSourceBadge{Source: s.Source, State: s.State, Label: label})
	}
	return out
}

func (h *Handler) PortalRoutes(r chi.Router) {
	r.Route("/v1/portal/analytics", func(r chi.Router) {
		if h.pulseHosted.Hosted {
			r.Use(h.pulseHostedAuthMiddleware)
		}
		r.Get("/streams/{streamID}", h.portalStreamDetail)
		r.Get("/streams/{streamID}/summary", h.portalStreamSummary)
		r.Get("/streams/{streamID}/sync/status", h.portalStreamSyncStatus)
		r.Get("/streams/{streamID}/replay-heatmap", h.portalReplayHeatmap)
		r.Get("/streams/{streamID}/games", h.portalStreamGames)
		r.Get("/streams/{streamID}/recap", h.portalStreamRecap)
		r.Get("/streams/{streamID}/peaks", h.portalStreamPeaks)
		r.Get("/streams/{streamID}/coverage-truth", h.portalStreamCoverageTruth)
		r.Get("/streams/{streamID}/minutes", h.portalStreamMinutes)
		r.Get("/channels/{login}/emotes", h.portalChannelEmotes)
		r.Get("/channels/{login}/streams", h.portalChannelStreams)
		r.Get("/channels/{login}/live", h.portalChannelLive)
	})
}

func portalMinuteOffsetSeconds(streamStart time.Time, rollup MinuteRollup) int {
	if streamStart.IsZero() || rollup.MinuteTS.IsZero() {
		return 0
	}
	sec := rollup.MinuteTS.Sub(streamStart).Seconds()
	if sec < 0 {
		return 0
	}
	return int(sec)
}

func portalMinutesCacheKey(streamID string, includeProvisionalPeaks bool) string {
	key := portalAnalyticsCachePrefix + "minutes:" + streamID
	if includeProvisionalPeaks {
		return key + ":provisional_peaks"
	}
	return key
}

func portalMinutesFromRollups(stream *StreamRecord, rollups []MinuteRollup, includeProvisionalPeaks bool) PortalStreamMinutesResponse {
	timeline := filterTimelineRollups(rollups)
	consolidated := consolidateRollupsByMinute(timeline)
	sorted := make([]MinuteRollup, 0, len(consolidated))
	for _, rollup := range consolidated {
		sorted = append(sorted, rollup)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MinuteTS.Before(sorted[j].MinuteTS)
	})
	points := portalMinutePointsFromRollups(stream, sorted)
	resp := PortalStreamMinutesResponse{
		StreamID:  stream.StreamID,
		Channel:   stream.Login,
		StartedAt: stream.StartedAt,
		Minutes:   points,
		UpdatedAt: time.Now().UnixMilli(),
	}
	if coverageStart := portalCoverageStartOffset(stream, timeline); coverageStart >= 0 {
		resp.CoverageStartOffsetSeconds = coverageStart
	}
	if includeProvisionalPeaks {
		resp.ProvisionalPeakMinutes = portalMinutePointsFromRollups(stream, provisionalPeakCandidateRollups(rollups))
	}
	return resp
}

func portalMinutePointsFromRollups(stream *StreamRecord, rollups []MinuteRollup) []PortalMinutePoint {
	points := make([]PortalMinutePoint, 0, len(rollups))
	for _, rollup := range rollups {
		offset := portalMinuteOffsetSeconds(stream.StartedAt, rollup)
		viewer := rollup.ViewerLatest
		if viewer == 0 {
			viewer = rollup.ViewerMax
		}
		if viewer == 0 {
			viewer = rollup.ViewerAvg
		}
		points = append(points, PortalMinutePoint{
			OffsetSeconds:     offset,
			ViewerAvg:         rollup.ViewerAvg,
			ViewerMax:         rollup.ViewerMax,
			ViewerLatest:      viewer,
			ChatCount:         rollup.ChatCount,
			TotalEmoteCount:   hubRollupEmoteCount(rollup),
			SevenTVEmoteCount: rollup.SevenTVEmoteCount,
			Missing:           rollup.Missing,
		})
	}
	return points
}

func portalCoverageStartOffset(stream *StreamRecord, rollups []MinuteRollup) int {
	coverageStart := -1
	for _, rollup := range rollups {
		offset := portalMinuteOffsetSeconds(stream.StartedAt, rollup)
		hasData := !rollup.Missing && (rollup.ChatCount > 0 || rollup.TotalEmoteCount > 0 || rollup.ViewerSamples > 0)
		if hasData && coverageStart < 0 {
			coverageStart = offset
		}
	}
	return coverageStart
}

func (h *Handler) portalStreamDetail(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rollups, _ := h.store.RollupsByStream(r.Context(), stream.StreamID)
	metrics := summarizeStreamMetrics(stream, filterTimelineRollups(rollups))
	state := "historical"
	if stream.EndedAt == nil {
		state = "live"
	}
	syncPhase := ""
	if h.syncService != nil {
		if syncStatus, syncErr := h.syncService.GetSyncStatus(r.Context(), stream.StreamID); syncErr == nil && syncStatus != nil && !syncStatus.Phase.IsTerminal() && !syncStatus.Stale {
			state = "syncing"
			syncPhase = string(syncStatus.Phase)
		}
	}
	stored := h.storedArtifactsForStream(r.Context(), stream.StreamID)
	sources := mergeStoredSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}, stored)
	var chatSourceMeta *StreamChatSourceMetadata
	badges := portalBadgesFromStored(stored, sources)
	if meta, err := h.store.GetStreamChatSourceMetadata(r.Context(), stream.StreamID); err == nil && meta != nil && meta.ChatSource != ChatSourceNone {
		chatSourceMeta = meta
		label, status := portalChatSourceLabel(*meta)
		if label != "" {
			badges = append(badges, PortalDataSourceBadge{Source: meta.ChatSource, State: meta.SourceConfidence, Label: label + " — " + status})
		}
	}
	writeJSON(w, http.StatusOK, PortalStreamDetail{
		Channel:          stream.Login,
		State:            state,
		SyncPhase:        syncPhase,
		Stream:           portalStreamRecordFrom(stream),
		Sources:          sources,
		UpdatedAt:        time.Now().UnixMilli(),
		VodID:            strings.TrimSpace(stream.VodID),
		ChatCoveragePct:  metrics.DataCoveragePct,
		ChatSourceMeta:   chatSourceMeta,
		AnalyticsQuality: portalQualityFromMetrics(metrics),
		ViewerSource:     persistedViewerSource(stream, filterTimelineRollups(rollups)),
		DataSourceBadges: badges,
		StoredArtifacts:  &stored,
	})
}

func (h *Handler) portalStreamSummary(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	if h.pulseHosted.Hosted && h.rateLimiter != nil {
		if p, ok := pulsePrincipalFromContext(r.Context()); ok {
			if allowed, retry := h.rateLimiter.AllowPortalSummary(r.Context(), p.ID); !allowed {
				writePortalRateLimited(w, retry)
				return
			}
		}
	}
	cacheKey := portalAnalyticsCachePrefix + "summary:" + streamID
	if body, ok := h.portalCacheGet(r.Context(), cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	metrics := summarizeStreamMetrics(stream, rollups)
	topEmotes := TopEmotesFromRollups(filterTimelineRollups(rollups), 15)
	topEmotes = h.rewriteHostedTopEmotes(r.Context(), topEmotes)
	stored := h.storedArtifactsForStream(r.Context(), stream.StreamID)
	sources := mergeStoredSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}, stored)
	resp := map[string]any{
		"channel":          stream.Login,
		"stream":           portalStreamRecordFrom(stream),
		"metrics":          metrics,
		"topEmotes":        topEmotes,
		"sources":          sources,
		"updatedAt":        time.Now().UnixMilli(),
		"analyticsQuality": portalQualityFromMetrics(metrics),
		"dataSourceBadges": portalBadgesFromStored(stored, sources),
		"storedArtifacts":  stored,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode_failed"})
		return
	}
	h.portalCacheSet(r.Context(), cacheKey, body, portalAnalyticsSummaryTTL)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) portalStreamSyncStatus(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.syncService == nil {
		writeJSON(w, http.StatusOK, PortalSyncStatus{Phase: "idle"})
		return
	}
	status, err := h.syncService.GetSyncStatus(r.Context(), streamID)
	if err != nil {
		writeJSON(w, http.StatusOK, PortalSyncStatus{Phase: "idle"})
		return
	}
	writeJSON(w, http.StatusOK, PortalSyncStatus{
		Phase:     string(status.Phase),
		Message:   status.Message,
		UpdatedAt: status.UpdatedAt,
		Stale:     status.Stale,
	})
}

func (h *Handler) portalReplayHeatmap(w http.ResponseWriter, r *http.Request) {
	h.replayHeatmap(w, r)
}

func (h *Handler) portalStreamGames(w http.ResponseWriter, r *http.Request) {
	h.getStreamGames(w, r)
}

func (h *Handler) portalStreamRecap(w http.ResponseWriter, r *http.Request) {
	h.getPulseStreamRecap(w, r)
}

func (h *Handler) portalStreamMinutes(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeHostedPortalStreamAccess(w, r) {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	includePeaks := r.URL.Query().Get("includeProvisionalPeaks") == "true"
	cacheKey := portalMinutesCacheKey(streamID, includePeaks)
	if body, ok := h.portalCacheGet(r.Context(), cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	resp := portalMinutesFromRollups(stream, rollups, includePeaks)
	timeline := filterTimelineRollups(rollups)
	h.enrichPortalMinuteTopEmotes(r.Context(), stream, resp.Minutes, timeline)
	if includePeaks {
		h.enrichPortalMinuteTopEmotes(r.Context(), stream, resp.ProvisionalPeakMinutes, provisionalPeakCandidateRollups(rollups))
	}
	body, err := json.Marshal(resp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode_failed"})
		return
	}
	h.portalCacheSet(r.Context(), cacheKey, body, portalAnalyticsSummaryTTL)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) authorizeHostedPortalStreamAccess(w http.ResponseWriter, r *http.Request) bool {
	// Portal analytics routes return sanitized aggregate rollups only (no raw chat).
	// Public /analytics pages are intentionally no-login on hosted production.
	_ = w
	_ = r
	return true
}

func (h *Handler) portalChannelEmotes(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	login := normalizeLogin(chi.URLParam(r, "login"))
	if login == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_login"})
		return
	}
	rangeDuration := parsePortalEmoteRange(r.URL.Query().Get("range"))
	cacheKey := portalAnalyticsCachePrefix + "channel-emotes:" + login + ":" + formatEmoteRange(rangeDuration)
	if body, ok := h.portalCacheGet(r.Context(), cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	resp, err := h.store.PortalChannelEmotes(r.Context(), login, rangeDuration)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "channel_emotes_unavailable"})
		return
	}
	resp.TopEmotes = h.decoratePortalChannelEmotes(r.Context(), resp.TopEmotes)
	body, err := json.Marshal(resp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode_failed"})
		return
	}
	h.portalCacheSet(r.Context(), cacheKey, body, portalAnalyticsSummaryTTL)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) portalCacheGet(ctx context.Context, key string) ([]byte, bool) {
	if h == nil || h.rdb == nil {
		return nil, false
	}
	val, err := h.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return val, true
}

func (h *Handler) portalCacheSet(ctx context.Context, key string, body []byte, ttl time.Duration) {
	if h == nil || h.rdb == nil || len(body) == 0 {
		return
	}
	_ = h.rdb.Set(ctx, key, body, ttl).Err()
}

type PortalChannelStreamsResponse struct {
	Channel   string               `json:"channel"`
	Items     []PortalStreamRecord `json:"items"`
	Sources   []SourceStatus       `json:"sources"`
	UpdatedAt int64                `json:"updatedAt"`
}

func (h *Handler) portalChannelStreams(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeHostedPortalStreamAccess(w, r) {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	items := make([]PortalStreamRecord, 0, len(streams))
	for i := range streams {
		rec := portalStreamRecordFrom(&streams[i])
		if segments, segErr := h.resolveStreamGameSegments(r.Context(), streams[i].StreamID); segErr == nil {
			rec.GamesSummary = gamesSummaryFromSegments(segments, streams[i].Category)
			if rec.Category == "" {
				rec.Category = dominantCategoryFromSegments(segments, streams[i].Category)
			}
		}
		items = append(items, *rec)
	}
	writeJSON(w, http.StatusOK, PortalChannelStreamsResponse{
		Channel:   login,
		Items:     items,
		Sources:   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

type PortalChannelLiveResponse struct {
	Channel                    string              `json:"channel"`
	State                      string              `json:"state"`
	Stream                     *PortalStreamRecord `json:"stream,omitempty"`
	Rollups                    []PortalMinutePoint `json:"rollups"`
	TopEmotes                  []TopEmote          `json:"topEmotes"`
	Sources                    []SourceStatus      `json:"sources"`
	UpdatedAt                  int64               `json:"updatedAt"`
	VodID                      string              `json:"vodId,omitempty"`
	SyncPhase                  string              `json:"syncPhase,omitempty"`
	CoverageStartOffsetSeconds int                 `json:"coverageStartOffsetSeconds,omitempty"`
	ViewerSource               string              `json:"viewerSource,omitempty"`
}

func (h *Handler) portalChannelLive(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeHostedPortalStreamAccess(w, r) {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
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
			writeJSON(w, http.StatusOK, PortalChannelLiveResponse{
				Channel:   login,
				State:     "not_collected",
				Rollups:   []PortalMinutePoint{},
				TopEmotes: []TopEmote{},
				Sources:   []SourceStatus{{Source: "analytics_db", State: "unavailable", Message: "No recent data"}},
				UpdatedAt: time.Now().UnixMilli(),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	timeline := filterTimelineRollups(rollups)
	points := portalMinutePointsFromRollups(stream, timeline)
	topEmotes := TopEmotesFromRollups(timeline, 15)
	topEmotes = h.rewriteHostedTopEmotes(r.Context(), topEmotes)
	state := "historical"
	if stream.EndedAt == nil {
		state = "live"
	}
	syncPhase := ""
	if h.syncService != nil {
		if syncStatus, syncErr := h.syncService.GetSyncStatus(r.Context(), stream.StreamID); syncErr == nil && syncStatus != nil && !syncStatus.Phase.IsTerminal() && !syncStatus.Stale {
			state = "syncing"
			syncPhase = string(syncStatus.Phase)
		}
	}
	coverageStart := portalCoverageStartOffset(stream, timeline)
	liveResp := PortalChannelLiveResponse{
		Channel:      login,
		State:        state,
		Stream:       h.portalLiveStreamRecord(r.Context(), stream),
		Rollups:      points,
		TopEmotes:    topEmotes,
		Sources:      []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt:    time.Now().UnixMilli(),
		VodID:        strings.TrimSpace(stream.VodID),
		SyncPhase:    syncPhase,
		ViewerSource: persistedViewerSource(stream, timeline),
	}
	if coverageStart >= 0 {
		liveResp.CoverageStartOffsetSeconds = coverageStart
	}
	writeJSON(w, http.StatusOK, liveResp)
}

func writePortalRateLimited(w http.ResponseWriter, retry time.Duration) {
	sec := int(retry.Seconds())
	if sec < 1 {
		sec = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(sec))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited", "scope": "portal_analytics"})
}
