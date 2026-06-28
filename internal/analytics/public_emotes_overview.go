package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	publicEmotesOverviewCacheKeyPrefix = "sp:public:emotes:overview"
	publicEmotesOverviewTTL            = 60 * time.Second
	publicEmotesOverviewSchemaVersion  = "ph3-auto-002b"
)

var publicEmotesForbiddenKeys = map[string]struct{}{
	"rawChat":            {},
	"rawChatText":        {},
	"chatText":           {},
	"messageText":        {},
	"fragments":          {},
	"chatter":            {},
	"chatterId":          {},
	"chatterLogin":       {},
	"chatterUsername":    {},
	"userId":             {},
	"userRankings":       {},
	"chatterLeaderboard": {},
	"viewerList":         {},
	"messages":           {},
}

type PublicProviderPreview struct {
	Provider       string  `json:"provider"`
	SharePct       float64 `json:"sharePct"`
	TotalUses      int64   `json:"totalUses"`
	TrackedMinutes int64   `json:"trackedMinutes"`
	CoveragePct    float64 `json:"coveragePct"`
	Confidence     float64 `json:"confidence"`
}

type PublicCreatorPreviewRow struct {
	Login          string  `json:"login"`
	DisplayName    string  `json:"displayName"`
	MetricLabel    string  `json:"metricLabel"`
	MetricValue    string  `json:"metricValue"`
	TrackedMinutes int64   `json:"trackedMinutes"`
	CoveragePct    float64 `json:"coveragePct"`
	Confidence     float64 `json:"confidence"`
	Placeholder    bool    `json:"placeholder"`
}

type PublicRisingEmotePreviewRow struct {
	EmoteKey       string  `json:"emoteKey"`
	Name           string  `json:"name"`
	Provider       string  `json:"provider"`
	TrendLabel     string  `json:"trendLabel"`
	TrendValue     string  `json:"trendValue"`
	TrackedMinutes int64   `json:"trackedMinutes"`
	CoveragePct    float64 `json:"coveragePct"`
	Confidence     float64 `json:"confidence"`
	Placeholder    bool    `json:"placeholder"`
}

type PublicEmotesSuppressionRules struct {
	Mode                   string `json:"mode"`
	MinimumTrackedMinutes  int64  `json:"minimumTrackedMinutes"`
	MinimumCoveragePct     int64  `json:"minimumCoveragePct"`
	MinimumConfidencePct   int64  `json:"minimumConfidencePct"`
	MinimumTotalUses       int64  `json:"minimumTotalUses"`
	SuppressedWhenCoverage bool   `json:"suppressedWhenCoverageLow"`
}

type PublicEmotesOverviewResponse struct {
	Range                     string                        `json:"range"`
	GeneratedAt               time.Time                     `json:"generatedAt"`
	SchemaVersion             string                        `json:"schemaVersion"`
	State                     string                        `json:"state"`
	Degraded                  bool                          `json:"degraded"`
	StalenessSec              int64                         `json:"stalenessSec"`
	TrackedMinutes            int64                         `json:"trackedMinutes"`
	CoveragePct               float64                       `json:"coveragePct"`
	Confidence                float64                       `json:"confidence"`
	AggregateOnly             bool                          `json:"aggregateOnly"`
	ProviderSummaryPreview    []PublicProviderPreview       `json:"providerSummaryPreview"`
	CreatorLeaderboardPreview []PublicCreatorPreviewRow     `json:"creatorLeaderboardPreview"`
	RisingEmotePreview        []PublicRisingEmotePreviewRow `json:"risingEmotePreview"`
	SuppressionRules          PublicEmotesSuppressionRules  `json:"suppressionRules"`
	UnavailableReason         string                        `json:"unavailableReason,omitempty"`
}

func parsePublicEmotesRange(raw string) string {
	rangeValue := strings.ToLower(strings.TrimSpace(raw))
	switch rangeValue {
	case "24h", "7d", "30d", "90d":
		return rangeValue
	default:
		return "7d"
	}
}

func publicEmotesOverviewCacheKey(rangeValue string) string {
	return publicEmotesOverviewCacheKeyPrefix + ":" + publicEmotesOverviewSchemaVersion + ":" + parsePublicEmotesRange(rangeValue)
}

func (h *Handler) getPublicEmotesOverview(w http.ResponseWriter, r *http.Request) {
	rangeValue := parsePublicEmotesRange(r.URL.Query().Get("range"))
	payload, fromCache, err := h.loadPublicEmotesOverview(r.Context(), false, rangeValue)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":              "public_emotes_overview_unavailable",
			"range":              rangeValue,
			"schemaVersion":      publicEmotesOverviewSchemaVersion,
			"aggregateOnly":      true,
			"minimumTrackedMins": int64(300),
		})
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) loadPublicEmotesOverview(ctx context.Context, forceRefresh bool, rangeValue string) (PublicEmotesOverviewResponse, bool, error) {
	rangeValue = parsePublicEmotesRange(rangeValue)
	cacheKey := publicEmotesOverviewCacheKey(rangeValue)
	if !forceRefresh && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			var payload PublicEmotesOverviewResponse
			if json.Unmarshal(cached, &payload) == nil {
				if err := ensureNoForbiddenPublicKeys(payload); err != nil {
					return PublicEmotesOverviewResponse{}, false, err
				}
				return payload, true, nil
			}
		}
	}

	v, err, _ := h.hubGroup.Do(cacheKey, func() (any, error) {
		if !forceRefresh && h.rdb != nil {
			if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
				var payload PublicEmotesOverviewResponse
				if json.Unmarshal(cached, &payload) == nil {
					if err := ensureNoForbiddenPublicKeys(payload); err != nil {
						return PublicEmotesOverviewResponse{}, err
					}
					return payload, nil
				}
			}
		}

		payload, buildErr := h.buildPublicEmotesOverview(ctx, rangeValue)
		if buildErr != nil {
			return PublicEmotesOverviewResponse{}, buildErr
		}
		if err := ensureNoForbiddenPublicKeys(payload); err != nil {
			return PublicEmotesOverviewResponse{}, err
		}
		if h.rdb != nil {
			body, _ := json.Marshal(payload)
			_ = h.rdb.Set(ctx, cacheKey, body, publicEmotesOverviewTTL).Err()
		}
		return payload, nil
	})
	if err != nil {
		return PublicEmotesOverviewResponse{}, false, err
	}
	return v.(PublicEmotesOverviewResponse), false, nil
}

func (h *Handler) buildPublicEmotesOverview(ctx context.Context, rangeValue string) (PublicEmotesOverviewResponse, error) {
	now := time.Now().UTC()
	suppression := PublicEmotesSuppressionRules{
		Mode:                   "suppress_below_minimums",
		MinimumTrackedMinutes:  publicEmoteMinimumTrackedMinutes,
		MinimumCoveragePct:     int64(publicEmoteMinimumCoveragePct),
		MinimumConfidencePct:   int64(publicEmoteMinimumConfidencePct),
		MinimumTotalUses:       publicEmoteMinimumTotalUses,
		SuppressedWhenCoverage: true,
	}

	payload := PublicEmotesOverviewResponse{
		Range:                  rangeValue,
		GeneratedAt:            now,
		SchemaVersion:          publicEmotesOverviewSchemaVersion,
		State:                  "empty",
		Degraded:               false,
		StalenessSec:           0,
		TrackedMinutes:         0,
		CoveragePct:            0,
		Confidence:             0,
		AggregateOnly:          true,
		ProviderSummaryPreview: []PublicProviderPreview{},
		CreatorLeaderboardPreview: []PublicCreatorPreviewRow{
			{
				Login:          "preview_creator",
				DisplayName:    "Preview Creator",
				MetricLabel:    "Placeholder only",
				MetricValue:    "Ranking not enabled",
				TrackedMinutes: 0,
				CoveragePct:    0,
				Confidence:     0,
				Placeholder:    true,
			},
		},
		RisingEmotePreview: []PublicRisingEmotePreviewRow{
			{
				EmoteKey:       "placeholder:preview",
				Name:           "Preview emote",
				Provider:       "placeholder",
				TrendLabel:     "Placeholder only",
				TrendValue:     "Trend scoring not enabled",
				TrackedMinutes: 0,
				CoveragePct:    0,
				Confidence:     0,
				Placeholder:    true,
			},
		},
		SuppressionRules: suppression,
	}

	if h == nil || h.store == nil {
		payload.State = "unavailable"
		payload.Degraded = true
		payload.UnavailableReason = "store_unavailable"
		return payload, nil
	}

	landscape, err := h.store.PublicEmoteProviderLandscape(ctx, rangeValue, now)
	if err != nil {
		payload.State = "unavailable"
		payload.Degraded = true
		payload.UnavailableReason = "provider_landscape_unavailable"
		return payload, nil
	}

	applyPublicEmotesProviderLandscape(&payload, landscape)
	return payload, nil
}

func applyPublicEmotesProviderLandscape(payload *PublicEmotesOverviewResponse, landscape PublicEmoteProviderLandscape) {
	if payload == nil {
		return
	}
	suppression := payload.SuppressionRules
	payload.ProviderSummaryPreview = landscape.Rows
	payload.TrackedMinutes = landscape.TrackedMinutes
	payload.CoveragePct = clampPublicPct(landscape.CoveragePct)
	payload.Confidence = clampPublicPct(landscape.Confidence)
	payload.StalenessSec = landscape.StalenessSec
	if len(landscape.Rows) == 0 || landscape.TrackedMinutes == 0 || landscape.TotalUses < suppression.MinimumTotalUses {
		payload.State = "empty"
	} else if landscape.TrackedMinutes < suppression.MinimumTrackedMinutes ||
		payload.CoveragePct < float64(suppression.MinimumCoveragePct) ||
		payload.Confidence < float64(suppression.MinimumConfidencePct) ||
		time.Duration(landscape.StalenessSec)*time.Second > publicEmoteProviderFreshnessMax ||
		landscape.LastRun.Status == "failed" {
		payload.State = "degraded"
		payload.Degraded = true
	} else {
		payload.State = "ready"
	}
}

func clampPublicPct(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func collectForbiddenPublicKeys(value any, path string, matches *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			nextPath := path + "." + key
			if _, banned := publicEmotesForbiddenKeys[key]; banned {
				*matches = append(*matches, nextPath)
			}
			collectForbiddenPublicKeys(nested, nextPath, matches)
		}
	case []any:
		for idx, item := range typed {
			nextPath := fmt.Sprintf("%s[%d]", path, idx)
			collectForbiddenPublicKeys(item, nextPath, matches)
		}
	}
}

func forbiddenPublicKeys(value any) []string {
	var matches []string
	collectForbiddenPublicKeys(value, "$", &matches)
	sort.Strings(matches)
	return matches
}

func ensureNoForbiddenPublicKeys(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	if matches := forbiddenPublicKeys(decoded); len(matches) > 0 {
		return fmt.Errorf("forbidden public payload keys: %s", strings.Join(matches, ", "))
	}
	return nil
}
