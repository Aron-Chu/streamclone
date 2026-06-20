package analytics

import (
	"context"
	"sync"
	"time"
)

type trackerPrefetchState struct {
	mu      sync.Mutex
	inflight map[string]struct{}
}

func newTrackerPrefetchState() *trackerPrefetchState {
	return &trackerPrefetchState{inflight: make(map[string]struct{})}
}

func (p *trackerPrefetchState) tryStart(streamID string) bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.inflight[streamID]; ok {
		return false
	}
	p.inflight[streamID] = struct{}{}
	return true
}

func (p *trackerPrefetchState) finish(streamID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	delete(p.inflight, streamID)
	p.mu.Unlock()
}

// PrefetchTracker warms the scraper cache for a TwitchTracker stream detail page.
// It is fire-and-forget and skips streams that already have good viewer coverage.
func (s *SyncService) PrefetchTracker(login, streamID string) (queued bool) {
	if s == nil || !s.ttPrefetchEnabled || login == "" || streamID == "" || s.scraperKey == "" {
		return false
	}
	if !s.trackerPrefetch.tryStart(streamID) {
		return false
	}

	go func() {
		defer s.trackerPrefetch.finish(streamID)

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.trackerScrapeTimeoutMS)*time.Millisecond+30*time.Second)
		defer cancel()

		stream, err := s.store.StreamByID(ctx, streamID)
		if err == nil && s.shouldSkipTracker(ctx, stream) {
			s.log.Debug("tracker prefetch skipped; viewer coverage already good", "stream_id", streamID)
			return
		}

		url := "https://twitchtracker.com/" + login + "/streams/" + streamID
		if _, err := s.scrapeTwitchTrackerCoalesced(ctx, streamID, url, stream, false, s.trackerScrapeTimeoutMS); err != nil {
			s.log.Debug("tracker prefetch scrape failed", "stream_id", streamID, "err", err)
			return
		}
		s.log.Info("tracker prefetch warmed scraper cache", "stream_id", streamID, "login", login)
	}()

	return true
}
