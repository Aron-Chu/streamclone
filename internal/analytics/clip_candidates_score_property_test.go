package analytics

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"

	"streamclone/internal/analytics/recap"
)

// scoreClampExpected mirrors the server-side clamp applied to every
// clip-candidate score (clipClampInt(moment.Score, 0, 100)). The test computes
// the expected value independently so it fails if the production clamp bounds
// ever drift.
func scoreClampExpected(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// buildScoreStream returns a minimal server-owned StreamRecord/StreamRecap pair
// carrying the given moments. The score is produced entirely server-side from
// these analytics signals — no client value participates.
func buildScoreStream(moments []recap.Moment) (*StreamRecord, recap.StreamRecap) {
	stream := &StreamRecord{
		StreamID:  "stream-score",
		Login:     "chan",
		Title:     "score run",
		Category:  "Just Chatting",
		StartedAt: time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC),
		VodID:     "vod-score",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-score",
		Login:           "chan",
		DurationSeconds: len(moments)*600 + 600,
		ClipCandidates:  moments,
	}
	return stream, rec
}

// TestPropClipCandidateScoreBoundedAndDeterministic asserts that the
// server-side clip-candidate score is (a) always clamped into [0,100] for
// arbitrary and extreme raw moment scores, (b) exactly the deterministic clamp
// of the source moment score, and (c) identical across repeated builds of the
// same input. This guards the `candidate_score` in the moment_context contract:
// it is computed on the server and cannot be pushed out of its bounded range.
//
// Feature: auto-clipper-replayforge-productization, Property P18-adjacent:
// server-side clip-candidate score is bounded [0,100] and deterministic.
//
// **Validates: Requirements 6.8**
func TestPropClipCandidateScoreBoundedAndDeterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "momentCount")
		moments := make([]recap.Moment, n)
		for i := 0; i < n; i++ {
			moments[i] = recap.Moment{
				// Unique, non-negative offsets keep every moment through the
				// build (no missing-window / duplicate-radius filtering) so the
				// output candidate i corresponds to moments[i].
				OffsetSeconds: i * 600,
				// Extreme range (well beyond [0,100]) exercises the clamp on
				// both ends and around the boundaries.
				Score:      rapid.IntRange(-100000, 100000).Draw(t, fmt.Sprintf("score%d", i)),
				Confidence: rapid.Float64Range(-5, 5).Draw(t, fmt.Sprintf("conf%d", i)),
				Reasons:    []string{"chat_spike"},
				ChatCount:  rapid.IntRange(0, 100000).Draw(t, fmt.Sprintf("chat%d", i)),
			}
		}

		stream, rec := buildScoreStream(moments)
		// MaxCandidates at the hard cap and no quality filters, so every
		// generated moment yields exactly one candidate in input order.
		opts := ClipCandidateBuildOptions{MaxCandidates: maxClipCandidateLimit}

		got := BuildClipCandidatesFromRecap(stream, rec, opts)
		if len(got) != n {
			t.Fatalf("len(candidates) = %d, want %d (all moments should survive with zero filters)", len(got), n)
		}

		for i, c := range got {
			if c.Score < 0 || c.Score > 100 {
				t.Fatalf("candidate %d score %d out of [0,100] (raw moment score %d)", i, c.Score, moments[i].Score)
			}
			if want := scoreClampExpected(moments[i].Score); c.Score != want {
				t.Fatalf("candidate %d score = %d, want clamp(%d)=%d", i, c.Score, moments[i].Score, want)
			}
			// The clamped bounded score is also mirrored into Signals-free
			// fields; confidence shares the same clamp discipline.
			if c.Confidence < 0 || c.Confidence > 1 {
				t.Fatalf("candidate %d confidence %v out of [0,1]", i, c.Confidence)
			}
		}

		// Determinism: an identical rebuild must produce identical scores.
		again := BuildClipCandidatesFromRecap(stream, rec, opts)
		if len(again) != len(got) {
			t.Fatalf("rebuild length %d != first build %d (non-deterministic)", len(again), len(got))
		}
		for i := range got {
			if again[i].Score != got[i].Score {
				t.Fatalf("candidate %d score not deterministic: first=%d again=%d", i, got[i].Score, again[i].Score)
			}
		}
	})
}

// TestPropClipCandidateScoreMonotonicInMomentScore asserts the intended
// monotonic relationship: a higher raw moment score never produces a lower
// clamped candidate score. This holds because the server-side clamp is a
// non-decreasing function, so stronger analytics spikes rank at least as high.
//
// Feature: auto-clipper-replayforge-productization, Property P18-adjacent:
// clip-candidate score is monotonic non-decreasing in the raw moment score.
//
// **Validates: Requirements 6.8**
func TestPropClipCandidateScoreMonotonicInMomentScore(t *testing.T) {
	scoreFor := func(raw int) int {
		stream, rec := buildScoreStream([]recap.Moment{{
			OffsetSeconds: 0,
			Score:         raw,
			Reasons:       []string{"chat_spike"},
		}})
		got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: maxClipCandidateLimit})
		if len(got) != 1 {
			t.Fatalf("len(candidates) = %d, want 1 for single moment (raw=%d)", len(got), raw)
		}
		return got[0].Score
	}

	rapid.Check(t, func(t *rapid.T) {
		a := rapid.IntRange(-100000, 100000).Draw(t, "a")
		b := rapid.IntRange(-100000, 100000).Draw(t, "b")
		if a > b {
			a, b = b, a
		}
		if sa, sb := scoreFor(a), scoreFor(b); sa > sb {
			t.Fatalf("monotonicity violated: score(%d)=%d > score(%d)=%d", a, sa, b, sb)
		}
	})
}

// TestBuildClipCandidatesScoreClampExamples covers explicit boundary examples
// alongside the property test: negative, zero, mid-range, exact bounds, and
// over-max raw scores all land at their clamped server-side value.
func TestBuildClipCandidatesScoreClampExamples(t *testing.T) {
	cases := []struct {
		raw  int
		want int
	}{
		{raw: -1000, want: 0},
		{raw: -1, want: 0},
		{raw: 0, want: 0},
		{raw: 50, want: 50},
		{raw: 100, want: 100},
		{raw: 101, want: 100},
		{raw: 100000, want: 100},
	}
	for _, tc := range cases {
		stream, rec := buildScoreStream([]recap.Moment{{
			OffsetSeconds: 0,
			Score:         tc.raw,
			Reasons:       []string{"chat_spike"},
		}})
		got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: maxClipCandidateLimit})
		if len(got) != 1 {
			t.Fatalf("raw=%d: len(candidates)=%d, want 1", tc.raw, len(got))
		}
		if got[0].Score != tc.want {
			t.Fatalf("raw=%d: score=%d, want %d", tc.raw, got[0].Score, tc.want)
		}
	}
}
