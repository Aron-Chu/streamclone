package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	publicStatsCacheKey  = "sp:public:stats"
	publicStatusCacheKey = "sp:public:status"
	publicCacheTTL       = 60 * time.Second
)

type PublicStatsResponse struct {
	StreamsTracked        int64     `json:"streamsTracked"`
	MomentsDetected       int64     `json:"momentsDetected"`
	ChatMessagesProcessed int64     `json:"chatMessagesProcessed"`
	EmotesIndexed         int64     `json:"emotesIndexed"`
	VodsAnalyzed          int64     `json:"vodsAnalyzed"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type PublicStatusResponse struct {
	Status    string    `json:"status"`
	API       string    `json:"api"`
	Degraded  bool      `json:"degraded"`
	Incident  *string   `json:"incident"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (h *Handler) PublicRoutes(r chi.Router) {
	r.Route("/v1/public", func(r chi.Router) {
		r.Get("/stats", h.getPublicStats)
		r.Get("/status", h.getPublicStatus)
		r.Get("/hub", h.getPublicHub)
		r.Get("/emotes/overview", h.getPublicEmotesOverview)
	})
}

func (h *Handler) StartPublicCacheRefresh(ctx context.Context) {
	if h == nil || h.store == nil {
		return
	}
	h.refreshOnce.Do(func() {
		h.refreshStop = make(chan struct{})
		go h.publicCacheRefreshLoop(ctx)
	})
}

func (h *Handler) publicCacheRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(publicCacheTTL)
	defer ticker.Stop()
	h.refreshPublicCaches(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.refreshStop:
			return
		case <-ticker.C:
			h.refreshPublicCaches(ctx)
		}
	}
}

func (h *Handler) refreshPublicCaches(ctx context.Context) {
	_, _, _ = h.loadPublicStats(ctx, true)
	_, _, _ = h.loadPublicStatus(ctx, true)
	_, _, _ = h.loadPublicHub(ctx, true, publicHubOptions{})
	_, _, _ = h.loadPublicEmotesOverview(ctx, true, parsePublicEmotesRange(""))
}

func (h *Handler) getPublicStats(w http.ResponseWriter, r *http.Request) {
	payload, fromCache, err := h.loadPublicStats(r.Context(), false)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "stats_unavailable"})
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) getPublicStatus(w http.ResponseWriter, r *http.Request) {
	payload, fromCache, err := h.loadPublicStatus(r.Context(), false)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "status_unavailable"})
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) loadPublicStats(ctx context.Context, forceRefresh bool) (PublicStatsResponse, bool, error) {
	if !forceRefresh && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, publicStatsCacheKey).Bytes(); err == nil && len(cached) > 0 {
			var payload PublicStatsResponse
			if json.Unmarshal(cached, &payload) == nil {
				return payload, true, nil
			}
		}
	}
	v, err, _ := h.statsGroup.Do("stats", func() (any, error) {
		if !forceRefresh && h.rdb != nil {
			if cached, err := h.rdb.Get(ctx, publicStatsCacheKey).Bytes(); err == nil && len(cached) > 0 {
				var payload PublicStatsResponse
				if json.Unmarshal(cached, &payload) == nil {
					return payload, nil
				}
			}
		}
		stats, err := h.store.PublicAggregateStats(ctx)
		if err != nil {
			return PublicStatsResponse{}, err
		}
		payload := PublicStatsResponse{
			StreamsTracked:        stats.StreamsTracked,
			MomentsDetected:       stats.MomentsDetected,
			ChatMessagesProcessed: stats.ChatMessagesProcessed,
			EmotesIndexed:         stats.EmotesIndexed,
			VodsAnalyzed:          stats.VodsAnalyzed,
			UpdatedAt:             time.Now().UTC(),
		}
		if h.rdb != nil {
			body, _ := json.Marshal(payload)
			_ = h.rdb.Set(ctx, publicStatsCacheKey, body, publicCacheTTL).Err()
		}
		return payload, nil
	})
	if err != nil {
		return PublicStatsResponse{}, false, err
	}
	return v.(PublicStatsResponse), false, nil
}

func (h *Handler) loadPublicStatus(ctx context.Context, forceRefresh bool) (PublicStatusResponse, bool, error) {
	if !forceRefresh && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, publicStatusCacheKey).Bytes(); err == nil && len(cached) > 0 {
			var payload PublicStatusResponse
			if json.Unmarshal(cached, &payload) == nil {
				return payload, true, nil
			}
		}
	}
	v, err, _ := h.statusGroup.Do("status", func() (any, error) {
		if !forceRefresh && h.rdb != nil {
			if cached, err := h.rdb.Get(ctx, publicStatusCacheKey).Bytes(); err == nil && len(cached) > 0 {
				var payload PublicStatusResponse
				if json.Unmarshal(cached, &payload) == nil {
					return payload, nil
				}
			}
		}
		degraded := false
		status := "operational"
		api := "up"
		if h.store != nil {
			if err := h.store.Ping(ctx); err != nil {
				degraded = true
				status = "degraded"
				api = "degraded"
			}
		}
		payload := PublicStatusResponse{
			Status:    status,
			API:       api,
			Degraded:  degraded,
			Incident:  nil,
			UpdatedAt: time.Now().UTC(),
		}
		if h.rdb != nil {
			body, _ := json.Marshal(payload)
			_ = h.rdb.Set(ctx, publicStatusCacheKey, body, publicCacheTTL).Err()
		}
		return payload, nil
	})
	if err != nil {
		return PublicStatusResponse{}, false, err
	}
	return v.(PublicStatusResponse), false, nil
}

type PublicAggregateStats struct {
	StreamsTracked        int64
	MomentsDetected       int64
	ChatMessagesProcessed int64
	EmotesIndexed         int64
	VodsAnalyzed          int64
}

func (s *Store) PublicAggregateStats(ctx context.Context) (PublicAggregateStats, error) {
	var out PublicAggregateStats
	if s == nil || s.db == nil {
		return out, nil
	}
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM analytics_streams),
			(SELECT COUNT(*)::bigint FROM pulse_bookmarks),
			(SELECT COALESCE(SUM(chat_count), 0)::bigint FROM analytics_minute_rollups),
			(SELECT COUNT(*)::bigint FROM emotes),
			(SELECT COUNT(*)::bigint FROM analytics_streams WHERE COALESCE(vod_id, '') <> '')`,
	).Scan(
		&out.StreamsTracked,
		&out.MomentsDetected,
		&out.ChatMessagesProcessed,
		&out.EmotesIndexed,
		&out.VodsAnalyzed,
	)
	return out, err
}
