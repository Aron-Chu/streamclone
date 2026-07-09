package analytics

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"streamclone/internal/jobstate"
)

// Consolidated Streamclone ↔ ReplayForge client/API contract tests
// (spec auto-clipper-replayforge-productization, RF-P5-012, Requirements 6.1, 6.3).
//
// This file pins the *wire shapes* Streamclone produces and consumes at the
// ReplayForge HTTP boundary so the integration contract cannot drift silently.
// It deliberately does NOT re-prove behavior already covered by earlier Phase 5
// tasks; those remain the source of truth for their slice of the contract:
//
//   - moment_context field content + server-side score, token-free body
//     (RF-P5-001 / RF-P5-010): clip_replayforge_test.go.
//   - authenticated Export Moment trigger + recorded Clip_Job id, Property 14
//     (RF-P5-002 / RF-P5-003): clip_replayforge_trigger_auth_test.go.
//   - minimal mirrored-status display DTO, honest empty state (RF-P5-004):
//     job_mirror_display_test.go.
//   - callback apply / stale-seq / same-state / missing-id / unconfigured
//     behavior (RF-P1-008): job_mirror_test.go; unauth-no-side-effect Property
//     P4 and in-set Property P2: job_mirror_prop_test.go.
//
// The gaps this file fills are (a) golden JSON-key assertions of the four
// request/response envelopes exchanged at the boundary, (b) a round-trip check
// that the trigger request's optional fields honor `omitempty`, and (c) a
// single status-code table for the callback endpoint mirroring the design's
// HTTP interface table (200 applied / 200 no-op / 400 invalid state / 401
// unauthenticated).

// TestContractReplayForgeTriggerRequestJSONKeys pins the top-level wire shape of
// the Streamclone → ReplayForge create request body (design interface table:
// POST create with moment_context). moment_context content is asserted
// separately in clip_replayforge_test.go.
func TestContractReplayForgeTriggerRequestJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(ReplayForgeTriggerRequest{}), []string{
		"channel",
		"title",
		"duration",
		"final_duration",
		"moment_context",
	})
}

// TestContractReplayForgeTriggerResponseJSONKeys pins the create response the
// client parses. existing_job_id carries the duplicate-suppressed id
// (Requirement 2.10) and the client falls back to it when job_id is empty.
func TestContractReplayForgeTriggerResponseJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(ReplayForgeTriggerResponse{}), []string{
		"status",
		"job_id",
		"existing_job_id",
		"reason",
	})
}

// TestContractStatusCallbackJSONKeys pins the ReplayForge → Streamclone
// Status_Callback request body: {job_id, state, seq, updated_at} exactly as the
// design interface table documents.
func TestContractStatusCallbackJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(StatusCallback{}), []string{
		"job_id",
		"state",
		"seq",
		"updated_at",
	})
}

// TestContractJobMirrorCallbackResponseJSONKeys pins the callback
// acknowledgement body the mirror returns for every accepted callback (applied
// or idempotent no-op). It must expose only minimal fields — never auth
// material or the internal principal id.
func TestContractJobMirrorCallbackResponseJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(jobMirrorCallbackResponse{}), []string{
		"jobId",
		"state",
		"seq",
		"applied",
	})
}

// TestContractJobMirrorEntryJSONKeys pins the persisted/displayed mirror
// read-model DTO (Job_Mirror shape in the design data models).
func TestContractJobMirrorEntryJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(JobMirrorEntry{}), []string{
		"jobId",
		"state",
		"seq",
		"updatedAt",
		"lastCallbackAt",
	})
}

// marshalledKeys marshals v and returns its sorted top-level JSON object keys.
func marshalledKeys(t *testing.T, v interface{}) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal %T to object: %v (raw=%s)", v, err, raw)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestContractReplayForgeTriggerRequestOmitemptyRoundTrip proves the runtime
// wire envelope, not just the struct tags: a fully-populated trigger request
// serializes all documented keys, while a minimal request (channel only) drops
// every `omitempty` optional field. This locks the required-vs-optional split
// at the create boundary.
func TestContractReplayForgeTriggerRequestOmitemptyRoundTrip(t *testing.T) {
	full := ReplayForgeTriggerRequest{
		Channel:       "xqc",
		Title:         "Good bit",
		Duration:      60,
		FinalDuration: 45,
		MomentContext: map[string]interface{}{"vod_id": "v123"},
	}
	wantFull := []string{"channel", "duration", "final_duration", "moment_context", "title"}
	if got := marshalledKeys(t, full); !equalStrings(got, wantFull) {
		t.Fatalf("fully-populated trigger request keys\ngot:  %v\nwant: %v", got, wantFull)
	}

	// channel has no omitempty, so it is the one required key that always emits.
	minimal := ReplayForgeTriggerRequest{Channel: "xqc"}
	wantMinimal := []string{"channel"}
	if got := marshalledKeys(t, minimal); !equalStrings(got, wantMinimal) {
		t.Fatalf("minimal trigger request keys\ngot:  %v\nwant: %v", got, wantMinimal)
	}

	// A create response with only existing_job_id (duplicate-suppressed) must
	// still round-trip that id back to the client so the mirror can record it.
	var resp ReplayForgeTriggerResponse
	if err := json.Unmarshal([]byte(`{"status":"exists","existing_job_id":"job_dup_1"}`), &resp); err != nil {
		t.Fatalf("decode duplicate-suppressed response: %v", err)
	}
	if resp.ExistingJobID != "job_dup_1" || resp.JobID != "" {
		t.Fatalf("duplicate-suppressed response = %+v, want existing_job_id only", resp)
	}
}

// TestContractStatusCallbackRoundTrip decodes the documented callback body and
// asserts every field lands on StatusCallback, then re-encodes to confirm the
// same wire keys survive a round trip.
func TestContractStatusCallbackRoundTrip(t *testing.T) {
	const body = `{"job_id":"job_rt_1","state":"rendering","seq":7,"updated_at":"2026-07-01T12:05:00Z"}`
	var cb StatusCallback
	if err := json.Unmarshal([]byte(body), &cb); err != nil {
		t.Fatalf("decode callback: %v", err)
	}
	if cb.JobID != "job_rt_1" || cb.State != jobstate.Rendering || cb.Seq != 7 {
		t.Fatalf("decoded callback = %+v, want job_rt_1/rendering/7", cb)
	}
	if cb.UpdatedAt.IsZero() {
		t.Fatal("updated_at must decode into a non-zero time")
	}
	wantKeys := []string{"job_id", "seq", "state", "updated_at"}
	if got := marshalledKeys(t, cb); !equalStrings(got, wantKeys) {
		t.Fatalf("re-encoded callback keys\ngot:  %v\nwant: %v", got, wantKeys)
	}
}

// TestContractCallbackStatusCodeTable consolidates the documented HTTP status
// codes for the authed Status_Callback endpoint into one table, matching the
// design interface table for POST /v1/clipper/callback. Behavioral edge cases
// (stale seq, same-state, unconfigured 503, no-side-effect proofs) live in
// job_mirror_test.go and job_mirror_prop_test.go; this asserts the contract
// codes together so a boundary regression is caught in one place.
func TestContractCallbackStatusCodeTable(t *testing.T) {
	cases := []struct {
		name     string
		token    string
		body     string
		wantCode int
	}{
		{
			name:     "authed in-set newer state applies -> 200",
			token:    testCallbackToken,
			body:     `{"job_id":"job_tbl_apply","state":"rendering","seq":3,"updated_at":"2026-07-01T12:00:00Z"}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "authed out-of-set state -> 400",
			token:    testCallbackToken,
			body:     `{"job_id":"job_tbl_bad","state":"not_a_state","seq":1,"updated_at":"2026-07-01T12:00:00Z"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing token -> 401",
			token:    "",
			body:     `{"job_id":"job_tbl_noauth","state":"queued","seq":1,"updated_at":"2026-07-01T12:00:00Z"}`,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "invalid token -> 401",
			token:    "wrong-token",
			body:     `{"job_id":"job_tbl_badauth","state":"queued","seq":1,"updated_at":"2026-07-01T12:00:00Z"}`,
			wantCode: http.StatusUnauthorized,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, router := newJobMirrorTestHandler()
			rec := postCallback(t, router, tc.token, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}

	// The idempotent no-op branch also returns 200: re-applying an already
	// applied (equal-seq) callback leaves the mirror unchanged.
	_, router := newJobMirrorTestHandler()
	body := `{"job_id":"job_tbl_noop","state":"rendering","seq":5,"updated_at":"2026-07-01T12:00:00Z"}`
	if first := postCallback(t, router, testCallbackToken, body); first.Code != http.StatusOK {
		t.Fatalf("first apply status = %d, want 200", first.Code)
	}
	repeat := postCallback(t, router, testCallbackToken, body)
	if repeat.Code != http.StatusOK {
		t.Fatalf("idempotent no-op status = %d, want 200", repeat.Code)
	}
	if decodeCallbackResponse(t, repeat).Applied {
		t.Fatal("re-applying the same callback must be an idempotent no-op")
	}
}
