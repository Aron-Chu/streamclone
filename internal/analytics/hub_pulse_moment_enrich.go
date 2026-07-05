package analytics

import (
	"strings"
	"time"

	"streamclone/internal/analytics/heatmap"
)

const (
	hubPulseEarlyStreamMaxSec   = 15 * 60
	hubPulseEarlyOffsetMaxSec   = 300
	hubPulseOpeningOffsetMaxSec = 180
	hubPulseOpeningChatMax      = 20
	hubPulseWarmupChatMax       = 15
)

func peakPrimaryReason(peak PortalPeak) string {
	for _, reason := range peak.Reasons {
		reason = strings.TrimSpace(reason)
		if reason != "" {
			return reason
		}
	}
	label := strings.TrimSpace(peak.ReasonLabel)
	switch label {
	case "Chat spike":
		return heatmap.ReasonChatSpike
	case "Emote spike":
		return "emote_spike"
	case "7TV emote spike":
		return heatmap.ReasonSevenTVSpike
	case "Twitch emote spike":
		return heatmap.ReasonTwitchEmoteSpike
	case "FFZ emote spike":
		return heatmap.ReasonFFZSpike
	case "Viewer spike":
		return heatmap.ReasonViewerSpike
	case "Game change":
		return heatmap.ReasonGameChange
	default:
		return strings.ReplaceAll(strings.ToLower(label), " ", "_")
	}
}

func enrichHubPulseMoment(
	peak PortalPeak,
	coverageStart int,
	streamAgeSec int,
) (label, kind, activityTag string, skip bool) {
	label = strings.TrimSpace(peak.ReasonLabel)
	if label == "" {
		label = "Moment"
	}
	kind = strings.TrimSpace(peak.DominantSignal)
	reason := peakPrimaryReason(peak)
	offset := peak.OffsetSeconds
	chatPerMin := peak.ChatCount
	emoteCount := peak.EmoteCount

	if reason == heatmap.ReasonGameChange || strings.Contains(reason, "game_change") {
		return "Game change", "game_change", "", false
	}

	if offset <= hubPulseOpeningOffsetMaxSec &&
		(reason == heatmap.ReasonViewerSpike || kind == "viewers") &&
		(chatPerMin < hubPulseOpeningChatMax || offset < coverageStart+coverageStartToleranceSec) {
		return "Just went live", "stream_opening", "", false
	}

	if shouldSkipWarmupPeak(offset, coverageStart, chatPerMin, emoteCount) {
		return "", "", "", true
	}

	activityTag = ""
	if streamAgeSec > 0 &&
		streamAgeSec <= hubPulseEarlyStreamMaxSec &&
		offset <= hubPulseEarlyOffsetMaxSec &&
		kind != "stream_opening" {
		activityTag = "early_stream"
	}

	return label, kind, activityTag, false
}

func shouldSkipWarmupPeak(offset, coverageStart, chatPerMin, emoteCount int) bool {
	if coverageStart <= 0 {
		return false
	}
	if offset >= coverageStart+coverageStartToleranceSec {
		return false
	}
	return chatPerMin < hubPulseWarmupChatMax && emoteCount <= 0
}

func streamAgeSeconds(startedAt time.Time, isLive bool) int {
	if startedAt.IsZero() || !isLive {
		return 0
	}
	return int(time.Since(startedAt.UTC()).Seconds())
}
