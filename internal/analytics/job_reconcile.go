package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"streamclone/internal/jobstate"
	"streamclone/internal/redact"
)

// JobStoreSnapshot is the authoritative {job_id, state, seq, updated_at} value
// pulled from ReplayForge's source-of-truth SQLite Job_Store via the authed
// `GET /v1/jobs/{id}/status` endpoint (spec
// auto-clipper-replayforge-productization, RF-P1-004). Reconciliation uses this
// snapshot to heal Job_Mirror drift: on disagreement the mirror is set to the
// store value (SoT wins), overriding the normal idempotent seq rules that
// govern Apply.
type JobStoreSnapshot struct {
	JobID     string    `json:"job_id"`
	State     string    `json:"state"`
	Seq       int64     `json:"seq"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Reconcile sets the mirror entry for snap.JobID to the authoritative Job_Store
// value whenever the two disagree, regardless of seq ordering. This is the
// SoT tie-break (RF-P1-010, Requirement 2.8): unlike Apply — which ignores
// stale/equal-seq callbacks to stay idempotent (Property 5) — reconciliation
// unconditionally adopts the store's state/seq, so a store value with a *lower*
// seq than the mirror still wins. Out-of-set store states are ignored
// defensively so the mirror can never hold a value outside the Job_State_Set
// (Property 2).
//
// It returns the resulting entry and whether the mirror was changed. Callbacks
// that already agree with the store are a no-op (changed=false) but still leave
// the mirror equal to the store, so the post-condition "mirror == store" holds
// for every reconciled job (Property 6).
func (m *JobMirror) Reconcile(snap JobStoreSnapshot) (JobMirrorEntry, bool) {
	if m == nil {
		return JobMirrorEntry{}, false
	}
	jobID := strings.TrimSpace(snap.JobID)
	state := strings.TrimSpace(snap.State)
	if jobID == "" {
		return JobMirrorEntry{}, false
	}
	// Defensive: never adopt a store state outside the canonical Job_State_Set.
	// The store is the source of truth, but the mirror invariant (Property 2)
	// still holds locally — a malformed/unknown state is left untouched rather
	// than mirrored.
	if !jobstate.InSet(state) {
		slog.Warn(redact.Log("job_mirror reconcile ignored: store state not in Job_State_Set",
			"job_id", jobID, "state", state))
		cur, _ := m.Get(jobID)
		return cur, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cur, exists := m.entries[jobID]
	// Already in agreement with the store: nothing to heal.
	if exists && cur.State == state && cur.Seq == snap.Seq {
		return cur, false
	}
	entry := JobMirrorEntry{
		JobID:     jobID,
		State:     state,
		Seq:       snap.Seq,
		UpdatedAt: snap.UpdatedAt,
		// Reconciliation is not a callback delivery, so preserve the last
		// callback timestamp (zero for a job first seen via reconcile).
		LastCallbackAt: cur.LastCallbackAt,
	}
	m.entries[jobID] = entry
	return entry, true
}

// JobStoreFetcher pulls the authoritative Job_Store snapshot for a job id. It
// is injected into ReconcileJob so reconciliation can be unit/property tested
// without any network I/O; the production adapter (NewReplayForgeStatusFetcher)
// carries the Auth_Token over HTTP.
type JobStoreFetcher func(ctx context.Context, jobID string) (JobStoreSnapshot, error)

// ReconcileJob is the reconciliation pull entry point: it fetches the
// authoritative Job_Store snapshot for jobID and reconciles the mirror to it
// (SoT tie-break). It returns the resulting entry, whether the mirror changed,
// and any fetch error. There is deliberately no background ticker here — a
// single reconcile pass is the unit of work; a caller may schedule it.
func (m *JobMirror) ReconcileJob(ctx context.Context, jobID string, fetch JobStoreFetcher) (JobMirrorEntry, bool, error) {
	if fetch == nil {
		return JobMirrorEntry{}, false, errors.New("reconcile_fetcher_nil")
	}
	snap, err := fetch(ctx, jobID)
	if err != nil {
		return JobMirrorEntry{}, false, err
	}
	entry, changed := m.Reconcile(snap)
	return entry, changed, nil
}

// jobStatusFetcher is the subset of the ReplayForge client used by
// reconciliation: an authed pull of the SoT status snapshot.
type jobStatusFetcher interface {
	GetJobStatus(ctx context.Context, jobID string) (JobStoreSnapshot, error)
}

// NewReplayForgeStatusFetcher adapts a ReplayForge status client into a
// JobStoreFetcher for reconciliation. The client owns the Auth_Token; keeping
// this behind the JobStoreFetcher seam lets reconcile be tested with a fake.
func NewReplayForgeStatusFetcher(client jobStatusFetcher) JobStoreFetcher {
	return func(ctx context.Context, jobID string) (JobStoreSnapshot, error) {
		if client == nil {
			return JobStoreSnapshot{}, errors.New("replayforge_unconfigured")
		}
		return client.GetJobStatus(ctx, jobID)
	}
}

// GetJobStatus pulls the authoritative {job_id, state, seq, updated_at} snapshot
// from ReplayForge's authed `GET /v1/jobs/{id}/status` endpoint (RF-P1-004) for
// reconciliation. It carries the Auth_Token as a bearer credential and never
// places it in the URL or logs.
func (c *ReplayForgeHTTPClient) GetJobStatus(ctx context.Context, jobID string) (JobStoreSnapshot, error) {
	if c == nil || c.baseURL == "" {
		return JobStoreSnapshot{}, errors.New("replayforge_unconfigured")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return JobStoreSnapshot{}, errors.New("replayforge_missing_job_id")
	}
	endpoint, err := url.JoinPath(c.baseURL, "/v1/jobs", jobID, "status")
	if err != nil {
		return JobStoreSnapshot{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return JobStoreSnapshot{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return JobStoreSnapshot{}, err
	}
	defer resp.Body.Close()
	var payload JobStoreSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return JobStoreSnapshot{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state := strings.TrimSpace(payload.State)
		if state == "" {
			state = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return JobStoreSnapshot{}, fmt.Errorf("replayforge_%s", state)
	}
	payload.JobID = strings.TrimSpace(payload.JobID)
	payload.State = strings.TrimSpace(payload.State)
	if payload.JobID == "" {
		payload.JobID = jobID
	}
	return payload, nil
}
