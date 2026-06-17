package netmeter

import (
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	OpTracker = "tracker"
	OpGQL     = "gql"
	OpEmote   = "emote"
	OpHelix   = "helix"
)

// Snapshot holds cumulative byte totals and the rate since the previous snapshot.
type Snapshot struct {
	TrackerScrapeBytes int64   `json:"trackerScrapeBytes"`
	GQLFetchBytes      int64   `json:"gqlFetchBytes"`
	EmotePreloadBytes  int64   `json:"emotePreloadBytes"`
	HelixBytes         int64   `json:"helixBytes"`
	TotalBytes         int64   `json:"totalBytes"`
	LastRateBps        float64 `json:"lastRateBps"`
}

// Meter counts outbound sync network usage by operation.
type Meter struct {
	mu sync.Mutex

	tracker int64
	gql     int64
	emote   int64
	helix   int64

	lastSnap    time.Time
	lastTotal   int64
	lastRateBps float64

	onRecord func(op string, n int64)
}

func NewMeter(onRecord func(op string, n int64)) *Meter {
	return &Meter{
		lastSnap: time.Now(),
		onRecord: onRecord,
	}
}

func (m *Meter) Record(op string, n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.mu.Lock()
	switch op {
	case OpTracker:
		m.tracker += n
	case OpGQL:
		m.gql += n
	case OpEmote:
		m.emote += n
	case OpHelix:
		m.helix += n
	}
	m.mu.Unlock()
	if m.onRecord != nil {
		m.onRecord(op, n)
	}
}

func (m *Meter) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	total := m.tracker + m.gql + m.emote + m.helix
	rate := m.lastRateBps
	if !m.lastSnap.IsZero() {
		if elapsed := now.Sub(m.lastSnap).Seconds(); elapsed > 0 {
			rate = float64(total-m.lastTotal) * 8 / elapsed
		}
	}
	m.lastRateBps = rate
	m.lastSnap = now
	m.lastTotal = total

	return Snapshot{
		TrackerScrapeBytes: m.tracker,
		GQLFetchBytes:      m.gql,
		EmotePreloadBytes:  m.emote,
		HelixBytes:         m.helix,
		TotalBytes:         total,
		LastRateBps:        rate,
	}
}

// CountingTransport wraps an http.RoundTripper and counts response body bytes.
type CountingTransport struct {
	Base  http.RoundTripper
	Meter *Meter
	Op    string
}

func (t *CountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || t.Meter == nil {
		return resp, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, meter: t.Meter, op: t.Op}
	return resp, err
}

type countingBody struct {
	io.ReadCloser
	meter *Meter
	op    string
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.meter.Record(b.op, int64(n))
	}
	return n, err
}

func NewCountingTransport(base http.RoundTripper, meter *Meter, op string) http.RoundTripper {
	return &CountingTransport{Base: base, Meter: meter, Op: op}
}
