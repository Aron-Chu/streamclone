package analytics

import (
	"encoding/json"
	"net/http"
	"testing"

	"streamclone/internal/jobstate"
)

// RF-P5-004 (Option B — minimal mirror): Streamclone DISPLAYS only the minimal
// mirrored Job_State honestly. These regression tests lock the *shape* of the
// mirrored-status response that the watch desk consumes:
//
//   - the displayed state is always a Job_State_Set member (or an honest empty
//     value when the mirror has no status yet), and
//   - the serialized response never leaks tokens, secrets, or principal ids.
//
// The in-set property itself is proven across many inputs by
// TestPropJobMirrorStateAlwaysInSet (Property 2); these focus on the display
// contract at the serialization boundary.

// leakyResponseKeys are substrings that must never appear as JSON keys in a
// mirrored-status response surfaced to a client (Requirement 1.7 / 6.4). The
// mirror is a minimal read model: it exposes job id, state, seq, and timing —
// never auth material or the internal principal id.
var leakyResponseKeys = []string{"token", "secret", "auth", "principal", "bearer"}

func assertNoLeakyKeys(t *testing.T, body []byte) {
	t.Helper()
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatalf("decode response as object: %v (body=%q)", err, string(body))
	}
	for key := range generic {
		lower := key
		for _, bad := range leakyResponseKeys {
			if containsFold(lower, bad) {
				t.Fatalf("mirrored-status response leaks key %q (matched %q): %s", key, bad, string(body))
			}
		}
	}
}

// containsFold is a tiny case-insensitive substring check kept local so the
// test has no dependency beyond the standard library shape it asserts.
func containsFold(s, sub string) bool {
	sLower := toLowerASCII(s)
	subLower := toLowerASCII(sub)
	return indexOf(sLower, subLower) >= 0
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestJobMirrorDisplayResponseIsMinimalAndInSet asserts that an applied callback
// surfaces the mirrored state as a Job_State_Set member and that the response
// body carries only the minimal mirror fields — no token/secret/principal.
//
// spec auto-clipper-replayforge-productization, RF-P5-004, Requirement 6.4.
func TestJobMirrorDisplayResponseIsMinimalAndInSet(t *testing.T) {
	_, router := newJobMirrorTestHandler()
	rec := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_display_1","state":"rendering","seq":4,"updated_at":"2026-07-01T12:00:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	resp := decodeCallbackResponse(t, rec)
	if !jobstate.InSet(resp.State) {
		t.Fatalf("displayed state %q is not a Job_State_Set member", resp.State)
	}
	if resp.State != jobstate.Rendering {
		t.Fatalf("displayed state = %q, want the mirrored value %q", resp.State, jobstate.Rendering)
	}
	assertNoLeakyKeys(t, rec.Body.Bytes())
}

// TestJobMirrorDisplayHonestEmptyWhenNoStatusYet asserts the honest empty state:
// a no-op callback for a job the mirror has never seen (seq not newer than the
// zero entry) surfaces an empty state rather than fabricating a value. The watch
// desk renders "no status yet" from this, not an out-of-set placeholder.
//
// spec auto-clipper-replayforge-productization, RF-P5-004, Requirement 6.4.
func TestJobMirrorDisplayHonestEmptyWhenNoStatusYet(t *testing.T) {
	h, router := newJobMirrorTestHandler()
	// seq 0 is not newer than the zero-value entry's seq (0) -> idempotent no-op
	// for a never-seen job, so nothing is applied or displayed.
	rec := postCallback(t, router, testCallbackToken,
		`{"job_id":"job_display_2","state":"queued","seq":0,"updated_at":"2026-07-01T12:00:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 no-op, got %d (%s)", rec.Code, rec.Body.String())
	}
	resp := decodeCallbackResponse(t, rec)
	if resp.Applied {
		t.Fatal("never-seen job with non-newer seq must be a no-op")
	}
	if resp.State != "" {
		t.Fatalf("displayed state = %q, want honest empty state for unknown job", resp.State)
	}
	if _, ok := h.jobMirror().Get("job_display_2"); ok {
		t.Fatal("no-op callback must not create a mirror entry")
	}
}

// TestJobMirrorEntrySerializationIsMinimal locks the read-model DTO shape: the
// serialized Job_Mirror entry exposes only the minimal display fields and never
// carries token/secret/principal keys.
//
// spec auto-clipper-replayforge-productization, RF-P5-004, Requirement 6.4.
func TestJobMirrorEntrySerializationIsMinimal(t *testing.T) {
	m := NewJobMirror()
	if _, applied := m.Apply(StatusCallback{JobID: "job_display_3", State: jobstate.Complete, Seq: 7}); !applied {
		t.Fatal("expected in-set callback to apply")
	}
	entry, ok := m.Get("job_display_3")
	if !ok {
		t.Fatal("expected mirror entry")
	}
	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if !jobstate.InSet(entry.State) {
		t.Fatalf("stored/displayed state %q is not a Job_State_Set member", entry.State)
	}
	assertNoLeakyKeys(t, body)
}
