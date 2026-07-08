package analytics

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"streamclone/internal/jobstate"
)

// callbackBody marshals a Status_Callback payload without hand-escaping.
func callbackBody(jobID, state string, seq int64) string {
	b, _ := json.Marshal(map[string]any{
		"job_id":     jobID,
		"state":      state,
		"seq":        seq,
		"updated_at": "2026-07-01T12:00:00Z",
	})
	return string(b)
}

// drawOutOfSetState draws a state guaranteed NOT to be a Job_State_Set member.
func drawOutOfSetState(rt *rapid.T) string {
	s := rapid.StringMatching(`[a-z_]{0,20}`).Draw(rt, "rawState")
	if trimmed := strings.TrimSpace(s); trimmed == "" || jobstate.InSet(trimmed) {
		return s + "_x_not_a_state"
	}
	return s
}

// TestPropJobMirrorStateAlwaysInSet drives arbitrary in-set and out-of-set
// callbacks through the authed handler and asserts the mirror only ever stores
// (and the response only ever reports) Job_State_Set members; out-of-set values
// are rejected with 400 and never applied.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: auto-clipper-replayforge-productization, Property 2: Mirrored/displayed state is always in the Job_State_Set
// **Validates: Requirements 2.2, 6.4**
func TestPropJobMirrorStateAlwaysInSet(t *testing.T) {
	states := jobstate.All()
	jobIDs := []string{"job_a", "job_b", "job_c"}

	rapid.Check(t, func(rt *rapid.T) {
		h, router := newJobMirrorTestHandler()
		n := rapid.IntRange(1, 30).Draw(rt, "numCallbacks")

		for i := 0; i < n; i++ {
			jobID := jobIDs[rapid.IntRange(0, len(jobIDs)-1).Draw(rt, "jobIdx")]
			seq := int64(rapid.IntRange(0, 50).Draw(rt, "seq"))
			inSet := rapid.Bool().Draw(rt, "inSet")

			var state string
			if inSet {
				state = states[rapid.IntRange(0, len(states)-1).Draw(rt, "stateIdx")]
			} else {
				state = drawOutOfSetState(rt)
			}

			rec := postCallback(t, router, testCallbackToken, callbackBody(jobID, state, seq))

			if inSet {
				if rec.Code != http.StatusOK {
					t.Fatalf("in-set state %q should be accepted, got %d", state, rec.Code)
				}
				resp := decodeCallbackResponse(t, rec)
				// The displayed state must always be a set member (or empty on a
				// no-op for a never-seen job).
				if resp.State != "" && !jobstate.InSet(resp.State) {
					t.Fatalf("displayed state %q is not in the Job_State_Set", resp.State)
				}
			} else if rec.Code != http.StatusBadRequest {
				t.Fatalf("out-of-set state %q must be rejected with 400, got %d", state, rec.Code)
			}

			// Invariant: every mirrored job holds an in-set state at all times.
			for _, jid := range jobIDs {
				if entry, ok := h.jobMirror().Get(jid); ok {
					if !jobstate.InSet(entry.State) {
						t.Fatalf("mirror stored out-of-set state %q for %s", entry.State, jid)
					}
				}
			}
		}
	})
}

// TestPropJobMirrorCallbackIdempotent asserts that a callback whose state is
// already applied (or whose seq is not newer) leaves the mirror unchanged no
// matter how many times it is re-applied, and always reports success.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: auto-clipper-replayforge-productization, Property 5: Status_Callback application is idempotent
// **Validates: Requirement 2.4**
func TestPropJobMirrorCallbackIdempotent(t *testing.T) {
	states := jobstate.All()
	const jobID = "job"

	rapid.Check(t, func(rt *rapid.T) {
		m := NewJobMirror()

		// Build a current mirror entry via a few in-set applies (seq >= 1 so the
		// first callback always establishes an entry).
		applies := rapid.IntRange(1, 10).Draw(rt, "applies")
		for i := 0; i < applies; i++ {
			state := states[rapid.IntRange(0, len(states)-1).Draw(rt, "buildStateIdx")]
			seq := int64(rapid.IntRange(1, 100).Draw(rt, "buildSeq"))
			m.Apply(StatusCallback{JobID: jobID, State: state, Seq: seq})
		}
		cur, ok := m.Get(jobID)
		if !ok {
			t.Fatal("expected an established mirror entry after build phase")
		}

		// Construct a callback that must be an idempotent no-op:
		//  - "not newer": seq <= cur.Seq with any in-set state, or
		//  - "already applied": state == cur.State with any seq.
		var noop StatusCallback
		if rapid.Bool().Draw(rt, "staleSeqBranch") {
			noop = StatusCallback{
				JobID: jobID,
				State: states[rapid.IntRange(0, len(states)-1).Draw(rt, "noopStateIdx")],
				Seq:   int64(rapid.IntRange(0, int(cur.Seq)).Draw(rt, "staleSeq")),
			}
		} else {
			noop = StatusCallback{
				JobID: jobID,
				State: cur.State,
				Seq:   int64(rapid.IntRange(0, 200).Draw(rt, "sameStateSeq")),
			}
		}

		reps := rapid.IntRange(1, 20).Draw(rt, "reps")
		for i := 0; i < reps; i++ {
			entry, applied := m.Apply(noop)
			if applied {
				t.Fatalf("idempotent no-op callback unexpectedly mutated the mirror: %+v", noop)
			}
			if entry.State != cur.State || entry.Seq != cur.Seq || entry.JobID != cur.JobID {
				t.Fatalf("no-op returned a changed entry: got %+v want %+v", entry, cur)
			}
		}

		after, _ := m.Get(jobID)
		if after.State != cur.State || after.Seq != cur.Seq {
			t.Fatalf("mirror changed after idempotent no-ops: got state=%q seq=%d want state=%q seq=%d",
				after.State, after.Seq, cur.State, cur.Seq)
		}
	})
}

// drawInvalidCallbackToken draws an Auth_Token that is guaranteed NOT to be the
// configured callback token: either missing (empty) or an arbitrary wrong
// value. postCallback/postWebhook omit the Authorization header entirely when
// the token is empty, so this covers both the "missing" and "invalid" cases in
// Requirement 2.5/2.6.
func drawInvalidCallbackToken(rt *rapid.T) string {
	if rapid.Bool().Draw(rt, "missingToken") {
		return "" // no Authorization header at all
	}
	tok := rapid.StringMatching(`[A-Za-z0-9_\-]{0,24}`).Draw(rt, "wrongToken")
	if tok == testCallbackToken {
		tok += "_not_the_token"
	}
	return tok
}

// TestPropUnauthenticatedJobMirrorCallbackRejected drives arbitrary
// unauthenticated Status_Callbacks (random job/state/seq, missing or wrong
// token) at the authed Job_Mirror callback endpoint. Every such request must be
// rejected with 401 and the Job_Mirror must be byte-for-byte unchanged compared
// to a snapshot taken before the unauthenticated traffic. The mirror is first
// seeded through the authenticated path so the "unchanged" assertion has real
// state to protect.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: auto-clipper-replayforge-productization, Property 4: Unauthenticated job mutation or callback is rejected without side effects
// **Validates: Requirements 2.5, 2.6, 2.7**
func TestPropUnauthenticatedJobMirrorCallbackRejected(t *testing.T) {
	states := jobstate.All()
	jobIDs := []string{"job_a", "job_b", "job_c"}

	rapid.Check(t, func(rt *rapid.T) {
		h, router := newJobMirrorTestHandler()

		// Seed the mirror with a few authenticated callbacks so the before/after
		// snapshot protects a non-trivial state.
		seedN := rapid.IntRange(0, 6).Draw(rt, "seedCount")
		for i := 0; i < seedN; i++ {
			jobID := jobIDs[rapid.IntRange(0, len(jobIDs)-1).Draw(rt, "seedJob")]
			state := states[rapid.IntRange(0, len(states)-1).Draw(rt, "seedState")]
			seq := int64(rapid.IntRange(1, 50).Draw(rt, "seedSeq"))
			postCallback(t, router, testCallbackToken, callbackBody(jobID, state, seq))
		}

		before := mirrorSnapshot(h.jobMirror())

		// Fire a batch of unauthenticated callbacks; every one must be 401.
		n := rapid.IntRange(1, 25).Draw(rt, "unauthCount")
		for i := 0; i < n; i++ {
			jobID := jobIDs[rapid.IntRange(0, len(jobIDs)-1).Draw(rt, "job")]
			state := states[rapid.IntRange(0, len(states)-1).Draw(rt, "state")]
			seq := int64(rapid.IntRange(0, 100).Draw(rt, "seq"))
			token := drawInvalidCallbackToken(rt)

			rec := postCallback(t, router, token, callbackBody(jobID, state, seq))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated callback (token=%q) must be rejected with 401, got %d (%s)",
					token, rec.Code, rec.Body.String())
			}
		}

		after := mirrorSnapshot(h.jobMirror())
		if before != after {
			t.Fatalf("Job_Mirror mutated under unauthenticated callbacks:\n before=%s\n  after=%s", before, after)
		}
	})
}

// TestPropUnauthenticatedReplayForgeWebhookRejected proves the same guarantee
// for the legacy Clip_Job mutation endpoint
// (POST /v1/internal/replayforge/jobs/{jobID}, Requirement 2.7): an
// unauthenticated request is rejected with 401 before the handler ever reaches
// the store, so no Clip_Job state can change. The handler is constructed with a
// nil store, making any state mutation impossible past the auth gate.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: auto-clipper-replayforge-productization, Property 4: Unauthenticated job mutation or callback is rejected without side effects
// **Validates: Requirements 2.5, 2.6, 2.7**
func TestPropUnauthenticatedReplayForgeWebhookRejected(t *testing.T) {
	states := jobstate.All()
	jobIDs := []string{"rf_1", "rf_2", "rf_3"}

	rapid.Check(t, func(rt *rapid.T) {
		_, router := newReplayForgeWebhookTestHandler()

		jobID := jobIDs[rapid.IntRange(0, len(jobIDs)-1).Draw(rt, "job")]
		state := states[rapid.IntRange(0, len(states)-1).Draw(rt, "state")]
		token := drawInvalidCallbackToken(rt)

		body, _ := json.Marshal(map[string]any{
			"job": map[string]any{"id": jobID, "state": state},
		})
		rec := postWebhook(t, router, token, jobID, string(body))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated ReplayForge webhook (token=%q) must be rejected with 401, got %d (%s)",
				token, rec.Code, rec.Body.String())
		}
	})
}
