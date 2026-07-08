// Package jobstate defines the canonical Job_State_Set shared by the
// Streamclone Job_Mirror callback handler and its tests
// (spec auto-clipper-replayforge-productization, RF-P1-008,
// Requirements 2.2, 6.4).
//
// This is the single Go source of truth for the set of legal Job_State
// values. It mirrors the ReplayForge Python state set shape-for-shape so both
// sides of the HTTP boundary agree on which states are legal. Any state not in
// this set is rejected by the mirror callback handler and is never applied to
// or displayed from the Job_Mirror (Property 2).
package jobstate

// Canonical Job_State_Set members. The mirror handler and tests both consume
// these constants rather than re-declaring string literals.
const (
	Queued            = "queued"
	ValidatingSource  = "validating_source"
	DownloadingSource = "downloading_source"
	Transcribing      = "transcribing"
	ReadyForEdit      = "ready_for_edit"
	Rendering         = "rendering"
	Rendered          = "rendered"
	UploadingArtifact = "uploading_artifact"
	Complete          = "complete"
	Failed            = "failed"
	RetryableFailed   = "retryable_failed"
	Expired           = "expired"
	SourceUnavailable = "source_unavailable"
	AuthRequired      = "auth_required"
	VODUnavailable    = "vod_unavailable"
)

// orderedStates preserves the design-document ordering so All returns a stable,
// human-readable sequence. It is the authoritative enumeration of the set.
var orderedStates = []string{
	Queued,
	ValidatingSource,
	DownloadingSource,
	Transcribing,
	ReadyForEdit,
	Rendering,
	Rendered,
	UploadingArtifact,
	Complete,
	Failed,
	RetryableFailed,
	Expired,
	SourceUnavailable,
	AuthRequired,
	VODUnavailable,
}

// membership is the lookup set derived from orderedStates. It is built once at
// package init and never mutated, so InSet is safe for concurrent use.
var membership = func() map[string]struct{} {
	m := make(map[string]struct{}, len(orderedStates))
	for _, s := range orderedStates {
		m[s] = struct{}{}
	}
	return m
}()

// InSet reports whether state is an exact member of the Job_State_Set. The
// match is exact: callers are responsible for trimming surrounding whitespace
// before calling if their transport can introduce it. Case and spelling must
// match a canonical member, so out-of-set values are rejected (Property 2).
func InSet(state string) bool {
	_, ok := membership[state]
	return ok
}

// All returns a copy of the canonical Job_State_Set in design-document order.
// A fresh slice is returned each call so callers cannot mutate the set.
func All() []string {
	out := make([]string, len(orderedStates))
	copy(out, orderedStates)
	return out
}
