package analytics

import (
	"context"
	"strings"
)

type vodGameCategoryLookup interface {
	VideoGameName(ctx context.Context, videoID string) (string, error)
}

// resolveSyncGameSegments prefers TwitchTracker segments and falls back to a single
// Helix/DB category segment when TT returned no game timeline.
func (s *SyncService) resolveSyncGameSegments(
	ctx context.Context,
	streamID, vodID, category string,
	durationSeconds int,
	ttGames []scrapedGame,
) []scrapedGame {
	if len(ttGames) > 0 || durationSeconds <= 0 {
		return ttGames
	}
	streamCategory := ""
	if s != nil && s.store != nil && strings.TrimSpace(streamID) != "" {
		if stream, err := s.store.StreamByID(ctx, streamID); err == nil && stream != nil {
			streamCategory = stream.Category
		}
	}
	var lookup vodGameCategoryLookup
	if s != nil && s.helix != nil {
		lookup = s.helix
	}
	gameName := resolveGameCategoryFallback(ctx, category, streamCategory, vodID, lookup)
	if gameName == "" {
		return nil
	}
	return []scrapedGame{{Title: gameName}}
}

func resolveGameCategoryFallback(ctx context.Context, category, streamCategory, vodID string, lookup vodGameCategoryLookup) string {
	gameName := normalizeSyncGameCategory(category)
	if gameName == "" {
		gameName = normalizeSyncGameCategory(streamCategory)
	}
	if gameName == "" && lookup != nil && strings.TrimSpace(vodID) != "" {
		if helixGame, err := lookup.VideoGameName(ctx, vodID); err == nil {
			gameName = normalizeSyncGameCategory(helixGame)
		}
	}
	return gameName
}

func fallbackGameSegmentsForStream(stream *StreamRecord) []GameSegment {
	if stream == nil || stream.EndedAt == nil {
		return nil
	}
	gameName := normalizeSyncGameCategory(stream.Category)
	durationSeconds := streamDurationSeconds(stream, nil)
	if gameName == "" || durationSeconds <= 0 {
		return nil
	}
	return []GameSegment{{
		StreamID:        stream.StreamID,
		GameName:        gameName,
		OffsetSeconds:   0,
		DurationSeconds: durationSeconds,
	}}
}

func normalizeSyncGameCategory(category string) string {
	category = strings.TrimSpace(category)
	switch strings.ToLower(category) {
	case "", "live", "syncing...", "syncing…":
		return ""
	default:
		return category
	}
}
