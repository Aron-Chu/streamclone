package analytics

import (
	"context"
	"strings"
	"time"
)

// recordLiveCategorySample writes a category timeline tick for IRC-collected live
// streams. Samples power GameSegmentsFromTop500Snapshots for all admitted channels.
func (s *Store) recordLiveCategorySample(
	ctx context.Context,
	stream LiveStream,
	profile UserProfile,
	seenAt time.Time,
	previousCategory string,
) error {
	if s == nil || s.db == nil {
		return nil
	}
	category := normalizeSyncGameCategory(stream.GameName)
	if category == "" || strings.TrimSpace(stream.ID) == "" || normalizeLogin(stream.Login) == "" {
		return nil
	}
	channelID := strings.TrimSpace(stream.BroadcasterID)
	if channelID == "" {
		channelID = strings.TrimSpace(profile.ID)
	}
	if channelID == "" || channelID == "pending" {
		return nil
	}

	seenAt = seenAt.UTC()
	tick := seenAt.Truncate(time.Minute)
	prev := normalizeSyncGameCategory(previousCategory)
	categoryChanged := prev != category
	if !categoryChanged {
		var exists bool
		if err := s.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM top500_live_snapshots
				WHERE channel_id = $1 AND sample_tick_at = $2 AND source = $3
			)`, channelID, tick, Top500SnapshotSourceIRCCollector).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
	}

	streamID := strings.TrimSpace(stream.ID)
	startedAt := stream.StartedAt.UTC()
	if startedAt.IsZero() {
		startedAt = seenAt
	}
	viewers := stream.ViewerCount
	snapshot := Top500LiveSnapshot{
		ChannelID:    channelID,
		Login:        normalizeLogin(stream.Login),
		StreamID:     &streamID,
		IsLive:       true,
		Title:        strings.TrimSpace(stream.Title),
		CategoryName: category,
		StartedAt:    &startedAt,
		ViewerCount:  &viewers,
		Language:     strings.TrimSpace(stream.Language),
		Tags:         stream.Tags,
		SampleTickAt: tick,
		SampledAt:    seenAt,
		Source:       Top500SnapshotSourceIRCCollector,
	}
	return s.UpsertTop500LiveSnapshot(ctx, snapshot)
}

// gameCategoryAtOffset returns the game/category name active at offsetSeconds.
func gameCategoryAtOffset(segments []GameSegment, offsetSeconds int) string {
	if offsetSeconds < 0 || len(segments) == 0 {
		return ""
	}
	for _, seg := range segments {
		end := seg.OffsetSeconds + seg.DurationSeconds
		if seg.DurationSeconds <= 0 {
			end = seg.OffsetSeconds + 1
		}
		if seg.OffsetSeconds <= offsetSeconds && offsetSeconds < end {
			return normalizeSyncGameCategory(seg.GameName)
		}
	}
	last := segments[len(segments)-1]
	if offsetSeconds >= last.OffsetSeconds {
		return normalizeSyncGameCategory(last.GameName)
	}
	return ""
}

// dominantCategoryFromSegments picks the longest-duration segment name for stream row display.
func dominantCategoryFromSegments(segments []GameSegment, fallback string) string {
	if len(segments) == 0 {
		return normalizeSyncGameCategory(fallback)
	}
	best := ""
	bestDur := -1
	for _, seg := range segments {
		name := normalizeSyncGameCategory(seg.GameName)
		if name == "" {
			continue
		}
		dur := seg.DurationSeconds
		if dur <= 0 {
			dur = 1
		}
		if dur > bestDur {
			bestDur = dur
			best = name
		}
	}
	if best != "" {
		return best
	}
	return normalizeSyncGameCategory(fallback)
}

// gamesSummaryFromSegments builds a compact multi-game label for portal sidebars.
func gamesSummaryFromSegments(segments []GameSegment, fallback string) string {
	if len(segments) == 0 {
		if cat := normalizeSyncGameCategory(fallback); cat != "" {
			return cat
		}
		return ""
	}
	seen := make(map[string]struct{}, len(segments))
	names := make([]string, 0, len(segments))
	for _, seg := range segments {
		name := normalizeSyncGameCategory(seg.GameName)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return normalizeSyncGameCategory(fallback)
	}
	return strings.Join(names, " · ")
}
