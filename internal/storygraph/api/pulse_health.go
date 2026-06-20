package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"streamclone/internal/storygraph/score"
	"streamclone/internal/storygraph/store"
)

func (h *Handler) directorySampleStatus(ctx context.Context, window string) map[string]any {
	out := map[string]any{
		"healthy":          false,
		"historyGathering": false,
	}
	if h.samplerHealth != nil {
		snap := h.samplerHealth.Snapshot()
		out["healthy"] = snap.Healthy
		out["lastSampleAt"] = snap.LastSuccessAt
		out["lastError"] = snap.LastError
		out["lastSampleCount"] = snap.LastSampleCount
		out["nextRetryAt"] = snap.NextRetryAt
		out["historyDays"] = snap.HistoryDays
		out["historyGathering"] = snap.HistoryGathering
	} else if last, err := h.store.LastDirectorySampleAt(ctx); err == nil && last != nil {
		out["lastSampleAt"] = last
		out["healthy"] = true
	}
	if strings.EqualFold(window, "7d") {
		if days, err := h.store.DirectorySampleHistoryDays(ctx); err == nil {
			out["historyDays"] = days
			out["historyGathering"] = days > 0 && days < 7
		}
	}
	return out
}

func (h *Handler) directorySamplerHealth(ctx context.Context) map[string]any {
	status := h.directorySampleStatus(ctx, "7d")
	if last, ok := status["lastSampleAt"].(*time.Time); ok && last != nil {
		status["lastSampleAt"] = last
	}
	return status
}

func (h *Handler) windowScoreComputeHealth() map[string]any {
	if h.windowScoreHealth == nil {
		return map[string]any{"healthy": false}
	}
	snap := h.windowScoreHealth.Snapshot()
	return map[string]any{
		"healthy":       snap.Healthy,
		"lastRunAt":     snap.LastRunAt,
		"lastSuccessAt": snap.LastSuccessAt,
		"lastError":     snap.LastError,
	}
}

func (h *Handler) enrichProfileFromMetadata(ctx context.Context, profile *store.StreamerStatProfile) {
	if profile == nil {
		return
	}
	needsFallback := profile.ViewersNow == 0 || profile.DisplayName == ""
	if !needsFallback {
		return
	}
	base := strings.TrimRight(strings.TrimSpace(h.cfg.MetadataServiceURL), "/")
	if base == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/channels/"+profile.Login+"/details", nil)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()
	var details struct {
		DisplayName string `json:"displayName"`
		Game        string `json:"game"`
		IsLive      bool   `json:"isLive"`
		Viewers     int    `json:"viewers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&details); err != nil {
		return
	}
	if profile.DisplayName == "" {
		profile.DisplayName = strings.TrimSpace(details.DisplayName)
	}
	if profile.Category == "" {
		profile.Category = strings.TrimSpace(details.Game)
	}
	if profile.ViewersNow == 0 && details.Viewers > 0 {
		profile.ViewersNow = details.Viewers
		profile.StatsSource = "metadata_fallback"
	}
	if !profile.IsLive && details.IsLive {
		profile.IsLive = details.IsLive
		if profile.StatsSource == "" || profile.StatsSource == "none" {
			profile.StatsSource = "metadata_fallback"
		}
	}
}

func (h *Handler) edition(w http.ResponseWriter, r *http.Request) {
	since, windowLabel, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_window",
			"hint":  "window must be today, 24h, or 7d",
		})
		return
	}
	ctx := r.Context()
	breaking, _ := h.store.ListFeed(ctx, "", "", "", "rank", windowLabel, since, 5, 0)
	corroborated, _ := h.store.ListFeed(ctx, "published", "", "", "rank", windowLabel, since, 8, 0)
	corroborated = score.FilterCorroborated(corroborated)
	if len(corroborated) > 5 {
		corroborated = corroborated[:5]
	}
	bans, _ := h.store.ListFeed(ctx, "", "bans", "", "rank", windowLabel, since, 5, 0)
	spreading, _ := h.store.ListRisingStoryCards(ctx, since, 5)
	recapSince, _, _ := ParseWindow("7d")
	recap, _ := h.store.ListFeed(ctx, "settled", "", "", "rank", "7d", recapSince, 5, 0)
	gainers, _ := h.store.RisingCandidates(ctx, windowLabel, "", 5)
	var biggestMover *store.RisingStreamer
	if len(gainers) > 0 {
		biggestMover = &gainers[0]
	}
	var newEntrants []store.RisingStreamer
	for _, row := range gainers {
		if row.NewEntrant {
			newEntrants = append(newEntrants, row)
		}
	}
	h.avatars.enrichCards(ctx, breaking)
	h.avatars.enrichCards(ctx, corroborated)
	h.avatars.enrichCards(ctx, bans)
	h.avatars.enrichCards(ctx, spreading)
	h.avatars.enrichCards(ctx, recap)
	totalLive, totalViewers := 0, 0
	if last, err := h.store.LastDirectorySampleAt(ctx); err == nil && last != nil {
		totalLive, _ = h.store.CountLiveInLatestRun(ctx)
		totalViewers, _ = h.store.SumViewersInLatestRun(ctx)
	}
	payload := score.WireEdition{
		Window:           windowLabel,
		Since:            since,
		RankModel:        pulseWireRankModel,
		Mode:             score.EditionMode(windowLabel),
		SampleStatus:     h.directorySampleStatus(ctx, windowLabel),
		Breaking:         breaking,
		TopCorroborated:  corroborated,
		BiggestMover:     biggestMover,
		Bans:             bans,
		BansOfTheDay:     bans,
		FastestSpreading: spreading,
		WeeklyRecap:      recap,
		NewEntrants:      newEntrants,
		TopGainers:       gainers,
	}
	if last, err := h.store.LastDirectorySampleAt(ctx); err == nil && last != nil {
		payload.TotalLive = &totalLive
		payload.TotalViewers = &totalViewers
	}
	payload.Sections = score.BuildEditionSections(payload)
	writeJSON(w, http.StatusOK, payload)
}
