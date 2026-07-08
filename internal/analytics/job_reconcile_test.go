package analytics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pgregory.net/rapid"

	"streamclone/internal/jobstate"
)

// seedMirror applies a single in-set callback so a job has a known baseline
// state/seq before reconciliation.
func seedMirror(t *testing.T, m *JobMirror, jobID, state string, seq int64) {
	t.Helper()
	if _, applied := m.Apply(StatusCallback{JobID: jobID, State: state, Seq: seq}); !applied {
		t.Fatalf("seed apply of %q seq=%d did not take", state, seq)
	}
}

// TestReconcileSetsMirrorToStoreOnDivergentState covers the acceptance check:
// after reconcile the mirror equals the store value for divergent jobs.
func TestReconcileSetsMirrorToStoreOnDivergentState(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_1", jobstate.Rendering, 5)

	entry, changed := m.Reconcile(JobStoreSnapshot{
		JobID: "job_1", State: jobstate.Transcribing, Seq: 6,
		UpdatedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
	})
	if !changed {
		t.Fatal("expected reconcile to change the divergent mirror entry")
	}
	if entry.State != jobstate.Transcribing || entry.Seq != 6 {
		t.Fatalf("entry = (%q,%d), want (transcribing,6)", entry.State, entry.Seq)
	}
	got, _ := m.Get("job_1")
	if got.State != jobstate.Transcribing || got.Seq != 6 {
		t.Fatalf("mirror = (%q,%d), want store value (transcribing,6)", got.State, got.Seq)
	}
}

// TestReconcileLowerSeqStillWins proves reconciliation is the tie-break that
// overrides the normal idempotent seq rule: a store snapshot with a LOWER seq
// than the mirror still wins (Apply would have rejected it as stale).
func TestReconcileLowerSeqStillWins(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_2", jobstate.Complete, 10)

	// Sanity: Apply of a lower-seq state is a no-op (idempotent rule).
	if _, applied := m.Apply(StatusCallback{JobID: "job_2", State: jobstate.Rendering, Seq: 3}); applied {
		t.Fatal("Apply should reject a stale lower-seq callback")
	}

	entry, changed := m.Reconcile(JobStoreSnapshot{JobID: "job_2", State: jobstate.Rendering, Seq: 3})
	if !changed {
		t.Fatal("reconcile must adopt the store value even with a lower seq")
	}
	if entry.State != jobstate.Rendering || entry.Seq != 3 {
		t.Fatalf("entry = (%q,%d), want store value (rendering,3)", entry.State, entry.Seq)
	}
}

// TestReconcileNoChangeWhenAlreadyAgree: agreement is a no-op but still leaves
// the mirror equal to the store.
func TestReconcileNoChangeWhenAlreadyAgree(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_3", jobstate.Rendering, 5)

	entry, changed := m.Reconcile(JobStoreSnapshot{JobID: "job_3", State: jobstate.Rendering, Seq: 5})
	if changed {
		t.Fatal("reconcile of an agreeing job must not report a change")
	}
	if entry.State != jobstate.Rendering || entry.Seq != 5 {
		t.Fatalf("entry = (%q,%d), want (rendering,5)", entry.State, entry.Seq)
	}
}

// TestReconcileCreatesEntryForUnknownJob: reconcile seeds a mirror entry for a
// job never seen via callback.
func TestReconcileCreatesEntryForUnknownJob(t *testing.T) {
	m := NewJobMirror()
	entry, changed := m.Reconcile(JobStoreSnapshot{JobID: "job_new", State: jobstate.Queued, Seq: 1})
	if !changed {
		t.Fatal("expected reconcile to create the missing mirror entry")
	}
	if entry.State != jobstate.Queued || entry.Seq != 1 {
		t.Fatalf("entry = (%q,%d), want (queued,1)", entry.State, entry.Seq)
	}
	if _, ok := m.Get("job_new"); !ok {
		t.Fatal("mirror should now hold the reconciled job")
	}
}

// TestReconcileIgnoresOutOfSetStoreState: a malformed store state is never
// adopted (Property 2 defense-in-depth); the existing entry is untouched.
func TestReconcileIgnoresOutOfSetStoreState(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_4", jobstate.Rendering, 5)

	entry, changed := m.Reconcile(JobStoreSnapshot{JobID: "job_4", State: "totally_bogus", Seq: 99})
	if changed {
		t.Fatal("out-of-set store state must not change the mirror")
	}
	if entry.State != jobstate.Rendering || entry.Seq != 5 {
		t.Fatalf("entry = (%q,%d), want unchanged (rendering,5)", entry.State, entry.Seq)
	}
}

// TestReconcileEmptyJobIDIsNoOp guards the defensive empty-id path.
func TestReconcileEmptyJobIDIsNoOp(t *testing.T) {
	m := NewJobMirror()
	if _, changed := m.Reconcile(JobStoreSnapshot{JobID: "   ", State: jobstate.Queued, Seq: 1}); changed {
		t.Fatal("blank job id must be a no-op")
	}
}

// TestReconcileJobUsesInjectedFetcher exercises the injectable reconcile entry
// point with a fake fetcher (no network).
func TestReconcileJobUsesInjectedFetcher(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_5", jobstate.Rendering, 5)

	fetch := func(_ context.Context, jobID string) (JobStoreSnapshot, error) {
		return JobStoreSnapshot{JobID: jobID, State: jobstate.Complete, Seq: 9}, nil
	}
	entry, changed, err := m.ReconcileJob(context.Background(), "job_5", fetch)
	if err != nil {
		t.Fatalf("ReconcileJob: %v", err)
	}
	if !changed || entry.State != jobstate.Complete || entry.Seq != 9 {
		t.Fatalf("entry = (%q,%d) changed=%v, want (complete,9) changed=true", entry.State, entry.Seq, changed)
	}
}

// TestReconcileJobPropagatesFetchError: a fetch failure leaves the mirror
// untouched and surfaces the error.
func TestReconcileJobPropagatesFetchError(t *testing.T) {
	m := NewJobMirror()
	seedMirror(t, m, "job_6", jobstate.Rendering, 5)

	wantErr := errors.New("boom")
	_, changed, err := m.ReconcileJob(context.Background(), "job_6", func(context.Context, string) (JobStoreSnapshot, error) {
		return JobStoreSnapshot{}, wantErr
	})
	if err == nil || changed {
		t.Fatalf("expected fetch error and no change, got err=%v changed=%v", err, changed)
	}
	if got, _ := m.Get("job_6"); got.State != jobstate.Rendering || got.Seq != 5 {
		t.Fatalf("mirror mutated on fetch error: %+v", got)
	}
}

// TestReplayForgeHTTPClientFetchesJobStatusSnapshot verifies the authed status
// pull hits GET /v1/jobs/{id}/status, carries the bearer token, and parses the
// {job_id, state, seq, updated_at} snapshot for reconciliation.
func TestReplayForgeHTTPClientFetchesJobStatusSnapshot(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"job_id":     "rf_123",
			"state":      "rendering",
			"seq":        7,
			"updated_at": "2026-07-01T12:05:00Z",
		})
	}))
	defer srv.Close()

	client := NewReplayForgeHTTPClient(srv.URL, "secret-token")
	snap, err := client.GetJobStatus(context.Background(), "rf_123")
	if err != nil {
		t.Fatalf("GetJobStatus: %v", err)
	}
	if gotPath != "/v1/jobs/rf_123/status" {
		t.Fatalf("path = %q, want /v1/jobs/rf_123/status", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("authorization = %q, want bearer token", gotAuth)
	}
	if snap.JobID != "rf_123" || snap.State != jobstate.Rendering || snap.Seq != 7 {
		t.Fatalf("snapshot = %+v, want (rf_123,rendering,7)", snap)
	}
}

// TestPropReconciliationHealsMirrorDrift is Property 6: for any mirror state and
// any ReplayForge store state, after a reconciliation pull the mirror's
// state/seq equals the store's authoritative value (Job_Store is the
// tie-breaker), even when the store seq is lower than the mirror seq.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: auto-clipper-replayforge-productization, Property 6: Reconciliation reconciles Job_Mirror to Job_Store on disagreement
// **Validates: Requirement 2.8**
func TestPropReconciliationHealsMirrorDrift(t *testing.T) {
	states := jobstate.All()
	const jobID = "job"

	rapid.Check(t, func(rt *rapid.T) {
		m := NewJobMirror()

		// Build an arbitrary mirror state (possibly empty / never seen).
		if rapid.Bool().Draw(rt, "seedMirror") {
			applies := rapid.IntRange(1, 8).Draw(rt, "applies")
			for i := 0; i < applies; i++ {
				st := states[rapid.IntRange(0, len(states)-1).Draw(rt, "mirrorState")]
				seq := int64(rapid.IntRange(1, 100).Draw(rt, "mirrorSeq"))
				m.Apply(StatusCallback{JobID: jobID, State: st, Seq: seq})
			}
		}

		// Arbitrary authoritative store snapshot: any in-set state and any seq,
		// including a seq lower than whatever the mirror currently holds.
		snap := JobStoreSnapshot{
			JobID:     jobID,
			State:     states[rapid.IntRange(0, len(states)-1).Draw(rt, "storeState")],
			Seq:       int64(rapid.IntRange(0, 120).Draw(rt, "storeSeq")),
			UpdatedAt: time.Unix(int64(rapid.IntRange(0, 1<<31-1).Draw(rt, "storeTs")), 0).UTC(),
		}

		m.Reconcile(snap)

		got, ok := m.Get(jobID)
		if !ok {
			t.Fatalf("expected a mirror entry after reconciling in-set store state %q", snap.State)
		}
		if got.State != snap.State || got.Seq != snap.Seq {
			t.Fatalf("after reconcile mirror=(%q,%d) must equal store=(%q,%d)",
				got.State, got.Seq, snap.State, snap.Seq)
		}
	})
}
