package analytics

import (
	"context"
	"sync"
)

type trackerScrapeCoalesce struct {
	mu       sync.Mutex
	inflight map[string]*trackerScrapeFlight
}

type trackerScrapeFlight struct {
	done chan struct{}
	html string
	err  error
}

func newTrackerScrapeCoalesce() *trackerScrapeCoalesce {
	return &trackerScrapeCoalesce{inflight: make(map[string]*trackerScrapeFlight)}
}

func (c *trackerScrapeCoalesce) do(ctx context.Context, key string, scrape func(context.Context) (string, error)) (string, error) {
	if c == nil || key == "" {
		return scrape(ctx)
	}

	c.mu.Lock()
	if flight, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-flight.done:
			return flight.html, flight.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	flight := &trackerScrapeFlight{done: make(chan struct{})}
	c.inflight[key] = flight
	c.mu.Unlock()

	flight.html, flight.err = scrape(ctx)
	close(flight.done)

	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	return flight.html, flight.err
}

func trackerScrapeCoalesceKey(url string, stream *StreamRecord) string {
	if stream != nil && stream.StreamID != "" {
		return stream.StreamID
	}
	return url
}
