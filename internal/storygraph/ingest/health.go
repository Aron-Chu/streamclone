package ingest

import (
	"sync"
	"time"
)

// SourceStatus is the last observed ingest outcome for one social source.
type SourceStatus struct {
	Source     string                        `json:"source"`
	Healthy    bool                          `json:"healthy"`
	LastOKAt   *time.Time                    `json:"last_ok_at,omitempty"`
	LastErrAt  *time.Time                    `json:"last_err_at,omitempty"`
	LastError  string                        `json:"last_error,omitempty"`
	LastItems  int                           `json:"last_items"`
	LastPollAt time.Time                     `json:"last_poll_at"`
	Details    map[string]SourceDetailStatus `json:"details,omitempty"`
}

// SourceDetailStatus is the last observed outcome for one source sub-path.
type SourceDetailStatus struct {
	Healthy    bool       `json:"healthy"`
	LastOKAt   *time.Time `json:"last_ok_at,omitempty"`
	LastErrAt  *time.Time `json:"last_err_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	LastItems  int        `json:"last_items"`
	LastPollAt time.Time  `json:"last_poll_at"`
}

// Health tracks per-source ingest outcomes for the source-health API.
type Health struct {
	mu     sync.RWMutex
	status map[string]SourceStatus
}

// NewHealth creates an empty ingest health tracker.
func NewHealth() *Health {
	return &Health{status: map[string]SourceStatus{}}
}

func (h *Health) record(source string, update func(*SourceStatus)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.status[source]
	row.Source = source
	row.LastPollAt = time.Now()
	update(&row)
	h.status[source] = row
}

func (h *Health) recordDetail(source, detail string, update func(*SourceDetailStatus)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.status[source]
	row.Source = source
	if row.Details == nil {
		row.Details = map[string]SourceDetailStatus{}
	}
	detailRow := row.Details[detail]
	detailRow.LastPollAt = time.Now()
	update(&detailRow)
	row.Details[detail] = detailRow
	h.status[source] = row
}

// RecordOK records a successful ingest poll.
func (h *Health) RecordOK(source string, items int) {
	now := time.Now()
	h.record(source, func(row *SourceStatus) {
		row.Healthy = true
		row.LastOKAt = &now
		row.LastError = ""
		row.LastItems = items
	})
}

// RecordSkip records a source that was skipped because Healthy() failed.
func (h *Health) RecordSkip(source, reason string) {
	now := time.Now()
	h.record(source, func(row *SourceStatus) {
		row.Healthy = false
		row.LastErrAt = &now
		row.LastError = reason
		row.LastItems = 0
	})
}

// RecordFail records a Search() failure.
func (h *Health) RecordFail(source string, err error) {
	now := time.Now()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	h.record(source, func(row *SourceStatus) {
		row.Healthy = false
		row.LastErrAt = &now
		row.LastError = msg
		row.LastItems = 0
	})
}

// RecordDetailOK records a successful source sub-path poll.
func (h *Health) RecordDetailOK(source, detail string, items int) {
	now := time.Now()
	h.recordDetail(source, detail, func(row *SourceDetailStatus) {
		row.Healthy = true
		row.LastOKAt = &now
		row.LastError = ""
		row.LastItems = items
	})
}

// RecordDetailSkip records a source sub-path that was intentionally skipped.
func (h *Health) RecordDetailSkip(source, detail, reason string) {
	now := time.Now()
	h.recordDetail(source, detail, func(row *SourceDetailStatus) {
		row.Healthy = false
		row.LastErrAt = &now
		row.LastError = reason
		row.LastItems = 0
	})
}

// RecordDetailFail records a source sub-path failure.
func (h *Health) RecordDetailFail(source, detail string, err error) {
	now := time.Now()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	h.recordDetail(source, detail, func(row *SourceDetailStatus) {
		row.Healthy = false
		row.LastErrAt = &now
		row.LastError = msg
		row.LastItems = 0
	})
}

// Snapshot returns a copy of the latest per-source statuses.
func (h *Health) Snapshot() map[string]SourceStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]SourceStatus, len(h.status))
	for k, v := range h.status {
		if v.Details != nil {
			details := make(map[string]SourceDetailStatus, len(v.Details))
			for name, detail := range v.Details {
				details[name] = detail
			}
			v.Details = details
		}
		out[k] = v
	}
	return out
}
