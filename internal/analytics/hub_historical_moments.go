package analytics

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	publicHubMomentsCachePrefix = "sp:public:hub:moments"
	publicHubMomentsCacheTTL    = 2 * time.Minute
	publicHubMomentsEmptyTTL    = 20 * time.Second
	hubHistoricalMomentsCap     = 10
	hubHistoricalCandidateCap   = 40
)

type hubHistoricalMinuteCandidate struct {
	StreamID          string
	Login             string
	DisplayName       string
	ProfileImageURL   string
	VodID             string
	Category          string
	StartedAt         time.Time
	MinuteTS          time.Time
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
}

// PublicHubMomentsResponse is a hosted-safe peak list for one activity-chart bucket.
type PublicHubMomentsResponse struct {
	BucketT               int64                `json:"bucketT"`
	BucketStart           time.Time            `json:"bucketStart"`
	BucketEnd             time.Time            `json:"bucketEnd"`
	HubGeneratedAt        time.Time            `json:"hubGeneratedAt"`
	Source                string               `json:"source"`
	Status                string               `json:"status"`
	Reason                string               `json:"reason,omitempty"`
	ActivityWindowMinutes int                  `json:"activityWindowMinutes"`
	Moments               []HubLivePulseMoment `json:"moments"`
}

func (h *Handler) getPublicHubMoments(w http.ResponseWriter, r *http.Request) {
	opts := publicHubOptionsFromRequest(r)
	bucketT, ok := parseHubBucketT(r.URL.Query().Get("bucketT"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_bucket_t"})
		return
	}
	limit := parseHubMomentsLimit(r.URL.Query().Get("limit"))
	payload, fromCache, err := h.loadPublicHubMoments(r.Context(), false, opts, bucketT, limit)
	if err != nil {
		start, end := hubBucketTimeRange(bucketT, opts.ActivityWindowMinutes)
		slog.Warn("public hub moments unavailable",
			"err", err,
			"bucketT", bucketT,
			"activityWindowMinutes", normalizePublicHubOptions(opts).ActivityWindowMinutes,
			"bucketStart", start,
			"bucketEnd", end,
		)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub_moments_unavailable"})
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, payload)
}

func parseHubBucketT(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parseHubMomentsLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return hubHistoricalMomentsCap
	}
	if n > hubHistoricalMomentsCap {
		return hubHistoricalMomentsCap
	}
	return n
}

func hubBucketTimeRange(bucketT int64, windowMinutes int) (start, end time.Time) {
	bucketMinutes := hubActivityBucketMinutes(windowMinutes)
	bucketMs := int64(bucketMinutes) * 60 * 1000
	if bucketMs <= 0 {
		bucketMs = 60 * 1000
	}
	startMs := (bucketT / bucketMs) * bucketMs
	endMs := startMs + bucketMs
	return time.UnixMilli(startMs).UTC(), time.UnixMilli(endMs).UTC()
}

func publicHubMomentsCacheKey(opts publicHubOptions, bucketT int64, limit int) string {
	opts = normalizePublicHubOptions(opts)
	return publicHubMomentsCachePrefix + ":" + strconv.Itoa(opts.ActivityWindowMinutes) + ":" + strconv.FormatInt(bucketT, 10) + ":" + strconv.Itoa(limit)
}

func (h *Handler) loadPublicHubMoments(ctx context.Context, forceRefresh bool, opts publicHubOptions, bucketT int64, limit int) (PublicHubMomentsResponse, bool, error) {
	opts = normalizePublicHubOptions(opts)
	cacheKey := publicHubMomentsCacheKey(opts, bucketT, limit)
	if !forceRefresh && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			var payload PublicHubMomentsResponse
			if json.Unmarshal(cached, &payload) == nil {
				return payload, true, nil
			}
		}
	}
	payload, err := h.buildPublicHubMoments(ctx, opts, bucketT, limit)
	if err != nil {
		return PublicHubMomentsResponse{}, false, err
	}
	if h.rdb != nil {
		body, _ := json.Marshal(payload)
		_ = h.rdb.Set(ctx, cacheKey, body, publicHubMomentsCacheTTLForPayload(payload)).Err()
	}
	return payload, false, nil
}

func publicHubMomentsCacheTTLForPayload(payload PublicHubMomentsResponse) time.Duration {
	if payload.Status == "empty" && payload.Reason == "no_corpus_peaks_in_bucket" {
		return publicHubMomentsEmptyTTL
	}
	return publicHubMomentsCacheTTL
}

func (h *Handler) buildPublicHubMoments(ctx context.Context, opts publicHubOptions, bucketT int64, limit int) (PublicHubMomentsResponse, error) {
	opts = normalizePublicHubOptions(opts)
	start, end := hubBucketTimeRange(bucketT, opts.ActivityWindowMinutes)
	resp := PublicHubMomentsResponse{
		BucketT:               bucketT,
		BucketStart:           start,
		BucketEnd:             end,
		HubGeneratedAt:        time.Now().UTC(),
		Source:                "corpus_historical",
		Status:                "empty",
		Reason:                "no_corpus_peaks_in_bucket",
		ActivityWindowMinutes: opts.ActivityWindowMinutes,
		Moments:               nil,
	}
	if h == nil || h.store == nil {
		resp.Reason = "store_unavailable"
		return resp, nil
	}
	candidates, err := h.store.TopHistoricalChatMinutesInWindow(ctx, start, end, hubHistoricalCandidateCap)
	if err != nil {
		return PublicHubMomentsResponse{}, err
	}
	if len(candidates) == 0 {
		return resp, nil
	}
	moments := h.hubHistoricalMomentsFromCandidates(ctx, candidates, limit)
	if len(moments) == 0 {
		return resp, nil
	}
	resp.Status = "ready"
	resp.Reason = ""
	resp.Moments = moments
	return resp, nil
}

func (h *Handler) hubHistoricalMomentsFromCandidates(ctx context.Context, candidates []hubHistoricalMinuteCandidate, limit int) []HubLivePulseMoment {
	if limit <= 0 {
		limit = hubHistoricalMomentsCap
	}
	seen := make(map[string]struct{}, limit)
	out := make([]HubLivePulseMoment, 0, limit)
	for _, cand := range candidates {
		streamID := strings.TrimSpace(cand.StreamID)
		if streamID == "" {
			continue
		}
		if _, ok := seen[streamID]; ok {
			continue
		}
		seen[streamID] = struct{}{}
		moment := hubHistoricalMomentFromCandidate(cand)
		if len(cand.Emotes) > 0 {
			top := TopEmotesFromRollups([]MinuteRollup{{
				MinuteTS:          cand.MinuteTS,
				ChatCount:         cand.ChatCount,
				TotalEmoteCount:   cand.TotalEmoteCount,
				SevenTVEmoteCount: cand.SevenTVEmoteCount,
				Emotes:            cand.Emotes,
			}}, 3)
			ext := make([]ExtensionEmote, 0, len(top))
			for _, emote := range top {
				ext = append(ext, ExtensionEmote{
					ID:       emote.ID,
					Name:     emote.Name,
					Provider: emote.Provider,
					ImageURL: emote.ImageURL,
					Count:    emote.Count,
				})
			}
			peak := PortalPeak{TopEmotes: h.decorateExtensionEmotesBatch(ctx, ext)}
			moment.TopEmotes = hubEmotesFromPeak(peak)
			moment.TopEmoteCode = peakTopEmoteCode(peak)
		}
		out = append(out, moment)
		if len(out) >= limit {
			break
		}
	}
	sortHubHistoricalMoments(out)
	return out
}

func hubHistoricalMomentFromCandidate(cand hubHistoricalMinuteCandidate) HubLivePulseMoment {
	login := normalizeLogin(cand.Login)
	displayName := strings.TrimSpace(cand.DisplayName)
	if displayName == "" {
		displayName = login
	}
	offset := 0
	if !cand.StartedAt.IsZero() && !cand.MinuteTS.IsZero() {
		offset = int(math.Max(0, cand.MinuteTS.Sub(cand.StartedAt).Seconds()))
	}
	score := historicalMomentScore(cand.ChatCount, cand.TotalEmoteCount)
	label, kind := historicalMomentLabel(cand)
	vodID := strings.TrimSpace(cand.VodID)
	return HubLivePulseMoment{
		Login:           login,
		DisplayName:     displayName,
		ProfileImageURL: strings.TrimSpace(cand.ProfileImageURL),
		StreamID:        strings.TrimSpace(cand.StreamID),
		VodID:           vodID,
		OffsetSeconds:   offset,
		At:              cand.MinuteTS.UTC().UnixMilli(),
		Score:           score,
		Label:           label,
		Kind:            kind,
		Source:          "corpus_historical",
		ChatPerMin:      cand.ChatCount,
		Confidence:      100,
		VodState:        portalVodState(vodID, offset, vodID != ""),
		Category:        strings.TrimSpace(cand.Category),
		StreamStartedAt: streamStartedAtMs(cand.StartedAt),
	}
}

func streamStartedAtMs(startedAt time.Time) int64 {
	if startedAt.IsZero() {
		return 0
	}
	return startedAt.UTC().UnixMilli()
}

func historicalMomentScore(chatCount, emoteCount int) int {
	score := chatCount / 5
	if emoteCount > chatCount {
		score = emoteCount / 3
	}
	if score < 1 && (chatCount > 0 || emoteCount > 0) {
		score = 1
	}
	if score > 100 {
		score = 100
	}
	return score
}

func historicalMomentLabel(cand hubHistoricalMinuteCandidate) (label, kind string) {
	if cand.SevenTVEmoteCount > 0 && cand.SevenTVEmoteCount >= cand.ChatCount/2 {
		return "Emote spike", "emotes"
	}
	if cand.TotalEmoteCount > cand.ChatCount && cand.TotalEmoteCount > 0 {
		return "Emote spike", "emotes"
	}
	return "Chat spike", "chat"
}

func sortHubHistoricalMoments(moments []HubLivePulseMoment) {
	sort.SliceStable(moments, func(i, j int) bool {
		if moments[i].Score != moments[j].Score {
			return moments[i].Score > moments[j].Score
		}
		return moments[i].ChatPerMin > moments[j].ChatPerMin
	})
}
