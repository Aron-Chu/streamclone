package analytics

import (
	"fmt"
	"strings"
)

const GoldVODSegmentStrategyV1 = "gold-gql-v1"

type GoldVODSegmentPlan struct {
	SegmentKey         string
	VODID              string
	StreamID           string
	Login              string
	StrategyVersion    string
	StartOffsetSeconds int
	EndOffsetSeconds   int
}

func PlanGoldVODSegments(vodID, streamID, login string, durationSeconds, segmentSeconds int, strategyVersion string) []GoldVODSegmentPlan {
	vodID = strings.TrimSpace(vodID)
	if vodID == "" || durationSeconds <= 0 {
		return nil
	}
	if segmentSeconds <= 0 {
		segmentSeconds = 600
	}
	strategyVersion = normalizeGoldSegmentStrategy(strategyVersion)
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)

	segments := make([]GoldVODSegmentPlan, 0, (durationSeconds+segmentSeconds-1)/segmentSeconds)
	for start := 0; start < durationSeconds; start += segmentSeconds {
		end := start + segmentSeconds
		if end > durationSeconds {
			end = durationSeconds
		}
		segments = append(segments, GoldVODSegmentPlan{
			SegmentKey:         GoldVODSegmentKey(vodID, start, end, strategyVersion),
			VODID:              vodID,
			StreamID:           streamID,
			Login:              login,
			StrategyVersion:    strategyVersion,
			StartOffsetSeconds: start,
			EndOffsetSeconds:   end,
		})
	}
	return segments
}

func GoldVODSegmentKey(vodID string, startOffsetSeconds, endOffsetSeconds int, strategyVersion string) string {
	return fmt.Sprintf("%s:%d:%d:%s", strings.TrimSpace(vodID), startOffsetSeconds, endOffsetSeconds, normalizeGoldSegmentStrategy(strategyVersion))
}

func normalizeGoldSegmentStrategy(strategyVersion string) string {
	strategyVersion = strings.TrimSpace(strategyVersion)
	if strategyVersion == "" {
		return GoldVODSegmentStrategyV1
	}
	return strategyVersion
}
