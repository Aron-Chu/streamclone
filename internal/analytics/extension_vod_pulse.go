package analytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/analytics/heatmap"
	pulserecap "streamclone/internal/analytics/recap"
)

type ExtensionVodTimelinePoint struct {
	OffsetSeconds int              `json:"offsetSeconds"`
	ChatPerMin    int              `json:"chatPerMin,omitempty"`
	EmotesPerMin  int              `json:"emotesPerMin,omitempty"`
	Viewers       int              `json:"viewers,omitempty"`
	Score         int              `json:"score,omitempty"`
	TopEmotes     []ExtensionEmote `json:"topEmotes,omitempty"`
}

type ExtensionVodTimeline struct {
	BucketSeconds int                         `json:"bucketSeconds"`
	Points        []ExtensionVodTimelinePoint `json:"points"`
}

type ExtensionVodMoment struct {
	OffsetSeconds int              `json:"offsetSeconds"`
	Label         string           `json:"label"`
	Reason        string           `json:"reason,omitempty"`
	Score         int              `json:"score,omitempty"`
	ChatPerMin    int              `json:"chatPerMin,omitempty"`
	EmotesPerMin  int              `json:"emotesPerMin,omitempty"`
	TopEmotes     []ExtensionEmote `json:"topEmotes,omitempty"`
	ThumbnailURL  string           `json:"thumbnailUrl,omitempty"`
}

type ExtensionVodClipCandidate struct {
	OffsetSeconds   int              `json:"offsetSeconds"`
	DurationSeconds int              `json:"durationSeconds,omitempty"`
	Label           string           `json:"label"`
	Reason          string           `json:"reason"`
	Score           int              `json:"score,omitempty"`
	ChatPerMin      int              `json:"chatPerMin,omitempty"`
	EmotesPerMin    int              `json:"emotesPerMin,omitempty"`
	TopEmotes       []ExtensionEmote `json:"topEmotes,omitempty"`
	ThumbnailURL    string           `json:"thumbnailUrl,omitempty"`
}

type ExtensionVodPulseResponse struct {
	Mode               string                     `json:"mode"`
	VodID              string                     `json:"vodId"`
	StreamID           string                     `json:"streamId,omitempty"`
	ChannelLogin       string                     `json:"channelLogin,omitempty"`
	ChannelDisplayName string                     `json:"channelDisplayName,omitempty"`
	Title              string                     `json:"title,omitempty"`
	StartedAt          *time.Time                 `json:"startedAt,omitempty"`
	DurationSeconds    int                        `json:"durationSeconds,omitempty"`
	CoverageStatus     string                     `json:"coverageStatus"`
	CoverageMessage    string                     `json:"coverageMessage,omitempty"`
	FullAnalyticsURL   string                     `json:"fullAnalyticsUrl,omitempty"`
	Recap              *pulserecap.StreamRecap    `json:"recap,omitempty"`
	Timeline           *ExtensionVodTimeline      `json:"timeline,omitempty"`
	TopMoments         []ExtensionVodMoment       `json:"topMoments,omitempty"`
	TopEmotes          []ExtensionEmote           `json:"topEmotes,omitempty"`
	BestClipCandidate  *ExtensionVodClipCandidate `json:"bestClipCandidate,omitempty"`
}

func (h *Handler) extensionPulseVod(w http.ResponseWriter, r *http.Request) {
	vodID := strings.TrimSpace(chi.URLParam(r, "vodId"))
	if vodID == "" || !validExtensionVodID(vodID) {
		slog.Warn("extension_vod_pulse invalid_vod_id", "vodId", vodID, "matched", true)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_vod_id"})
		return
	}
	slog.Info("extension_vod_pulse request", "vodId", vodID, "matched", true)
	payload, err := h.buildExtensionVodPulse(r.Context(), vodID)
	if err != nil {
		slog.Error("extension_vod_pulse build_failed", "vodId", vodID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "vod_pulse_unavailable",
			"message": "Replay Pulse is temporarily unavailable.",
		})
		return
	}
	slog.Info(
		"extension_vod_pulse response",
		"vodId", vodID,
		"streamId", payload.StreamID,
		"channelLogin", payload.ChannelLogin,
		"coverageStatus", payload.CoverageStatus,
	)
	writeJSON(w, http.StatusOK, payload)
}

func validExtensionVodID(vodID string) bool {
	if len(vodID) < 6 || len(vodID) > 20 {
		return false
	}
	for _, ch := range vodID {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (h *Handler) buildExtensionVodPulse(ctx context.Context, vodID string) (ExtensionVodPulseResponse, error) {
	out := ExtensionVodPulseResponse{
		Mode:           "vod",
		VodID:          vodID,
		CoverageStatus: "missing",
	}
	if h.store == nil {
		out.CoverageStatus = "error"
		out.CoverageMessage = "store_unavailable"
		return out, nil
	}

	stream, err := h.store.StreamByVodID(ctx, vodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("extension_vod_pulse stream_lookup", "vodId", vodID, "found", false)
			out.CoverageMessage = "No replay analytics have been indexed for this VOD yet."
			return out, nil
		}
		return ExtensionVodPulseResponse{}, err
	}
	slog.Info(
		"extension_vod_pulse stream_lookup",
		"vodId", vodID,
		"found", true,
		"streamId", stream.StreamID,
		"channelLogin", stream.Login,
	)

	heatmapRollups, startedAt, err := h.consolidateForHeatmap(ctx, stream.StreamID)
	if err != nil {
		return ExtensionVodPulseResponse{}, err
	}

	cfg, cfgErr := heatmap.LoadScoringConfig()
	if cfgErr != nil {
		cfg = heatmap.DefaultScoringConfig()
	}
	alignedPoints := heatmap.AlignedDetailPoints(heatmapRollups, cfg)

	streamStart := stream.StartedAt
	if streamStart.IsZero() && !startedAt.IsZero() {
		streamStart = startedAt
	}

	durationSeconds := 0
	if stream.EndedAt != nil && !streamStart.IsZero() {
		durationSeconds = int(stream.EndedAt.Sub(streamStart).Seconds())
	}
	if durationSeconds <= 0 && len(heatmapRollups) > 0 {
		durationSeconds = len(heatmapRollups) * 60
	}

	backfillRunning := false
	backfillFailed := false
	if h.pulseBackfill != nil && stream.StreamID != "" {
		if active := h.pulseBackfill.ActiveJobForStream(stream.StreamID); active != nil {
			backfillRunning = !isPulseBackfillTerminal(active.Status)
		}
		backfillFailed = h.pulseBackfill.BackfillFailedForStream(stream.StreamID)
	}

	coverage := computePulseCoverage(heatmapRollups, streamStart, durationSeconds, false, vodID, backfillRunning, backfillFailed)
	status, message := extensionVodCoverageStatus(coverage, len(heatmapRollups), backfillRunning)

	fullMax := len(heatmapRollups) + 1
	fullWindowRollups, fullWindowPoints := trimExtensionFullWindow(heatmapRollups, alignedPoints, fullMax, false)
	fullRollups, _ := buildExtensionRollupsAndLanes(fullWindowRollups, fullWindowPoints, streamStart)
	peaks := buildExtensionPeaks(heatmapRollups, alignedPoints, false, "historical", streamStart)

	topEmotes := convertTopEmotesToExtension(
		TopEmotesFromRollups(storeRollupsFromHeatmap(heatmapRollups), 10),
	)

	var recap *pulserecap.StreamRecap
	if built, err := h.buildPulseStreamRecap(ctx, stream.StreamID); err == nil {
		recap = &built
	}

	login := strings.ToLower(strings.TrimSpace(stream.Login))
	out.StreamID = stream.StreamID
	out.ChannelLogin = login
	out.ChannelDisplayName = strings.TrimSpace(stream.DisplayName)
	out.Title = strings.TrimSpace(stream.Title)
	out.DurationSeconds = durationSeconds
	out.CoverageStatus = status
	out.CoverageMessage = message
	if login != "" {
		out.FullAnalyticsURL = fmt.Sprintf("/analytics/%s", login)
		if stream.StreamID != "" {
			out.FullAnalyticsURL = fmt.Sprintf("/analytics/%s/s/%s", login, stream.StreamID)
		}
	}
	if !streamStart.IsZero() {
		t := streamStart.UTC()
		out.StartedAt = &t
	}

	if len(fullRollups) > 0 {
		out.Timeline = buildExtensionVodTimeline(fullRollups, alignedPoints)
	}
	if len(peaks) > 0 {
		out.TopMoments = extensionVodMomentsFromPeaks(peaks, 8)
	}
	if len(topEmotes) > 0 {
		out.TopEmotes = topEmotes
	}
	if recap != nil {
		out.Recap = recap
		if len(out.TopMoments) == 0 {
			out.TopMoments = extensionVodMomentsFromRecap(*recap, 8)
		}
	}
	out.BestClipCandidate = extensionVodBestClip(out.TopMoments, recap, peaks)

	if status == "ready" && len(fullRollups) == 0 && recap == nil {
		out.CoverageStatus = "partial"
		out.CoverageMessage = "Stream linked but replay analytics are still building."
	}

	return out, nil
}

func extensionVodCoverageStatus(coverage ExtensionCoverage, rollupCount int, backfillRunning bool) (status, message string) {
	if backfillRunning {
		return "syncing", "Replay analytics are still syncing for this VOD."
	}
	switch coverage.State {
	case "synced", "complete", "ready":
		if rollupCount > 0 {
			return "ready", ""
		}
		return "partial", "Replay analytics are partially available."
	case "waiting_for_vod", "syncing", "backfill_running", "backfill_pending":
		return "syncing", "Replay analytics are still syncing for this VOD."
	case "backfill_failed", "failed":
		return "partial", "Replay analytics are partially available."
	default:
		if rollupCount > 0 {
			return "partial", "Replay analytics are partially available."
		}
		return "missing", "No replay analytics have been indexed for this VOD yet."
	}
}

func buildExtensionVodTimeline(
	rollups []ExtensionRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
) *ExtensionVodTimeline {
	if len(rollups) == 0 {
		return nil
	}
	scoreByOffset := map[int]int{}
	for _, pt := range points {
		scoreByOffset[pt.OffsetSeconds] = pt.Score
	}
	out := make([]ExtensionVodTimelinePoint, 0, len(rollups))
	for _, rollup := range rollups {
		emotes := rollupEmoteCountFromExtension(rollup)
		out = append(out, ExtensionVodTimelinePoint{
			OffsetSeconds: rollup.OffsetSeconds,
			ChatPerMin:    rollup.ChatCount,
			EmotesPerMin:  emotes,
			Viewers:       rollup.ViewerCount,
			Score:         scoreByOffset[rollup.OffsetSeconds],
			TopEmotes:     rollup.TopEmotes,
		})
	}
	return &ExtensionVodTimeline{BucketSeconds: 60, Points: out}
}

func rollupEmoteCountFromExtension(r ExtensionRollup) int {
	if r.TotalEmoteCount > 0 {
		return r.TotalEmoteCount
	}
	return r.SevenTvEmoteCount
}

func extensionVodMomentsFromPeaks(peaks []ExtensionPeak, limit int) []ExtensionVodMoment {
	if limit <= 0 {
		limit = 8
	}
	out := make([]ExtensionVodMoment, 0, minVodInt(limit, len(peaks)))
	for _, peak := range peaks {
		if len(out) >= limit {
			break
		}
		out = append(out, ExtensionVodMoment{
			OffsetSeconds: peak.OffsetSeconds,
			Label:         peak.ReasonLabel,
			Reason:        firstVodReason(peak.Reasons),
			Score:         peak.Score,
			ChatPerMin:    peak.ChatCount,
			EmotesPerMin:  peak.EmoteCount,
			TopEmotes:     peak.TopEmotes,
		})
	}
	return out
}

func extensionVodMomentsFromRecap(recap pulserecap.StreamRecap, limit int) []ExtensionVodMoment {
	if limit <= 0 {
		limit = 8
	}
	out := make([]ExtensionVodMoment, 0, minVodInt(limit, len(recap.TopMoments)))
	for _, moment := range recap.TopMoments {
		if len(out) >= limit {
			break
		}
		reason := firstVodReason(moment.Reasons)
		emotes := make([]ExtensionEmote, 0, len(moment.TopEmotes))
		for _, emote := range moment.TopEmotes {
			emotes = append(emotes, ExtensionEmote{
				Name:     emote.Code,
				Count:    emote.Count,
				Provider: emote.Provider,
				ID:       emote.ID,
				ImageURL: emote.ImageURL,
			})
		}
		out = append(out, ExtensionVodMoment{
			OffsetSeconds: moment.OffsetSeconds,
			Label:         extensionReasonLabel(reason),
			Reason:        reason,
			Score:         moment.Score,
			ChatPerMin:    moment.ChatCount,
			EmotesPerMin:  moment.EmoteCount,
			TopEmotes:     emotes,
		})
	}
	return out
}

func extensionVodBestClip(
	moments []ExtensionVodMoment,
	recap *pulserecap.StreamRecap,
	peaks []ExtensionPeak,
) *ExtensionVodClipCandidate {
	if recap != nil && len(recap.ClipCandidates) > 0 {
		candidate := recap.ClipCandidates[0]
		reason := firstVodReason(candidate.Reasons)
		return &ExtensionVodClipCandidate{
			OffsetSeconds:   candidate.OffsetSeconds,
			DurationSeconds: 30,
			Label:           extensionReasonLabel(reason),
			Reason:          extensionVodClipReason(candidate.ChatCount, candidate.EmoteCount, reason, candidate.Score),
			Score:           candidate.Score,
			ChatPerMin:      candidate.ChatCount,
			EmotesPerMin:    candidate.EmoteCount,
		}
	}
	if len(moments) > 0 {
		m := moments[0]
		return &ExtensionVodClipCandidate{
			OffsetSeconds:   m.OffsetSeconds,
			DurationSeconds: 30,
			Label:           m.Label,
			Reason:          extensionVodClipReason(m.ChatPerMin, m.EmotesPerMin, m.Reason, m.Score),
			Score:           m.Score,
			ChatPerMin:      m.ChatPerMin,
			EmotesPerMin:    m.EmotesPerMin,
			TopEmotes:       m.TopEmotes,
			ThumbnailURL:    m.ThumbnailURL,
		}
	}
	if len(peaks) > 0 {
		p := peaks[0]
		reason := firstVodReason(p.Reasons)
		return &ExtensionVodClipCandidate{
			OffsetSeconds:   p.OffsetSeconds,
			DurationSeconds: 30,
			Label:           p.ReasonLabel,
			Reason:          extensionVodClipReason(p.ChatCount, p.EmoteCount, reason, p.Score),
			Score:           p.Score,
			ChatPerMin:      p.ChatCount,
			EmotesPerMin:    p.EmoteCount,
			TopEmotes:       p.TopEmotes,
		}
	}
	return nil
}

func extensionVodClipReason(chatPerMin, emotesPerMin int, reason string, score int) string {
	parts := make([]string, 0, 4)
	if chatPerMin > 0 {
		parts = append(parts, fmt.Sprintf("%d chat/min", chatPerMin))
	}
	if emotesPerMin > 0 {
		parts = append(parts, fmt.Sprintf("%d emotes/min", emotesPerMin))
	}
	if label := extensionReasonLabel(reason); label != "" {
		parts = append(parts, strings.ToLower(label))
	}
	if score > 0 {
		parts = append(parts, fmt.Sprintf("%d score", score))
	}
	if len(parts) == 0 {
		return "Peak activity moment"
	}
	return strings.Join(parts, " · ")
}

func firstVodReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return strings.TrimSpace(reasons[0])
}

func minVodInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
