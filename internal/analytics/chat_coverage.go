package analytics

import (
	"context"
	"fmt"
	"strings"
)

// ChatCoverageSummary describes how much of the stream timeline has chat rollups.
type ChatCoverageSummary struct {
	ChatSpanMinutes   int     `json:"chatSpanMinutes"`
	StreamSpanMinutes int     `json:"streamSpanMinutes"`
	CoveragePct       float64 `json:"coveragePct"`
	Partial           bool    `json:"partial"`
	VodDurationSec    int     `json:"vodDurationSec,omitempty"`
}

const chatCoveragePartialThreshold = 0.35

// chatCoverageSummary computes chat span vs stream span for UI warnings and sync messages.
func chatCoverageSummary(rollups []MinuteRollup, stream *StreamRecord, vodDurationSec int) ChatCoverageSummary {
	return chatCoverageSummaryFromRollups(filterTimelineRollups(rollups), stream, vodDurationSec)
}

func chatCoverageSummaryFromRollups(rollups []MinuteRollup, stream *StreamRecord, vodDurationSec int) ChatCoverageSummary {
	out := ChatCoverageSummary{VodDurationSec: vodDurationSec}
	if len(rollups) == 0 {
		return out
	}

	out.StreamSpanMinutes = len(rollups)
	if stream != nil && stream.EndedAt != nil && !stream.StartedAt.IsZero() {
		if d := int(stream.EndedAt.Sub(stream.StartedAt).Minutes()); d > 0 {
			out.StreamSpanMinutes = d
		}
	}

	firstIdx, lastIdx := -1, -1
	for i, rollup := range rollups {
		if rollup.ChatCount <= 0 && rollup.SevenTVEmoteCount <= 0 {
			continue
		}
		if firstIdx < 0 {
			firstIdx = i
		}
		lastIdx = i
	}
	if firstIdx < 0 {
		return out
	}

	out.ChatSpanMinutes = lastIdx - firstIdx + 1
	if out.StreamSpanMinutes > 0 {
		out.CoveragePct = float64(out.ChatSpanMinutes) / float64(out.StreamSpanMinutes) * 100
	}

	streamDurationSec := out.StreamSpanMinutes * 60
	if streamDurationSec <= 0 {
		streamDurationSec = len(rollups) * 60
	}
	if out.CoveragePct < chatCoveragePartialThreshold*100 {
		out.Partial = true
	}
	if vodDurationSec > 0 && streamDurationSec > 0 && vodDurationSec < streamDurationSec/2 {
		out.Partial = true
	}
	return out
}

func formatPartialChatCoverageMessage(vodID string, summary ChatCoverageSummary) string {
	vodLabel := strings.TrimSpace(vodID)
	if vodLabel == "" {
		vodLabel = "VOD"
	} else {
		vodLabel = "VOD " + vodLabel
	}
	chatClock := formatVodClock(summary.ChatSpanMinutes * 60)
	streamClock := formatVodClock(summary.StreamSpanMinutes * 60)
	return fmt.Sprintf(
		"Chat synced for first %s of %s stream (%s). Twitch archive may still be processing — re-sync later.",
		chatClock,
		streamClock,
		vodLabel,
	)
}

// hasGoodChatCoverageFromRollups returns true when persisted minute rollups look
// complete enough to skip a full VOD GQL re-fetch on chat-only resync.
func hasGoodChatCoverageFromRollups(rollups []MinuteRollup, stream *StreamRecord) bool {
	if stream == nil || stream.ChatMessages <= 0 || len(rollups) < 10 {
		return false
	}

	var totalChat int64
	firstIdx, lastIdx := -1, -1
	for i, rollup := range rollups {
		if rollup.ChatCount <= 0 && rollup.SevenTVEmoteCount <= 0 {
			continue
		}
		totalChat += int64(rollup.ChatCount)
		if firstIdx < 0 {
			firstIdx = i
		}
		lastIdx = i
	}
	if firstIdx < 0 || totalChat <= 0 {
		return false
	}

	chatSpan := lastIdx - firstIdx + 1
	if float64(chatSpan)/float64(len(rollups)) < chatCoveragePartialThreshold {
		return false
	}

	if stream.ChatMessages > 0 {
		ratio := float64(totalChat) / float64(stream.ChatMessages)
		if ratio < 0.85 || ratio > 1.15 {
			return false
		}
	}

	return true
}

func (s *SyncService) shouldSkipVODChat(ctx context.Context, stream *StreamRecord, vodID string) bool {
	if stream == nil || strings.TrimSpace(vodID) == "" {
		return false
	}
	if strings.TrimSpace(stream.VodID) != "" && strings.TrimSpace(stream.VodID) != strings.TrimSpace(vodID) {
		return false
	}
	if cp, err := s.store.GetSyncCheckpoint(ctx, stream.StreamID, vodID); err == nil && cp != nil {
		return false
	}
	rollups, err := s.store.RollupsByStream(ctx, stream.StreamID)
	if err != nil {
		s.log.Warn("skip vod chat coverage check failed", "stream_id", stream.StreamID, "err", err)
		return stream.ChatMessages > 0
	}
	return hasGoodChatCoverageFromRollups(rollups, stream)
}

func countChatMinutesInMap(commentsMap map[int][]string) int {
	n := 0
	for _, comments := range commentsMap {
		if len(comments) > 0 {
			n++
		}
	}
	return n
}
