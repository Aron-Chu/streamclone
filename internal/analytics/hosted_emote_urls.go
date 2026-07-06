package analytics

import (
	"context"
	"strings"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/emoteimage"
)

func (h *Handler) hostedEmoteCDNBase() string {
	if h == nil || !h.pulseHosted.Hosted {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(h.cdnPublicBase), "/")
}

func (h *Handler) rewriteHostedTopEmotes(ctx context.Context, emotes []TopEmote) []TopEmote {
	if len(emotes) == 0 {
		return emotes
	}
	hosted := h != nil && h.pulseHosted.Hosted
	base := h.hostedEmoteCDNBase()
	metadata := h.lookupEmoteMetadataForTopEmotes(ctx, emotes)
	var lookup map[string]string
	if hosted || base != "" {
		lookup = h.lookupProviderIDsForTopEmotes(ctx, emotes)
	}
	if !hosted && base == "" && len(metadata) == 0 {
		return emotes
	}
	return rewriteHostedTopEmoteURLs(emotes, lookup, metadata, base, hosted || base != "")
}

// rewritePortalTopEmotes resolves browser-loadable image URLs for public hub responses.
// Always performs provider-id lookup so synced 7TV UUIDs become public CDN URLs even
// when PULSE_HOSTED_MODE is false (local portal dev against localhost:8090).
func (h *Handler) rewritePortalTopEmotes(ctx context.Context, emotes []TopEmote) []TopEmote {
	if len(emotes) == 0 {
		return emotes
	}
	metadata := h.lookupEmoteMetadataForTopEmotes(ctx, emotes)
	lookup := h.lookupProviderIDsForTopEmotes(ctx, emotes)
	base := h.hostedEmoteCDNBase()
	return rewriteHostedTopEmoteURLs(emotes, lookup, metadata, base, true)
}

func rewriteHostedTopEmoteURLs(emotes []TopEmote, lookup map[string]string, metadata map[string]EmoteMetadata, cdnBase string, rewriteURLs bool) []TopEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := make([]TopEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		id := strings.TrimSpace(out[i].ID)
		if meta, ok := metadata[id]; ok {
			out[i].ZeroWidth = meta.ZeroWidth
			out[i].Animated = meta.Animated
		}
		if rewriteURLs {
			providerID := ""
			if lookup != nil {
				providerID = lookup[id]
			}
			out[i].ImageURL = emoteimage.HostedBrowserURL(cdnBase, out[i].Provider, out[i].ID, providerID)
		}
	}
	return out
}

func (h *Handler) lookupEmoteMetadataForTopEmotes(ctx context.Context, emotes []TopEmote) map[string]EmoteMetadata {
	if h == nil || h.store == nil {
		return map[string]EmoteMetadata{}
	}
	ids := make([]string, 0, len(emotes))
	for _, e := range emotes {
		if id := strings.TrimSpace(e.ID); id != "" {
			ids = append(ids, id)
		}
	}
	lookup, err := h.store.LookupEmoteMetadata(ctx, ids)
	if err != nil {
		return map[string]EmoteMetadata{}
	}
	return lookup
}

func (h *Handler) lookupProviderIDsForTopEmotes(ctx context.Context, emotes []TopEmote) map[string]string {
	if h == nil || h.store == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(emotes))
	for _, e := range emotes {
		if id := strings.TrimSpace(e.ID); id != "" {
			ids = append(ids, id)
		}
	}
	lookup, err := h.store.LookupProviderEmoteIDs(ctx, ids)
	if err != nil {
		return map[string]string{}
	}
	return lookup
}

func rewriteHostedExtensionEmoteURLs(emotes []ExtensionEmote, lookup map[string]string, cdnBase string) []ExtensionEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := make([]ExtensionEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		providerID := lookup[strings.TrimSpace(out[i].ID)]
		out[i].ImageURL = emoteimage.HostedBrowserURL(cdnBase, out[i].Provider, out[i].ID, providerID)
	}
	return out
}

func decorateExtensionEmotes(emotes []ExtensionEmote, lookup map[string]string, metadata map[string]EmoteMetadata, cdnBase string) []ExtensionEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := rewriteHostedExtensionEmoteURLs(emotes, lookup, cdnBase)
	if len(metadata) == 0 {
		return out
	}
	for i := range out {
		if meta, ok := metadata[strings.TrimSpace(out[i].ID)]; ok {
			out[i].ZeroWidth = meta.ZeroWidth
			out[i].Animated = meta.Animated
		}
	}
	return out
}

func collectExtensionEmoteLocalIDs(emotes []ExtensionEmote) []string {
	if len(emotes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(emotes))
	out := make([]string, 0, len(emotes))
	for _, emote := range emotes {
		id := strings.TrimSpace(emote.ID)
		if id == "" || !emoteimage.IsLocalEmoteID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (h *Handler) lookupDecorationForExtensionEmotes(ctx context.Context, emotes []ExtensionEmote) (map[string]string, map[string]EmoteMetadata) {
	lookup := map[string]string{}
	metadata := map[string]EmoteMetadata{}
	if h == nil || h.store == nil {
		return lookup, metadata
	}
	localIDs := collectExtensionEmoteLocalIDs(emotes)
	if len(localIDs) == 0 {
		return lookup, metadata
	}
	if resolved, err := h.store.LookupProviderEmoteIDs(ctx, localIDs); err == nil {
		lookup = resolved
	}
	if meta, err := h.store.LookupEmoteMetadata(ctx, localIDs); err == nil {
		metadata = meta
	}
	return lookup, metadata
}

func (h *Handler) decorateExtensionEmotesBatch(ctx context.Context, emotes []ExtensionEmote) []ExtensionEmote {
	if len(emotes) == 0 {
		return emotes
	}
	base := h.hostedEmoteCDNBase()
	lookup, metadata := h.lookupDecorationForExtensionEmotes(ctx, emotes)
	if base == "" && len(lookup) == 0 && len(metadata) == 0 {
		return emotes
	}
	return decorateExtensionEmotes(emotes, lookup, metadata, base)
}

func (h *Handler) decorateExtensionVodPulseEmotes(ctx context.Context, payload *ExtensionVodPulseResponse) {
	if h == nil || payload == nil {
		return
	}
	lookup, metadata := h.lookupDecorationForExtensionEmotes(ctx, collectExtensionVodPulseEmotes(payload))
	base := h.hostedEmoteCDNBase()
	if base == "" && len(lookup) == 0 && len(metadata) == 0 {
		return
	}
	payload.TopEmotes = decorateExtensionEmotes(payload.TopEmotes, lookup, metadata, base)
	if payload.Timeline != nil {
		for i := range payload.Timeline.Points {
			payload.Timeline.Points[i].TopEmotes = decorateExtensionEmotes(payload.Timeline.Points[i].TopEmotes, lookup, metadata, base)
		}
	}
	for i := range payload.TopMoments {
		payload.TopMoments[i].TopEmotes = decorateExtensionEmotes(payload.TopMoments[i].TopEmotes, lookup, metadata, base)
	}
	if payload.BestClipCandidate != nil {
		payload.BestClipCandidate.TopEmotes = decorateExtensionEmotes(payload.BestClipCandidate.TopEmotes, lookup, metadata, base)
	}
}

func collectExtensionVodPulseEmotes(payload *ExtensionVodPulseResponse) []ExtensionEmote {
	if payload == nil {
		return nil
	}
	out := make([]ExtensionEmote, 0, len(payload.TopEmotes))
	out = append(out, payload.TopEmotes...)
	if payload.Timeline != nil {
		for i := range payload.Timeline.Points {
			out = append(out, payload.Timeline.Points[i].TopEmotes...)
		}
	}
	for i := range payload.TopMoments {
		out = append(out, payload.TopMoments[i].TopEmotes...)
	}
	if payload.BestClipCandidate != nil {
		out = append(out, payload.BestClipCandidate.TopEmotes...)
	}
	return out
}

func (h *Handler) decorateHeatmapResponseEmotes(resp *heatmap.HeatmapResponse) {
	if h == nil || resp == nil {
		return
	}
	cdnBase := h.hostedEmoteCDNBase()
	if cdnBase == "" {
		return
	}
	for i := range resp.Points {
		resp.Points[i].TopEmotes = decorateHeatmapEmoteURLs(resp.Points[i].TopEmotes, cdnBase)
	}
}

func (h *Handler) decorateHeatmapDetailResponseEmotes(resp *heatmap.HeatmapDetailResponse) {
	if h == nil || resp == nil {
		return
	}
	cdnBase := h.hostedEmoteCDNBase()
	if cdnBase == "" {
		return
	}
	for i := range resp.Points {
		resp.Points[i].TopEmotes = decorateHeatmapEmoteURLs(resp.Points[i].TopEmotes, cdnBase)
	}
}

func decorateHeatmapEmoteURLs(emotes []heatmap.HeatmapEmote, cdnBase string) []heatmap.HeatmapEmote {
	if len(emotes) == 0 || strings.TrimSpace(cdnBase) == "" {
		return emotes
	}
	out := make([]heatmap.HeatmapEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		out[i].ImageURL = emoteimage.AbsolutizeHostedCDN(cdnBase, out[i].ImageURL)
	}
	return out
}

func (h *Handler) decoratePortalPeaks(ctx context.Context, peaks []PortalPeak) []PortalPeak {
	if h == nil || len(peaks) == 0 {
		return peaks
	}
	out := make([]PortalPeak, len(peaks))
	copy(out, peaks)
	for i := range out {
		if len(out[i].TopEmotes) == 0 {
			continue
		}
		out[i].TopEmotes = h.decorateExtensionEmotesBatch(ctx, out[i].TopEmotes)
	}
	return out
}

func portalMinuteTopEmotesFromTop(emotes []TopEmote) []PortalMinuteTopEmote {
	if len(emotes) == 0 {
		return nil
	}
	out := make([]PortalMinuteTopEmote, 0, len(emotes))
	for _, emote := range emotes {
		name := strings.TrimSpace(emote.Name)
		if name == "" {
			continue
		}
		out = append(out, PortalMinuteTopEmote{
			Name:     name,
			Provider: emote.Provider,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *Handler) enrichPortalMinuteTopEmotes(ctx context.Context, stream *StreamRecord, points []PortalMinutePoint, rollups []MinuteRollup) {
	if h == nil || stream == nil || len(points) == 0 || len(rollups) == 0 {
		return
	}
	rollupByOffset := make(map[int]MinuteRollup, len(rollups))
	for _, rollup := range rollups {
		offset := portalMinuteOffsetSeconds(stream.StartedAt, rollup)
		rollupByOffset[offset] = rollup
	}
	for i := range points {
		rollup, ok := rollupByOffset[points[i].OffsetSeconds]
		if !ok || len(rollup.Emotes) == 0 {
			continue
		}
		top := TopEmotesFromRollups([]MinuteRollup{rollup}, 3)
		top = h.rewriteHostedTopEmotes(ctx, top)
		points[i].TopEmotes = portalMinuteTopEmotesFromTop(top)
	}
}

func (h *Handler) decoratePortalChannelEmotes(ctx context.Context, emotes []PortalChannelEmote) []PortalChannelEmote {
	if len(emotes) == 0 {
		return emotes
	}
	cdnBase := h.hostedEmoteCDNBase()
	out := make([]PortalChannelEmote, len(emotes))
	copy(out, emotes)

	localIDs := make([]string, 0, len(emotes))
	for _, emote := range emotes {
		id := strings.TrimSpace(emote.ProviderEmoteID)
		if emoteimage.IsLocalEmoteID(id) {
			localIDs = append(localIDs, id)
		}
	}
	lookup := map[string]string{}
	if h != nil && h.store != nil && len(localIDs) > 0 {
		if resolved, err := h.store.LookupProviderEmoteIDs(ctx, localIDs); err == nil {
			lookup = resolved
		}
	}

	for i := range out {
		providerID := strings.TrimSpace(out[i].ProviderEmoteID)
		localID := ""
		upstream := ""
		if emoteimage.IsLocalEmoteID(providerID) {
			localID = providerID
			upstream = lookup[providerID]
		} else {
			upstream = providerID
		}
		out[i].ImageURL = emoteimage.HostedBrowserURL(cdnBase, out[i].Provider, localID, upstream)
	}
	return out
}
