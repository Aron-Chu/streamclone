package analytics

import (
	"context"
	"sort"
	"strings"
	"time"
)

const (
	hubLivePulseMomentsCap      = 10
	hubLivePulseScanCap         = 12
	hubLivePulsePeaksPerChannel = 5
)

// HubLivePulseMoment is a public-safe peak row for the analytics hub Pulse
// Moments Live panel. Each row carries its channel identity so the portal can
// show network-wide IRC peaks, not just one featured session.
type HubLivePulseMoment struct {
	Login           string     `json:"login,omitempty"`
	DisplayName     string     `json:"displayName,omitempty"`
	ProfileImageURL string     `json:"profileImageUrl,omitempty"`
	StreamID        string     `json:"streamId,omitempty"`
	VodID           string     `json:"vodId,omitempty"`
	OffsetSeconds   int        `json:"offsetSeconds"`
	At              int64      `json:"at,omitempty"`
	Score           int        `json:"score"`
	Label           string     `json:"label"`
	Kind            string     `json:"kind,omitempty"`
	Source          string     `json:"source,omitempty"`
	ChatPerMin      int        `json:"chatPerMin,omitempty"`
	ViewerDelta     string     `json:"viewerDelta,omitempty"`
	TopEmoteCode    string     `json:"topEmoteCode,omitempty"`
	TopEmotes       []HubEmote `json:"topEmotes,omitempty"`
	Confidence      int        `json:"confidence,omitempty"`
	VodState        string     `json:"vodState,omitempty"`
	Category        string     `json:"category,omitempty"`
	StreamStartedAt int64      `json:"streamStartedAt,omitempty"`
	ActivityTag     string     `json:"activityTag,omitempty"`
}

// hubLivePulseMomentsMeta exposes a hosted-safe deploy/readiness signal for the
// portal when network-wide peaks are empty. Old analytics deploys omit these
// fields entirely; the portal treats missing status as undeployed network API.
func hubLivePulseMomentsMeta(liveChannels []HubLiveChannel, moments []HubLivePulseMoment) (status, reason string) {
	if len(moments) > 0 {
		return "ready", ""
	}
	eligible := 0
	for _, ch := range liveChannels {
		if strings.TrimSpace(ch.Login) != "" && ch.ChatPerMin >= hubFeaturedMinChatPerMin {
			eligible++
		}
	}
	if eligible == 0 {
		return "fallback", "no_irc_eligible_channels"
	}
	return "no_peaks", "no_detected_peaks_in_pool"
}

func (h *Handler) buildHubLivePulseMoments(ctx context.Context, liveChannels []HubLiveChannel) []HubLivePulseMoment {
	if h == nil || h.store == nil || len(liveChannels) == 0 {
		return nil
	}
	sorted := append([]HubLiveChannel(nil), liveChannels...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].ChatPerMin != sorted[j].ChatPerMin {
			return sorted[i].ChatPerMin > sorted[j].ChatPerMin
		}
		return sorted[i].Viewers > sorted[j].Viewers
	})

	candidates := make([]HubLivePulseMoment, 0, hubLivePulseMomentsCap*2)
	scanned := 0
	for _, ch := range sorted {
		if scanned >= hubLivePulseScanCap {
			break
		}
		login := normalizeLogin(ch.Login)
		if login == "" || ch.ChatPerMin < hubFeaturedMinChatPerMin {
			continue
		}
		stream, err := h.store.LatestStreamByLogin(ctx, login)
		if err != nil || stream == nil {
			continue
		}
		streamID := strings.TrimSpace(stream.StreamID)
		bundle, err := h.loadPortalSessionFigmaBundle(ctx, stream)
		if err != nil || len(bundle.peaks) == 0 {
			continue
		}
		scanned++
		vodID := strings.TrimSpace(bundle.vodID)
		if vodID == "" {
			vodID = strings.TrimSpace(stream.VodID)
		}
		limit := hubLivePulsePeaksPerChannel
		if limit > len(bundle.peaks) {
			limit = len(bundle.peaks)
		}
		coverageStart := coverageStartOffsetSeconds(bundle.rollups, bundle.startedAt)
		streamAgeSec := streamAgeSeconds(bundle.startedAt, bundle.isLive)
		for _, peak := range bundle.peaks[:limit] {
			peak = h.enrichPortalPeakEmotes(ctx, peak, bundle.rollups, bundle.points)
			category := h.resolveHubMomentCategory(ctx, streamID, stream, ch, peak.OffsetSeconds)
			moment, ok := hubLivePulseMomentFromPeak(ch, peak, vodID, stream.StartedAt, streamID, coverageStart, streamAgeSec, category)
			if !ok {
				continue
			}
			candidates = append(candidates, moment)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].ChatPerMin != candidates[j].ChatPerMin {
			return candidates[i].ChatPerMin > candidates[j].ChatPerMin
		}
		return candidates[i].OffsetSeconds > candidates[j].OffsetSeconds
	})

	if len(candidates) > hubLivePulseMomentsCap {
		candidates = candidates[:hubLivePulseMomentsCap]
	}
	return candidates
}

func hubLivePulseMomentFromPeak(
	ch HubLiveChannel,
	peak PortalPeak,
	vodID string,
	startedAt time.Time,
	streamID string,
	coverageStart int,
	streamAgeSec int,
	category string,
) (HubLivePulseMoment, bool) {
	label, kind, activityTag, skip := enrichHubPulseMoment(peak, coverageStart, streamAgeSec)
	if skip {
		return HubLivePulseMoment{}, false
	}
	vodState := strings.TrimSpace(peak.VodState)
	if vodState == "" {
		vodState = portalVodState(vodID, peak.OffsetSeconds, vodID != "")
	}
	displayName := strings.TrimSpace(ch.DisplayName)
	if displayName == "" {
		displayName = ch.Login
	}
	var at int64
	var streamStartedAt int64
	if !startedAt.IsZero() {
		streamStartedAt = startedAt.UTC().UnixMilli()
		if peak.OffsetSeconds >= 0 {
			at = startedAt.Add(time.Duration(peak.OffsetSeconds) * time.Second).UnixMilli()
		}
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = strings.TrimSpace(ch.Category)
	}
	return HubLivePulseMoment{
		Login:           normalizeLogin(ch.Login),
		DisplayName:     displayName,
		ProfileImageURL: ch.ProfileImageURL,
		StreamID:        strings.TrimSpace(streamID),
		VodID:           vodID,
		OffsetSeconds:   peak.OffsetSeconds,
		At:              at,
		Score:           peak.Score,
		Label:           label,
		Kind:            kind,
		Source:          hubFeaturedMomentSource(vodState),
		ChatPerMin:      peak.ChatCount,
		ViewerDelta:     peak.ViewerDelta,
		TopEmoteCode:    peakTopEmoteCode(peak),
		TopEmotes:       hubEmotesFromPeak(peak),
		Confidence:      peak.Confidence,
		VodState:        vodState,
		Category:        category,
		StreamStartedAt: streamStartedAt,
		ActivityTag:     activityTag,
	}, true
}
