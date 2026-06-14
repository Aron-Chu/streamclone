package analytics

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/analytics/heatmap"
)

func (h *Handler) invalidateHeatmapCache(w http.ResponseWriter, r *http.Request) {
	streamID := chi.URLParam(r, "streamID")
	if h.heatmapCache != nil {
		h.heatmapCache.Invalidate(r.Context(), streamID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) replayHeatmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}

	window := 60
	if wq := r.URL.Query().Get("window"); wq != "" {
		parsed, err := strconv.Atoi(wq)
		if err != nil || parsed < 10 || parsed > 600 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "window must be an integer between 10 and 600"})
			return
		}
		window = parsed
	}

	detail := false
	if dq := r.URL.Query().Get("detail"); dq != "" {
		detail = strings.EqualFold(dq, "true") || dq == "1"
	}

	channel := r.URL.Query().Get("channel")
	_ = channel

	cfg, err := heatmap.LoadScoringConfig()
	if err != nil {
		cfg = heatmap.DefaultScoringConfig()
		slog.Warn("heatmap: scoring config load failed, using defaults", "error", err)
	}

	updatedAt, err := h.store.GetStreamUpdatedAt(ctx, streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check stream"})
		return
	}

	updatedAtMs := updatedAt.UnixMilli()
	cacheKey := heatmap.CacheKey(streamID, "v1", updatedAtMs, window)

	if h.heatmapCache != nil {
		if cached, ok := h.heatmapCache.Get(ctx, cacheKey); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	rollups, _, err := h.consolidateForHeatmap(ctx, streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load rollups"})
		return
	}

	if len(rollups) == 0 {
		empty := heatmap.HeatmapResponse{
			StreamID:       streamID,
			WindowSeconds:  window,
			Confidence:     0,
			ScoringVersion: cfg.Version,
			UpdatedAt:      updatedAtMs,
			Points:         []heatmap.ReplayHeatmapPoint{},
		}
		writeJSON(w, http.StatusOK, empty)
		return
	}

	var respBytes []byte
	if detail {
		resp := heatmap.ComputeHeatmapDetail(rollups, cfg)
		resp.StreamID = streamID
		resp.WindowSeconds = window
		resp.UpdatedAt = updatedAtMs
		respBytes, _ = json.Marshal(resp)
	} else {
		resp := heatmap.ComputeHeatmap(rollups, cfg)
		resp.StreamID = streamID
		resp.WindowSeconds = window
		resp.UpdatedAt = updatedAtMs
		respBytes, _ = json.Marshal(resp)
	}

	if h.heatmapCache != nil {
		h.heatmapCache.Set(ctx, cacheKey, respBytes, heatmap.DefaultCacheTTL)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}
