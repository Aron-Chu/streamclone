package analytics

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/analytics/recap"
)

const (
	hubFeaturedChartCap      = 60
	hubFeaturedMomentsCap    = 10
	hubFeaturedBurstsCap     = 6
	hubFeaturedMinChatPerMin = 1.0
)

// HubFeaturedSession is a bounded, public-safe preview for the analytics hub.
// It never includes raw chat, emote maps, or storage internals.
type HubFeaturedSession struct {
	State           string                   `json:"state"`
	Reason          string                   `json:"reason,omitempty"`
	Login           string                   `json:"login,omitempty"`
	DisplayName     string                   `json:"displayName,omitempty"`
	StreamID        string                   `json:"streamId,omitempty"`
	Category        string                   `json:"category,omitempty"`
	StartedAt       string                   `json:"startedAt,omitempty"`
	VodID           string                   `json:"vodId,omitempty"`
	Viewers         int                      `json:"viewers,omitempty"`
	ChatPerMin      float64                  `json:"chatPerMin,omitempty"`
	SeventvPerMin   float64                  `json:"seventvPerMin,omitempty"`
	PeakCount       int                      `json:"peakCount,omitempty"`
	DataCoveragePct float64                  `json:"dataCoveragePct,omitempty"`
	TopMoments      []HubFeaturedMoment      `json:"topMoments,omitempty"`
	ChartPoints     []HubFeaturedChartPoint  `json:"chartPoints,omitempty"`
	TopEmoteBursts  []HubFeaturedEmoteBurst  `json:"topEmoteBursts,omitempty"`
	CoverageTruth   []HubFeaturedCoverageRow `json:"coverageTruth,omitempty"`
}

type HubFeaturedMoment struct {
	OffsetSeconds int        `json:"offsetSeconds"`
	Score         int        `json:"score"`
	Label         string     `json:"label"`
	Kind          string     `json:"kind,omitempty"`
	Source        string     `json:"source,omitempty"`
	ChatPerMin    int        `json:"chatPerMin,omitempty"`
	EmotesPerMin  int        `json:"emotesPerMin,omitempty"`
	Viewers       int        `json:"viewers,omitempty"`
	ViewerDelta   string     `json:"viewerDelta,omitempty"`
	TopEmoteCode  string     `json:"topEmoteCode,omitempty"`
	TopEmotes     []HubEmote `json:"topEmotes,omitempty"`
	Confidence    int        `json:"confidence,omitempty"`
	VodState      string     `json:"vodState,omitempty"`
}

type HubFeaturedChartPoint struct {
	OffsetSeconds int `json:"offsetSeconds"`
	ChatNorm      int `json:"chatNorm"`
	ViewersNorm   int `json:"viewersNorm"`
	EmotesNorm    int `json:"emotesNorm"`
	Heat          int `json:"heat"`
}

type HubFeaturedEmoteBurst struct {
	Code              string  `json:"code"`
	Provider          string  `json:"provider,omitempty"`
	ImageURL          string  `json:"imageUrl,omitempty"`
	Count             int     `json:"count"`
	DeltaPct          float64 `json:"deltaPct,omitempty"`
	PeakOffset        string  `json:"peakOffset,omitempty"`
	PeakOffsetSeconds int     `json:"peakOffsetSeconds,omitempty"`
	SharePct          float64 `json:"sharePct,omitempty"`
}

type HubFeaturedCoverageRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Ok    bool   `json:"ok"`
}

func emptyHubFeaturedSession(reason string) HubFeaturedSession {
	return HubFeaturedSession{State: "empty", Reason: reason}
}

func (h *Handler) buildHubFeaturedSession(ctx context.Context, liveChannels []HubLiveChannel) HubFeaturedSession {
	if h.store == nil {
		return emptyHubFeaturedSession("store_unavailable")
	}
	pick := pickHubFeaturedChannel(liveChannels)
	if pick == nil {
		return emptyHubFeaturedSession("no_qualifying_session")
	}
	stream, err := h.store.LatestStreamByLogin(ctx, normalizeLogin(pick.Login))
	if err != nil || stream == nil {
		return emptyHubFeaturedSession("stream_unavailable")
	}
	bundle, err := h.loadPortalSessionFigmaBundle(ctx, stream)
	if err != nil {
		return emptyHubFeaturedSession("rollup_unavailable")
	}
	if len(bundle.peaks) == 0 {
		return emptyHubFeaturedSession("insufficient_peaks")
	}
	metrics := summarizeStreamMetrics(stream, filterTimelineRollups(storeRollupsFromHeatmap(bundle.rollups)))
	vodID := strings.TrimSpace(stream.VodID)
	if vodID == "" {
		vodID = strings.TrimSpace(bundle.vodID)
	}
	coverage := computePulseCoverage(
		bundle.rollups,
		stream.StartedAt,
		bundle.currentOffset,
		bundle.isLive,
		vodID,
		false,
		false,
	)
	recapOut := recap.Build(recap.Input{
		StreamID:        stream.StreamID,
		Login:           stream.Login,
		VodID:           nullableVodID(vodID),
		StartedAt:       stream.StartedAt,
		DurationSeconds: bundle.currentOffset,
		Rollups:         bundle.rollups,
		Points:          bundle.points,
	})
	return HubFeaturedSession{
		State:           "ready",
		Login:           normalizeLogin(stream.Login),
		DisplayName:     displayNameOrLogin(stream),
		StreamID:        stream.StreamID,
		Category:        strings.TrimSpace(stream.Category),
		StartedAt:       stream.StartedAt.UTC().Format(time.RFC3339),
		VodID:           vodID,
		Viewers:         pick.Viewers,
		ChatPerMin:      pick.ChatPerMin,
		SeventvPerMin:   pick.SevenTVPerMin,
		PeakCount:       len(bundle.peaks),
		DataCoveragePct: metrics.DataCoveragePct,
		TopMoments:      hubFeaturedMomentsFromPeaks(bundle.peaks, bundle.rollups, bundle.points, vodID),
		ChartPoints:     hubFeaturedChartPoints(bundle.rollups, bundle.points),
		TopEmoteBursts:  hubFeaturedBurstsFromRecap(recapOut, bundle.peaks),
		CoverageTruth:   hubFeaturedCoverageRows(coverage, vodID, metrics.DataCoveragePct, pick.SevenTVPerMin),
	}
}

func pickHubFeaturedChannel(liveChannels []HubLiveChannel) *HubLiveChannel {
	var pick *HubLiveChannel
	for i := range liveChannels {
		ch := &liveChannels[i]
		if strings.TrimSpace(ch.Login) == "" || ch.ChatPerMin < hubFeaturedMinChatPerMin {
			continue
		}
		if pick == nil || ch.ChatPerMin > pick.ChatPerMin {
			pick = ch
		}
	}
	return pick
}

func hubFeaturedMomentsFromPeaks(
	peaks []PortalPeak,
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	vodID string,
) []HubFeaturedMoment {
	out := make([]HubFeaturedMoment, 0, hubFeaturedMomentsCap)
	for _, peak := range peaks {
		if len(out) >= hubFeaturedMomentsCap {
			break
		}
		vodState := portalVodState(vodID, peak.OffsetSeconds, vodID != "")
		out = append(out, HubFeaturedMoment{
			OffsetSeconds: peak.OffsetSeconds,
			Score:         peak.Score,
			Label:         peak.ReasonLabel,
			Kind:          strings.TrimSpace(peak.DominantSignal),
			Source:        hubFeaturedMomentSource(vodState),
			ChatPerMin:    peak.ChatCount,
			EmotesPerMin:  peak.EmoteCount,
			Viewers:       peak.Viewers,
			ViewerDelta:   peak.ViewerDelta,
			TopEmoteCode:  peakTopEmoteCode(peak),
			TopEmotes:     hubEmotesFromPeak(peak),
			Confidence:    peak.Confidence,
			VodState:      vodState,
		})
	}
	if len(out) > 0 {
		return out
	}
	// Fallback when peaks endpoint shape is empty but points exist.
	for _, pt := range points {
		if pt.Score <= 0 || len(out) >= hubFeaturedMomentsCap {
			break
		}
		chat := 0
		viewers := 0
		if rollup, ok := rollupAtOffset(rollups, points, pt.OffsetSeconds); ok {
			chat = rollup.ChatCount
			viewers = rollup.ViewerLatest
		}
		vodState := portalVodState(vodID, pt.OffsetSeconds, vodID != "")
		out = append(out, HubFeaturedMoment{
			OffsetSeconds: pt.OffsetSeconds,
			Score:         pt.Score,
			Label:         extensionReasonLabel(pt.Reason),
			Kind:          dominantSignalFromReason(pt.Reason),
			Source:        hubFeaturedMomentSource(vodState),
			ChatPerMin:    chat,
			Viewers:       viewers,
			Confidence:    int(math.Round(pt.Confidence * 100)),
			VodState:      vodState,
		})
	}
	return out
}

func peakTopEmoteCode(peak PortalPeak) string {
	if len(peak.TopEmotes) == 0 {
		return ""
	}
	return strings.TrimSpace(peak.TopEmotes[0].Name)
}

func hubEmotesFromPeak(peak PortalPeak) []HubEmote {
	if len(peak.TopEmotes) == 0 {
		return nil
	}
	out := make([]HubEmote, 0, len(peak.TopEmotes))
	for _, emote := range peak.TopEmotes {
		name := strings.TrimSpace(emote.Name)
		if name == "" {
			continue
		}
		out = append(out, HubEmote{
			Name:     name,
			Provider: emote.Provider,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
		})
	}
	return out
}

func hubFeaturedMomentSource(vodState string) string {
	switch strings.ToLower(strings.TrimSpace(vodState)) {
	case "synced":
		return "vod_synced"
	case "partial":
		return "partial"
	default:
		return "live_irc"
	}
}

func hubBurstFromExtensionEmote(emote ExtensionEmote, peakOffset string, peakOffsetSeconds int, sharePct float64) HubFeaturedEmoteBurst {
	code := strings.TrimSpace(emote.Name)
	return HubFeaturedEmoteBurst{
		Code:              code,
		Provider:          emote.Provider,
		ImageURL:          emote.ImageURL,
		Count:             emote.Count,
		PeakOffset:        peakOffset,
		PeakOffsetSeconds: peakOffsetSeconds,
		SharePct:          sharePct,
	}
}

func hubFeaturedChartPoints(rollups []heatmap.MinuteRollup, points []heatmap.ReplayHeatmapDetailPoint) []HubFeaturedChartPoint {
	if len(rollups) == 0 {
		return nil
	}
	start := 0
	if len(rollups) > hubFeaturedChartCap {
		start = len(rollups) - hubFeaturedChartCap
	}
	slice := rollups[start:]
	pointSlice := points
	if len(pointSlice) > len(slice) {
		pointSlice = pointSlice[len(pointSlice)-len(slice):]
	}
	maxChat, maxViewers, maxEmotes, maxHeat := 1, 1, 1, 1
	for i, rollup := range slice {
		maxChat = hubMaxInt(maxChat, rollup.ChatCount)
		maxViewers = hubMaxInt(maxViewers, rollup.ViewerLatest)
		maxEmotes = hubMaxInt(maxEmotes, rollupEmoteCount(rollup))
		if i < len(pointSlice) {
			maxHeat = hubMaxInt(maxHeat, pointSlice[i].Score)
		}
	}
	out := make([]HubFeaturedChartPoint, 0, len(slice))
	for i, rollup := range slice {
		offset := i * 60
		if i < len(pointSlice) {
			offset = pointSlice[i].OffsetSeconds
		}
		heat := 0
		if i < len(pointSlice) {
			heat = pointSlice[i].Score
		}
		out = append(out, HubFeaturedChartPoint{
			OffsetSeconds: offset,
			ChatNorm:      normPct(rollup.ChatCount, maxChat),
			ViewersNorm:   normPct(rollup.ViewerLatest, maxViewers),
			EmotesNorm:    normPct(rollupEmoteCount(rollup), maxEmotes),
			Heat:          normPct(heat, maxHeat),
		})
	}
	return out
}

func hubFeaturedBurstsFromRecap(rec recap.StreamRecap, peaks []PortalPeak) []HubFeaturedEmoteBurst {
	out := make([]HubFeaturedEmoteBurst, 0, hubFeaturedBurstsCap)
	seen := make(map[string]bool)
	if rec.FunniestEmoteBurst != nil && rec.FunniestEmoteBurst.Count > 0 {
		code := strings.TrimSpace(rec.FunniestEmoteBurst.Code)
		if code == "" {
			code = "emote burst"
		}
		out = append(out, HubFeaturedEmoteBurst{
			Code:              code,
			Count:             rec.FunniestEmoteBurst.Count,
			PeakOffset:        formatCoverageOffset(rec.FunniestEmoteBurst.OffsetSeconds),
			PeakOffsetSeconds: rec.FunniestEmoteBurst.OffsetSeconds,
		})
		seen[code] = true
	}
	for _, peak := range peaks {
		for _, emote := range peak.TopEmotes {
			code := strings.TrimSpace(emote.Name)
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			out = append(out, hubBurstFromExtensionEmote(
				emote,
				formatCoverageOffset(peak.OffsetSeconds),
				peak.OffsetSeconds,
				emoteSharePct(emote.Count, peak.EmoteCount),
			))
			if len(out) >= hubFeaturedBurstsCap {
				return out
			}
		}
	}
	for _, emote := range rec.TopEmotes {
		code := strings.TrimSpace(emote.Code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, HubFeaturedEmoteBurst{
			Code:  code,
			Count: emote.Count,
		})
		if len(out) >= hubFeaturedBurstsCap {
			break
		}
	}
	return out
}

func emoteSharePct(count, total int) float64 {
	if total <= 0 || count <= 0 {
		return 0
	}
	return math.Round(float64(count)/float64(total)*1000) / 10
}

func hubFeaturedCoverageRows(cov ExtensionCoverage, vodID string, dataCoveragePct float64, seventvPerMin float64) []HubFeaturedCoverageRow {
	vodOk := strings.TrimSpace(vodID) != ""
	rows := []HubFeaturedCoverageRow{
		{
			Label: "VOD available",
			Value: ternary(vodOk, "Yes", "No"),
			Ok:    vodOk,
		},
		{
			Label: "Chat replay",
			Value: cov.Message,
			Ok:    cov.HasFullStreamCoverage || !cov.HasGaps,
		},
		{
			Label: "Source confidence",
			Value: fmt.Sprintf("%.0f%%", dataCoveragePct),
			Ok:    dataCoveragePct >= 70,
		},
		{
			Label: "Backfill",
			Value: backfillLabel(cov),
			Ok:    !cov.HasGaps || cov.CanBackfill,
		},
		{
			Label: "Data freshness",
			Value: ternary(cov.State == CoverageStateBackfillRunning, "Syncing", "Current window"),
			Ok:    cov.State != CoverageStateBackfillFailed,
		},
		{
			Label: "7TV coverage",
			Value: fmt.Sprintf("%.0f / min", seventvPerMin),
			Ok:    seventvPerMin > 0,
		},
	}
	return rows
}

func backfillLabel(cov ExtensionCoverage) string {
	switch cov.State {
	case CoverageStateBackfillRunning:
		return "Running"
	case CoverageStateBackfillFailed:
		return "Failed"
	case CoverageStateWaitingForVOD:
		return "Waiting for VOD"
	case CoverageStateVODUnavailable:
		return "Unavailable"
	default:
		if cov.CanBackfill {
			return "Available"
		}
		if cov.HasGaps {
			return "Gaps detected"
		}
		return "Not needed"
	}
}

func normPct(value, max int) int {
	if max <= 0 || value <= 0 {
		return 0
	}
	return int(math.Min(100, math.Round(float64(value)/float64(max)*100)))
}

func hubMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ternary(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func nullableVodID(vodID string) *string {
	if strings.TrimSpace(vodID) == "" {
		return nil
	}
	v := strings.TrimSpace(vodID)
	return &v
}
