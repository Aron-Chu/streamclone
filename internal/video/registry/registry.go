package registry

import (
	"sync"
	"sync/atomic"
	"time"

	"streamclone/internal/video/usher"
)

type Streamer interface {
	Wait() error
	Kill()
}

type StartupBreakdown struct {
	UpstreamFetchMs int64  `json:"upstreamFetchMs,omitempty"`
	WorkerSpawnMs   int64  `json:"workerSpawnMs,omitempty"`
	HLSProbeBudgetMs int64 `json:"hlsProbeBudgetMs,omitempty"`
	HLSReadyMs      int64  `json:"hlsReadyMs,omitempty"`
	TotalMs         int64  `json:"totalMs,omitempty"`
	Backend         string `json:"backend,omitempty"`
}

type Session struct {
	Channel           string
	Quality           string
	HLSURL            string
	Renditions        []usher.Rendition
	SelectedRendition *usher.Rendition
	WorkerBackend     string
	StartupMs         int64
	StartupBreakdown  StartupBreakdown
	FallbackAttempted bool
	FallbackAttempts  int
	LastStartError    string
	QualityRestarted  bool
	LatencyMode       string
	LiveEdge          int
	VodID             string
	OffsetSeconds     int
	SeekSeconds       int
	StartedAt         time.Time

	listeners atomic.Int64
	lastSeen  atomic.Int64
	stopped   atomic.Bool
	released  atomic.Bool
	restarts  atomic.Int64

	mu              sync.Mutex
	stream          Streamer
	listenerIDs     map[string]struct{}
	workerStartedAt time.Time
	lastRestartAt   time.Time
	lastWorkerError string
}

func (s *Session) Touch()           { s.lastSeen.Store(time.Now().UnixNano()) }
func (s *Session) Listeners() int64 { return s.listeners.Load() }
func (s *Session) LastSeen() time.Time {
	return time.Unix(0, s.lastSeen.Load())
}
func (s *Session) Stopped() bool { return s.stopped.Load() }
func (s *Session) MarkStopped()  { s.stopped.Store(true) }
func (s *Session) Restarts() int64 {
	return s.restarts.Load()
}
func (s *Session) MarkReleased() bool {
	return s.released.CompareAndSwap(false, true)
}

func (s *Session) AddListener(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" {
		if s.listenerIDs == nil {
			s.listenerIDs = make(map[string]struct{})
		}
		if _, exists := s.listenerIDs[id]; exists {
			s.Touch()
			return
		}
		s.listenerIDs[id] = struct{}{}
	}
	s.listeners.Add(1)
	s.Touch()
}

func (s *Session) Leave(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" {
		if _, exists := s.listenerIDs[id]; !exists {
			return
		}
		delete(s.listenerIDs, id)
		if s.listeners.Add(-1) <= 0 {
			s.listeners.Store(0)
			s.Touch()
		}
		return
	}
	if s.listeners.Add(-1) <= 0 {
		s.listeners.Store(0)
		s.listenerIDs = nil
		s.Touch()
	}
}

func (s *Session) Stream() Streamer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stream
}

func (s *Session) SetStream(st Streamer) {
	s.mu.Lock()
	s.stream = st
	s.mu.Unlock()
}

func (s *Session) MarkWorkerStart(now time.Time) {
	s.mu.Lock()
	s.workerStartedAt = now
	s.mu.Unlock()
}

func (s *Session) RecordRestart(now time.Time, err error) int64 {
	count := s.restarts.Add(1)
	s.mu.Lock()
	s.lastRestartAt = now
	if err != nil {
		s.lastWorkerError = err.Error()
	} else {
		s.lastWorkerError = ""
	}
	s.mu.Unlock()
	return count
}

func (s *Session) RecordWorkerError(err error) {
	s.mu.Lock()
	if err != nil {
		s.lastWorkerError = err.Error()
	} else {
		s.lastWorkerError = ""
	}
	s.mu.Unlock()
}

func (s *Session) WorkerStartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workerStartedAt
}

func (s *Session) LastRestartAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRestartAt
}

func (s *Session) LastWorkerError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastWorkerError
}

type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func New() *Registry { return &Registry{sessions: map[string]*Session{}} }

func (r *Registry) Get(ch string) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[ch]
	return s, ok
}

func (r *Registry) Add(s *Session) {
	s.Touch()
	r.mu.Lock()
	r.sessions[s.Channel] = s
	r.mu.Unlock()
}

func (r *Registry) Remove(ch string) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[ch]
	if ok {
		delete(r.sessions, ch)
	}
	return s, ok
}

func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

func (r *Registry) Snapshot() []*Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

func (r *Registry) Reap(now time.Time, idle time.Duration) []*Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	var reaped []*Session
	for ch, s := range r.sessions {
		if now.Sub(s.LastSeen()) <= idle {
			continue
		}
		s.stopped.Store(true)
		delete(r.sessions, ch)
		reaped = append(reaped, s)
	}
	return reaped
}
