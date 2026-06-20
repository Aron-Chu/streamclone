package ingest

import (
	"sync"
	"time"
)

// DirectorySamplerStatus tracks directory sample runs for source-health and rising APIs.
type DirectorySamplerStatus struct {
	Healthy         bool       `json:"healthy"`
	LastRunAt       time.Time  `json:"lastRunAt"`
	LastSuccessAt   *time.Time `json:"lastSuccessAt,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	LastSampleCount int        `json:"lastSampleCount"`
	NextRetryAt     *time.Time `json:"nextRetryAt,omitempty"`
	HistoryDays     float64    `json:"historyDays,omitempty"`
	HistoryGathering bool      `json:"historyGathering,omitempty"`
}

// DirectorySamplerHealth tracks metadata directory sampling outcomes.
type DirectorySamplerHealth struct {
	mu     sync.RWMutex
	status DirectorySamplerStatus
}

func NewDirectorySamplerHealth() *DirectorySamplerHealth {
	return &DirectorySamplerHealth{}
}

func (h *DirectorySamplerHealth) RecordAttempt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status.LastRunAt = time.Now().UTC()
}

func (h *DirectorySamplerHealth) RecordSuccess(count int, historyDays float64) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status.Healthy = true
	h.status.LastRunAt = now
	h.status.LastSuccessAt = &now
	h.status.LastError = ""
	h.status.LastSampleCount = count
	h.status.NextRetryAt = nil
	h.status.HistoryDays = historyDays
	h.status.HistoryGathering = historyDays > 0 && historyDays < 7
}

func (h *DirectorySamplerHealth) RecordFailure(errMsg string, retryAt *time.Time) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status.Healthy = false
	h.status.LastRunAt = now
	h.status.LastError = errMsg
	h.status.LastSampleCount = 0
	h.status.NextRetryAt = retryAt
}

func (h *DirectorySamplerHealth) Snapshot() DirectorySamplerStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// WindowScoreStatus tracks the last window score recompute.
type WindowScoreStatus struct {
	Healthy       bool       `json:"healthy"`
	LastRunAt     time.Time  `json:"lastRunAt"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

// WindowScoreHealth tracks window score recompute outcomes.
type WindowScoreHealth struct {
	mu     sync.RWMutex
	status WindowScoreStatus
}

func NewWindowScoreHealth() *WindowScoreHealth {
	return &WindowScoreHealth{}
}

func (h *WindowScoreHealth) RecordSuccess() {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status.Healthy = true
	h.status.LastRunAt = now
	h.status.LastSuccessAt = &now
	h.status.LastError = ""
}

func (h *WindowScoreHealth) RecordFailure(errMsg string) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status.Healthy = false
	h.status.LastRunAt = now
	h.status.LastError = errMsg
}

func (h *WindowScoreHealth) Snapshot() WindowScoreStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}
