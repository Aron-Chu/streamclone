package recap

import (
	"sort"
	"strings"
	"time"

	"streamclone/internal/analytics/heatmap"
)

const (
	maxTopMoments     = 10
	maxTopEmotes      = 10
	maxClipCandidates = 5
	clipScoreCutoff   = 70
)

type Input struct {
	StreamID        string
	Login           string
	VodID           *string
	StartedAt       time.Time
	DurationSeconds int
	Rollups         []heatmap.MinuteRollup
	Points          []heatmap.ReplayHeatmapDetailPoint
}

type StreamRecap struct {
	StreamID           string      `json:"streamId"`
	Login              string      `json:"login"`
	VodID              *string     `json:"vodId,omitempty"`
	DurationSeconds    int         `json:"durationSeconds"`
	TotalMessages      int         `json:"totalMessages"`
	PeakChatPerMin     int         `json:"peakChatPerMin"`
	TopMoments         []Moment    `json:"topMoments"`
	TopEmotes          []Emote     `json:"topEmotes"`
	BiggestChatSpike   *ChatSpike  `json:"biggestChatSpike,omitempty"`
	FunniestEmoteBurst *EmoteBurst `json:"funniestEmoteBurst,omitempty"`
	ClipCandidates     []Moment    `json:"clipCandidates"`
}

type Moment struct {
	OffsetSeconds int      `json:"offsetSeconds"`
	Score         int      `json:"score"`
	Reasons       []string `json:"reasons"`
	TopEmotes     []Emote  `json:"topEmotes,omitempty"`
}

type Emote struct {
	Code     string `json:"code"`
	Count    int    `json:"count"`
	Provider string `json:"provider,omitempty"`
}

type ChatSpike struct {
	OffsetSeconds int `json:"offsetSeconds"`
	ChatPerMin    int `json:"chatPerMin"`
}

type EmoteBurst struct {
	OffsetSeconds int    `json:"offsetSeconds"`
	Code          string `json:"code,omitempty"`
	Count         int    `json:"count"`
}

func Build(input Input) StreamRecap {
	recap := StreamRecap{
		StreamID:        input.StreamID,
		Login:           strings.ToLower(strings.TrimSpace(input.Login)),
		VodID:           input.VodID,
		DurationSeconds: input.DurationSeconds,
		TopMoments:      topMoments(input.Points),
		TopEmotes:       topSevenTVEmotes(input.Rollups),
	}
	if recap.DurationSeconds <= 0 {
		recap.DurationSeconds = len(input.Rollups) * 60
	}
	recap.ClipCandidates = clipCandidates(recap.TopMoments)
	recap.TotalMessages, recap.PeakChatPerMin, recap.BiggestChatSpike, recap.FunniestEmoteBurst = summarizeRollups(input.Rollups, input.StartedAt)
	return recap
}

func topMoments(points []heatmap.ReplayHeatmapDetailPoint) []Moment {
	items := make([]Moment, 0, len(points))
	for _, point := range points {
		if point.Score <= 0 {
			continue
		}
		items = append(items, momentFromPoint(point))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return items[i].OffsetSeconds < items[j].OffsetSeconds
	})
	if len(items) > maxTopMoments {
		items = items[:maxTopMoments]
	}
	return items
}

func momentFromPoint(point heatmap.ReplayHeatmapDetailPoint) Moment {
	reason := strings.TrimSpace(point.Reason)
	if reason == "" {
		reason = heatmap.ReasonManual
	}
	return Moment{
		OffsetSeconds: point.OffsetSeconds,
		Score:         point.Score,
		Reasons:       []string{reason},
		TopEmotes:     convertHeatmapEmotes(point.TopEmotes),
	}
}

func convertHeatmapEmotes(in []heatmap.HeatmapEmote) []Emote {
	if len(in) == 0 {
		return nil
	}
	out := make([]Emote, 0, len(in))
	for _, item := range in {
		out = append(out, Emote{
			Code:     item.Name,
			Count:    item.Count,
			Provider: item.Provider,
		})
	}
	return out
}

func clipCandidates(top []Moment) []Moment {
	out := make([]Moment, 0, maxClipCandidates)
	for _, item := range top {
		if item.Score >= clipScoreCutoff {
			out = append(out, item)
		}
		if len(out) == maxClipCandidates {
			return out
		}
	}
	for _, item := range top {
		if len(out) == maxClipCandidates {
			break
		}
		if item.Score < clipScoreCutoff {
			out = append(out, item)
		}
	}
	return out
}

func summarizeRollups(rollups []heatmap.MinuteRollup, startedAt time.Time) (totalMessages int, peakChat int, biggest *ChatSpike, burst *EmoteBurst) {
	for i, rollup := range rollups {
		if rollup.Missing {
			continue
		}
		offset := offsetForRollup(i, rollup, startedAt)
		totalMessages += rollup.ChatCount
		if rollup.ChatCount > peakChat {
			peakChat = rollup.ChatCount
			biggest = &ChatSpike{OffsetSeconds: offset, ChatPerMin: rollup.ChatCount}
		}
		code, count := topSevenTVEmoteForRollup(rollup)
		if count == 0 && rollup.SevenTVEmoteCount > 0 {
			count = rollup.SevenTVEmoteCount
		}
		if count > 0 && (burst == nil || count > burst.Count || (count == burst.Count && offset < burst.OffsetSeconds)) {
			burst = &EmoteBurst{OffsetSeconds: offset, Code: code, Count: count}
		}
	}
	return totalMessages, peakChat, biggest, burst
}

func offsetForRollup(index int, rollup heatmap.MinuteRollup, startedAt time.Time) int {
	if !startedAt.IsZero() && !rollup.MinuteTS.IsZero() {
		offset := int(rollup.MinuteTS.Sub(startedAt.UTC().Truncate(time.Minute)).Seconds())
		if offset >= 0 {
			return offset
		}
	}
	return index * 60
}

func topSevenTVEmotes(rollups []heatmap.MinuteRollup) []Emote {
	counts := map[string]int{}
	for _, rollup := range rollups {
		if rollup.Missing {
			continue
		}
		for key, count := range rollup.Emotes {
			provider, _, name := splitEmoteKey(key)
			if !isSevenTVProvider(provider) || count <= 0 {
				continue
			}
			counts[name] += count
		}
	}
	if len(counts) == 0 {
		return []Emote{}
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > maxTopEmotes {
		names = names[:maxTopEmotes]
	}
	out := make([]Emote, 0, len(names))
	for _, name := range names {
		out = append(out, Emote{Code: name, Count: counts[name], Provider: "seventv"})
	}
	return out
}

func topSevenTVEmoteForRollup(rollup heatmap.MinuteRollup) (string, int) {
	bestName := ""
	bestCount := 0
	for key, count := range rollup.Emotes {
		provider, _, name := splitEmoteKey(key)
		if !isSevenTVProvider(provider) || count <= 0 {
			continue
		}
		if count > bestCount || (count == bestCount && name < bestName) {
			bestName = name
			bestCount = count
		}
	}
	return bestName, bestCount
}

func splitEmoteKey(key string) (provider, id, name string) {
	parts := strings.SplitN(key, ":", 3)
	switch len(parts) {
	case 3:
		return strings.ToLower(parts[0]), parts[1], parts[2]
	case 2:
		return strings.ToLower(parts[0]), parts[1], parts[1]
	default:
		return "", "", key
	}
}

func isSevenTVProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "seventv", "7tv":
		return true
	default:
		return false
	}
}
