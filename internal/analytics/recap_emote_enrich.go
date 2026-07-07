package analytics

import (
	"context"
	"strings"

	pulserecap "streamclone/internal/analytics/recap"
)

// Scan enough rollup emotes to build a 7TV catalog for recap rows. Do not take
// the global top-N before filtering — recap top 7TV codes are often outside the
// overall top-N when Twitch emotes dominate total usage.
const recapEmoteCatalogScanLimit = 500

func (h *Handler) enrichRecapTopEmotes(ctx context.Context, recap *pulserecap.StreamRecap, storeRollups []MinuteRollup, streamID, twitchID string) {
	if recap == nil {
		return
	}
	codes := recapEmoteCodesFromRecap(recap)
	if len(codes) == 0 {
		recap.EmoteEnrichmentStatus = ""
		return
	}
	lookupCodes := recapEmoteCodes(codes)
	rollupCatalog := filterTopEmotesSevenTV(TopEmotesFromRollups(storeRollups, recapEmoteCatalogScanLimit))
	var historyCatalog, snapshotCatalog []TopEmote
	if h != nil && h.store != nil && strings.TrimSpace(streamID) != "" {
		if rows, err := h.store.RecapEmoteCatalogFromStreamHistory(ctx, streamID, lookupCodes); err == nil {
			historyCatalog = rows
		}
		if strings.TrimSpace(twitchID) != "" {
			if rows, err := h.store.RecapEmoteCatalogFromChannelSnapshots(ctx, twitchID, lookupCodes); err == nil {
				snapshotCatalog = rows
			}
		}
	}
	catalog := mergeRecapEmoteCatalogs(rollupCatalog, historyCatalog, snapshotCatalog)
	if h != nil {
		catalog = h.rewriteHostedTopEmotes(ctx, catalog)
	}
	recap.TopEmotes = enrichRecapEmotes(recap.TopEmotes, catalog)
	for i := range recap.TopMoments {
		recap.TopMoments[i].TopEmotes = enrichRecapEmotes(recap.TopMoments[i].TopEmotes, catalog)
	}
	for i := range recap.ClipCandidates {
		recap.ClipCandidates[i].TopEmotes = enrichRecapEmotes(recap.ClipCandidates[i].TopEmotes, catalog)
	}
	recap.EmoteEnrichmentStatus = computeEmoteEnrichmentStatus(recap.TopEmotes)
}

func recapEmoteCodesFromRecap(recap *pulserecap.StreamRecap) []string {
	if recap == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var codes []string
	add := func(emotes []pulserecap.Emote) {
		for _, emote := range emotes {
			code := strings.TrimSpace(emote.Code)
			if code == "" {
				continue
			}
			key := strings.ToLower(code)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			codes = append(codes, code)
		}
	}
	add(recap.TopEmotes)
	for i := range recap.TopMoments {
		add(recap.TopMoments[i].TopEmotes)
	}
	for i := range recap.ClipCandidates {
		add(recap.ClipCandidates[i].TopEmotes)
	}
	return codes
}

func enrichRecapEmotes(recapEmotes []pulserecap.Emote, catalog []TopEmote) []pulserecap.Emote {
	if len(recapEmotes) == 0 {
		return recapEmotes
	}
	byName := make(map[string]TopEmote, len(catalog))
	for _, item := range catalog {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" {
			continue
		}
		byName[key] = item
	}
	out := make([]pulserecap.Emote, len(recapEmotes))
	copy(out, recapEmotes)
	for i, emote := range out {
		count := emote.Count
		key := strings.ToLower(strings.TrimSpace(emote.Code))
		if cat, ok := byName[key]; ok {
			out[i].ID = strings.TrimSpace(cat.ID)
			out[i].ImageURL = strings.TrimSpace(cat.ImageURL)
			if strings.TrimSpace(out[i].Provider) == "" {
				out[i].Provider = cat.Provider
			}
		}
		out[i].Count = count
	}
	return out
}

func computeEmoteEnrichmentStatus(emotes []pulserecap.Emote) string {
	if len(emotes) == 0 {
		return ""
	}
	resolved := 0
	for _, emote := range emotes {
		if recapEmoteHasImageMetadata(emote) {
			resolved++
		}
	}
	if resolved == 0 {
		return "missing"
	}
	if resolved >= len(emotes) {
		return "complete"
	}
	return "partial"
}

func recapEmoteHasImageMetadata(emote pulserecap.Emote) bool {
	return strings.TrimSpace(emote.ImageURL) != "" || strings.TrimSpace(emote.ID) != ""
}

func filterTopEmotesSevenTV(emotes []TopEmote) []TopEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := make([]TopEmote, 0, len(emotes))
	for _, emote := range emotes {
		if isRecapSevenTVProvider(emote.Provider) {
			out = append(out, emote)
		}
	}
	return out
}

func isRecapSevenTVProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "seventv", "7tv":
		return true
	default:
		return false
	}
}
