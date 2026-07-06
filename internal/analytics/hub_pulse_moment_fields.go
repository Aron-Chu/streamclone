package analytics

import (
	"strconv"
	"strings"
)

// normalizeHubPulseMomentFields applies shared post-build normalization for
// live and historical hub pulse rows. Source-specific builders retain coverage,
// VOD state, labels, and activity tags.
func normalizeHubPulseMomentFields(moment *HubLivePulseMoment) {
	if moment == nil {
		return
	}
	moment.Login = normalizeLogin(moment.Login)
	moment.DisplayName = strings.TrimSpace(moment.DisplayName)
	if moment.DisplayName == "" {
		moment.DisplayName = moment.Login
	}
	moment.Category = strings.TrimSpace(moment.Category)

	if moment.EmotesPerMin <= 0 {
		if sum := hubPulseMomentTopEmoteSum(moment.TopEmotes); sum > 0 {
			moment.EmotesPerMin = sum
		}
	}

	if moment.TopEmoteCode == "" {
		moment.TopEmoteCode = peakTopEmoteCode(PortalPeak{TopEmotes: hubExtensionEmotesFromHub(moment.TopEmotes)})
	}
}

func hubPulseMomentTopEmoteSum(top []HubEmote) int {
	sum := 0
	for _, emote := range top {
		sum += emote.Count
	}
	return sum
}

func hubExtensionEmotesFromHub(top []HubEmote) []ExtensionEmote {
	if len(top) == 0 {
		return nil
	}
	out := make([]ExtensionEmote, 0, len(top))
	for _, emote := range top {
		out = append(out, ExtensionEmote{
			Name:     emote.Name,
			Provider: emote.Provider,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
		})
	}
	return out
}

func historicalCandidateEmotesPerMin(cand hubHistoricalMinuteCandidate) int {
	if cand.TotalEmoteCount > 0 {
		return cand.TotalEmoteCount
	}
	return cand.SevenTVEmoteCount
}

func hubPulseMomentsInTimeRange(moments []HubLivePulseMoment, startMs, endMs int64) []HubLivePulseMoment {
	if len(moments) == 0 || endMs <= startMs {
		return nil
	}
	out := make([]HubLivePulseMoment, 0, len(moments))
	for _, moment := range moments {
		at := moment.At
		if at <= 0 {
			continue
		}
		if at >= startMs && at < endMs {
			out = append(out, moment)
		}
	}
	return out
}

func mergeHubPulseMoments(corpus, live []HubLivePulseMoment, limit int) ([]HubLivePulseMoment, string) {
	if limit <= 0 {
		limit = hubHistoricalMomentsCap
	}
	seen := make(map[string]struct{}, limit*2)
	out := make([]HubLivePulseMoment, 0, limit)
	add := func(moment HubLivePulseMoment) {
		if len(out) >= limit {
			return
		}
		key := hubPulseMomentDedupeKey(moment)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, moment)
	}
	for _, moment := range live {
		add(moment)
	}
	for _, moment := range corpus {
		add(moment)
	}
	sortHubHistoricalMoments(out)
	if len(out) > limit {
		out = out[:limit]
	}
	source := "corpus_historical"
	if len(corpus) > 0 && len(live) > 0 {
		source = "bucket_merged"
	} else if len(live) > 0 && len(corpus) == 0 {
		source = "live_irc"
	}
	return out, source
}

func hubPulseMomentDedupeKey(moment HubLivePulseMoment) string {
	login := normalizeLogin(moment.Login)
	if login == "" {
		return ""
	}
	streamID := strings.TrimSpace(moment.StreamID)
	if moment.At > 0 {
		return login + "|" + streamID + "|" + strconv.FormatInt(moment.At, 10)
	}
	return login + "|" + streamID + "|" + strconv.Itoa(moment.OffsetSeconds)
}
