package analytics

import (
	"sync"
	"time"

	"streamclone/internal/jobstate"
)

// StatusCallback is the ReplayForge → Streamclone callback payload
// (spec auto-clipper-replayforge-productization, RF-P1-006/RF-P1-008).
// ReplayForge POSTs one on every job state change; Streamclone applies it to
// the Job_Mirror read model. The mirror is never authoritative — ReplayForge's
// SQLite Job_Store is the source of truth — so this model only ever tracks the
// last state ReplayForge reported.
type StatusCallback struct {
	JobID     string    `json:"job_id"`
	State     string    `json:"state"`
	Seq       int64     `json:"seq"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JobMirrorEntry is the minimal per-job read model (Option B — minimal mirror).
// Seq is the last applied monotonic sequence number; callbacks with a lower or
// equal Seq are ignored so application is idempotent (Property 5).
type JobMirrorEntry struct {
	JobID          string    `json:"jobId"`
	State          string    `json:"state"`
	Seq            int64     `json:"seq"`
	UpdatedAt      time.Time `json:"updatedAt,omitempty"`
	LastCallbackAt time.Time `json:"lastCallbackAt,omitempty"`
}

// JobMirror is the Streamclone read model of ReplayForge job state. It is
// updated ONLY through Apply (driven by the authed idempotent Status_Callback
// handler); there is deliberately no setter that bypasses the seq/state guards.
//
// The current implementation keeps entries in memory. Reconciliation
// (RF-P1-010) and durable persistence are follow-up tasks; this type exposes
// Get/Apply so those can wrap or replace the backing store without changing the
// handler contract.
type JobMirror struct {
	mu      sync.RWMutex
	entries map[string]JobMirrorEntry
}

// NewJobMirror returns an empty in-memory Job_Mirror.
func NewJobMirror() *JobMirror {
	return &JobMirror{entries: make(map[string]JobMirrorEntry)}
}

// Get returns the current mirror entry for jobID and whether one exists.
func (m *JobMirror) Get(jobID string) (JobMirrorEntry, bool) {
	if m == nil {
		return JobMirrorEntry{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[jobID]
	return entry, ok
}

// jobMirrorDecision classifies how a callback should be handled relative to the
// current mirror entry, independent of transport and auth. It is a pure
// function of (current entry, callback) so it can be exhaustively property
// tested.
type jobMirrorDecision int

const (
	// mirrorReject: state is not a member of the Job_State_Set → 400, never
	// applied or displayed (Property 2).
	mirrorReject jobMirrorDecision = iota
	// mirrorNoOp: state already applied or seq not newer → 200 idempotent
	// no-op, mirror unchanged (Property 5).
	mirrorNoOp
	// mirrorApply: newer in-set state → apply and 200.
	mirrorApply
)

// classifyCallback decides how cb should be handled against cur (the zero value
// when the job is not yet mirrored). It mirrors the design pseudocode:
//
//	if !InStateSet(cb.State) -> reject
//	if cb.Seq <= cur.Seq || cb.State == cur.State -> no-op
//	else -> apply
func classifyCallback(cur JobMirrorEntry, cb StatusCallback) jobMirrorDecision {
	if !jobstate.InSet(cb.State) {
		return mirrorReject
	}
	if cb.Seq <= cur.Seq || cb.State == cur.State {
		return mirrorNoOp
	}
	return mirrorApply
}

// Apply attempts to apply cb to the mirror and returns the resulting (or
// unchanged) entry plus whether a mutation occurred. It enforces both mirror
// invariants under the lock:
//
//   - out-of-set states are never stored (Property 2, defense-in-depth: the
//     handler already rejects them with 400 before calling Apply);
//   - stale/equal-seq or same-state callbacks are idempotent no-ops that leave
//     the entry untouched (Property 5).
func (m *JobMirror) Apply(cb StatusCallback) (JobMirrorEntry, bool) {
	if m == nil {
		return JobMirrorEntry{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := m.entries[cb.JobID] // zero value when the job is not yet mirrored
	switch classifyCallback(cur, cb) {
	case mirrorApply:
		entry := JobMirrorEntry{
			JobID:          cb.JobID,
			State:          cb.State,
			Seq:            cb.Seq,
			UpdatedAt:      cb.UpdatedAt,
			LastCallbackAt: cb.UpdatedAt,
		}
		m.entries[cb.JobID] = entry
		return entry, true
	default: // mirrorReject or mirrorNoOp — never mutate
		return cur, false
	}
}
