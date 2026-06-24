package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	portalAnalyticsCachePrefix = "sp:portal:analytics:"
	portalAnalyticsSummaryTTL  = 45 * time.Second
	portalAnalyticsRLPrefix    = "sp:rl:portal:summary:"
	portalAnalyticsRLPerMin    = 60
)

// PortalStreamDetail is a user-safe stream shell without minute rollups or raw chat.
type PortalStreamDetail struct {
	Channel         string               `json:"channel"`
	State           string               `json:"state"`
	Stream          *PortalStreamRecord  `json:"stream,omitempty"`
	Sources         []SourceStatus       `json:"sources"`
	UpdatedAt       int64                `json:"updatedAt"`
	VodID           string               `json:"vodId,omitempty"`
	SyncPhase       string               `json:"syncPhase,omitempty"`
	ChatCoveragePct float64              `json:"chatCoveragePct,omitempty"`
	AnalyticsQuality string              `json:"analyticsQuality,omitempty"`
	DataSourceBadges []PortalDataSourceBadge `json:"dataSourceBadges,omitempty"`
}

type PortalStreamRecord struct {
	StreamID       string     `json:"streamId"`
	Login          string     `json:"login"`
	DisplayName    string     `json:"displayName,omitempty"`
	Title          string     `json:"title,omitempty"`
	Category       string     `json:"category,omitempty"`
	StartedAt      time.Time  `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
	CurrentViewers int        `json:"currentViewers,omitempty"`
	PeakViewers    int        `json:"peakViewers,omitempty"`
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
		VodID:          rec.VodID,
	}
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
	})
}

func (h *Handler) portalStreamDetail(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rollups, _ := h.store.RollupsByStream(r.Context(), stream.StreamID)
	metrics := summarizeStreamMetrics(stream, rollups)
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
	sources := []SourceStatus{{Source: "analytics_db", State: "ready"}}
	writeJSON(w, http.StatusOK, PortalStreamDetail{
		Channel:          stream.Login,
		State:            state,
		SyncPhase:        syncPhase,
		Stream:           portalStreamRecordFrom(stream),
		Sources:          sources,
		UpdatedAt:        time.Now().UnixMilli(),
		VodID:            strings.TrimSpace(stream.VodID),
		ChatCoveragePct:  metrics.DataCoveragePct,
		AnalyticsQuality: portalQualityFromMetrics(metrics),
		DataSourceBadges: portalBadgesFromSources(sources),
	})
}

func (h *Handler) portalStreamSummary(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
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
	resp := map[string]any{
		"channel": stream.Login,
		"stream":  portalStreamRecordFrom(stream),
		"metrics": metrics,
		"topEmotes": TopEmotesFromRollups(rollups, 15),
		"sources":   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		"updatedAt": time.Now().UnixMilli(),
		"analyticsQuality": portalQualityFromMetrics(metrics),
		"dataSourceBadges": portalBadgesFromSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}),
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

func writePortalRateLimited(w http.ResponseWriter, retry time.Duration) {
	sec := int(retry.Seconds())
	if sec < 1 {
		sec = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(sec))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited", "scope": "portal_analytics"})
}
