package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/analytics/heatmap"
)

// PortalPeak is a portal-safe peak moment used by StreamPulse analytics UI.
type PortalPeak struct {
	OffsetSeconds  int              `json:"offsetSeconds"`
	Score          int              `json:"score"`
	Reasons        []string         `json:"reasons,omitempty"`
	ReasonLabel    string           `json:"reasonLabel"`
	DominantSignal string           `json:"dominantSignal"`
	ChatCount      int              `json:"chatCount"`
	EmoteCount     int              `json:"emoteCount"`
	ViewerDelta    string           `json:"viewerDelta,omitempty"`
	Confidence     int              `json:"confidence,omitempty"`
	VodState       string           `json:"vodState,omitempty"`
	TopEmotes      []ExtensionEmote `json:"topEmotes,omitempty"`
}

type PortalStreamPeaksResponse struct {
	StreamID  string       `json:"streamId"`
	Login     string       `json:"login"`
	Peaks     []PortalPeak `json:"peaks"`
	UpdatedAt int64        `json:"updatedAt"`
}

type PortalCoverageTruthResponse struct {
	StreamID        string                   `json:"streamId"`
	Login           string                   `json:"login"`
	Coverage        ExtensionCoverage        `json:"coverage"`
	CoverageTruth   []HubFeaturedCoverageRow `json:"coverageTruth"`
	DataCoveragePct float64                  `json:"dataCoveragePct,omitempty"`
	VodID           string                   `json:"vodId,omitempty"`
	UpdatedAt       int64                    `json:"updatedAt"`
}

type portalSessionFigmaBundle struct {
	stream          *StreamRecord
	rollups         []heatmap.MinuteRollup
	points          []heatmap.ReplayHeatmapDetailPoint
	startedAt       time.Time
	isLive          bool
	vodID           string
	currentOffset   int
	peaks           []PortalPeak
	dataCoveragePct float64
}

func (h *Handler) loadPortalSessionFigmaBundle(ctx context.Context, stream *StreamRecord) (*portalSessionFigmaBundle, error) {
	if stream == nil {
		return nil, errors.New("missing_stream")
	}
	heatmapRollups, startedAt, err := h.consolidateForHeatmap(ctx, stream.StreamID)
	if err != nil {
		return nil, err
	}
	cfg, cfgErr := heatmap.LoadScoringConfig()
	if cfgErr != nil {
		cfg = heatmap.DefaultScoringConfig()
	}
	points := heatmap.AlignedDetailPoints(heatmapRollups, cfg)
	isLive := stream.EndedAt == nil
	streamStart := stream.StartedAt
	if streamStart.IsZero() && !startedAt.IsZero() {
		streamStart = startedAt
	}
	currentOffset := 0
	if isLive && !streamStart.IsZero() {
		currentOffset = int(math.Max(0, time.Since(streamStart).Seconds()))
	} else if !isLive && stream.EndedAt != nil && !streamStart.IsZero() {
		currentOffset = int(stream.EndedAt.Sub(streamStart).Seconds())
	}
	vodID := strings.TrimSpace(stream.VodID)
	extPeaks := buildExtensionPeaks(heatmapRollups, points, isLive, vodID, streamStart)
	peaks := h.decoratePortalPeaks(ctx, portalPeaksFromExtension(extPeaks, heatmapRollups, points, vodID))
	metrics := summarizeStreamMetrics(stream, filterTimelineRollups(storeRollupsFromHeatmap(heatmapRollups)))
	return &portalSessionFigmaBundle{
		stream:          stream,
		rollups:         heatmapRollups,
		points:          points,
		startedAt:       streamStart,
		isLive:          isLive,
		vodID:           vodID,
		currentOffset:   currentOffset,
		peaks:           peaks,
		dataCoveragePct: metrics.DataCoveragePct,
	}, nil
}

func portalPeaksFromExtension(
	ext []ExtensionPeak,
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	vodID string,
) []PortalPeak {
	out := make([]PortalPeak, 0, len(ext))
	for _, peak := range ext {
		confidence := 0
		if pt, ok := detailPointAtOffset(points, peak.OffsetSeconds); ok {
			confidence = int(math.Round(pt.Confidence * 100))
		}
		out = append(out, PortalPeak{
			OffsetSeconds:  peak.OffsetSeconds,
			Score:          peak.Score,
			Reasons:        peak.Reasons,
			ReasonLabel:    peak.ReasonLabel,
			DominantSignal: peak.DominantSignal,
			ChatCount:      peak.ChatCount,
			EmoteCount:     peak.EmoteCount,
			ViewerDelta:    viewerDeltaAtOffset(rollups, points, peak.OffsetSeconds),
			Confidence:     confidence,
			VodState:       portalVodState(vodID, peak.OffsetSeconds, vodID != ""),
			TopEmotes:      peak.TopEmotes,
		})
	}
	return out
}

func detailPointAtOffset(points []heatmap.ReplayHeatmapDetailPoint, offset int) (heatmap.ReplayHeatmapDetailPoint, bool) {
	for _, pt := range points {
		if pt.OffsetSeconds == offset {
			return pt, true
		}
	}
	return heatmap.ReplayHeatmapDetailPoint{}, false
}

func viewerDeltaAtOffset(rollups []heatmap.MinuteRollup, points []heatmap.ReplayHeatmapDetailPoint, offset int) string {
	idx := -1
	for i, pt := range points {
		if pt.OffsetSeconds == offset {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(rollups) {
		return ""
	}
	current := rollups[idx].ViewerLatest
	prev := 0
	if idx > 0 {
		prev = rollups[idx-1].ViewerLatest
	}
	if prev <= 0 {
		return ""
	}
	delta := current - prev
	if delta == 0 {
		return "0"
	}
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return fmt.Sprintf("%d", delta)
}

func portalVodState(vodID string, offsetSeconds int, hasVod bool) string {
	if !hasVod {
		return "no_vod"
	}
	if offsetSeconds <= 0 {
		return "vod_ready"
	}
	return "vod_ready"
}

func (h *Handler) portalStreamPeaks(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	cacheKey := portalAnalyticsCachePrefix + "peaks:" + streamID
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
	bundle, err := h.loadPortalSessionFigmaBundle(r.Context(), stream)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	resp := PortalStreamPeaksResponse{
		StreamID:  stream.StreamID,
		Login:     stream.Login,
		Peaks:     bundle.peaks,
		UpdatedAt: time.Now().UnixMilli(),
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

func (h *Handler) portalStreamCoverageTruth(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	cacheKey := portalAnalyticsCachePrefix + "coverage-truth:" + streamID
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
	bundle, err := h.loadPortalSessionFigmaBundle(r.Context(), stream)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	coverage := computePulseCoverage(
		bundle.rollups,
		bundle.startedAt,
		bundle.currentOffset,
		bundle.isLive,
		bundle.vodID,
		false,
		false,
	)
	seventvPerMin := 0.0
	if metrics := summarizeStreamMetrics(stream, filterTimelineRollups(storeRollupsFromHeatmap(bundle.rollups))); metrics.SevenTVPerMin > 0 {
		seventvPerMin = metrics.SevenTVPerMin
	}
	resp := PortalCoverageTruthResponse{
		StreamID:        stream.StreamID,
		Login:           stream.Login,
		Coverage:        coverage,
		CoverageTruth:   hubFeaturedCoverageRows(coverage, bundle.vodID, bundle.dataCoveragePct, seventvPerMin),
		DataCoveragePct: bundle.dataCoveragePct,
		VodID:           bundle.vodID,
		UpdatedAt:       time.Now().UnixMilli(),
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
