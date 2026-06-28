package analytics

import (
	"sort"
)

const defaultIVRPeaksMaxMinutes = 5
const defaultIVRPeaksMinChatCount = 10

// selectIVRPeakRollups keeps only top-N chat peak minutes for peaks-only provisional writes.
// Emotes are stripped — peaks-only mode does not claim provider-accurate emote scoring.
func selectIVRPeakRollups(rollups []MinuteRollup, maxPeaks, minChat int) []MinuteRollup {
	if maxPeaks <= 0 {
		maxPeaks = defaultIVRPeaksMaxMinutes
	}
	if minChat <= 0 {
		minChat = defaultIVRPeaksMinChatCount
	}
	candidates := make([]MinuteRollup, 0, len(rollups))
	for _, r := range rollups {
		if r.ChatCount < minChat {
			continue
		}
		candidates = append(candidates, r)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ChatCount == candidates[j].ChatCount {
			return candidates[i].MinuteTS.Before(candidates[j].MinuteTS)
		}
		return candidates[i].ChatCount > candidates[j].ChatCount
	})
	if len(candidates) > maxPeaks {
		candidates = candidates[:maxPeaks]
	}
	out := make([]MinuteRollup, 0, len(candidates))
	for _, r := range candidates {
		r.TotalEmoteCount = 0
		r.SevenTVEmoteCount = 0
		r.Emotes = map[string]int{}
		r.ChatSource = RollupChatSourceIVR
		r.SourceConfidence = SourceConfidenceProvisional
		r.ChatSourceDetail = RollupDetailIVRPeaksOnly
		out = append(out, r)
	}
	return out
}

func peakMinuteTimestamps(rollups []MinuteRollup) []string {
	out := make([]string, 0, len(rollups))
	for _, r := range rollups {
		if r.ChatCount <= 0 {
			continue
		}
		out = append(out, r.MinuteTS.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return out
}
