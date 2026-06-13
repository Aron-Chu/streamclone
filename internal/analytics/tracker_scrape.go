package analytics

import (
	"context"
	"time"
)

const (
	ttMaxAgeRecentMS      = 15 * 60 * 1000        // live / <6h ended
	ttMaxAgeMidMS         = 6 * 60 * 60 * 1000    // 6–48h
	ttMaxAgeStaleMS       = 24 * 60 * 60 * 1000   // 48h–7d
	ttMaxAgeVeryStaleMS   = 7 * 24 * 60 * 60 * 1000 // >7d archived VODs
	ttDirectHTTPStaleAfter = 6 * time.Hour
)

func (s *SyncService) shouldSkipTracker(ctx context.Context, stream *StreamRecord) bool {
	if stream == nil || stream.ViewerSamples <= 0 {
		return false
	}
	rollups, err := s.store.RollupsByStream(ctx, stream.StreamID)
	if err != nil {
		s.log.Warn("skip tracker coverage check failed; using viewerSamples only", "stream_id", stream.StreamID, "err", err)
		return stream.ViewerSamples > 0
	}
	return hasGoodViewerCoverageFromRollups(rollups, stream)
}

func (s *SyncService) trackerScrapeMaxAgeMS(stream *StreamRecord, viewersOnly bool) int {
	if viewersOnly {
		return 0
	}
	if !s.passTTMaxAge {
		return 0
	}
	if s.ttMaxAgeMSDefault > 0 {
		return s.ttMaxAgeMSDefault
	}
	if stream == nil {
		return 0
	}
	endedAt := stream.EndedAt
	if endedAt == nil || endedAt.IsZero() {
		return ttMaxAgeRecentMS
	}
	staleMax := s.ttStaleMaxAgeMS
	if staleMax <= 0 {
		staleMax = ttMaxAgeVeryStaleMS
	}

	age := time.Since(*endedAt)
	switch {
	case age >= 7*24*time.Hour:
		return staleMax
	case age >= 48*time.Hour:
		return ttMaxAgeStaleMS
	case age >= 6*time.Hour:
		return ttMaxAgeMidMS
	default:
		return ttMaxAgeRecentMS
	}
}

func (s *SyncService) shouldTryDirectHTTP(stream *StreamRecord) bool {
	if s == nil || !s.ttDirectHTTPEnabled {
		return false
	}
	if !s.ttDirectHTTPStaleOnly {
		return true
	}
	if stream == nil || stream.EndedAt == nil || stream.EndedAt.IsZero() {
		return false
	}
	return time.Since(*stream.EndedAt) >= ttDirectHTTPStaleAfter
}
