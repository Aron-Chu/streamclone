package heatmap

import (
	"testing"
	"time"
)

func mkPoint(offset, score int) ReplayHeatmapPoint {
	return ReplayHeatmapPoint{
		OffsetSeconds:   offset,
		DurationSeconds: defaultWindowSeconds,
		Score:           score,
		Reason:          ReasonChatSpike,
		MinuteTs:        time.Unix(int64(offset), 0).UTC(),
	}
}

// TestDecimateOmitsZeroScore verifies Requirement 12.3: zero-score points are
// dropped entirely even when the total is well under the cap.
func TestDecimateOmitsZeroScore(t *testing.T) {
	in := []ReplayHeatmapPoint{
		mkPoint(0, 0),
		mkPoint(60, 40),
		mkPoint(120, 0),
		mkPoint(180, 75),
		mkPoint(240, 0),
	}
	out := decimate(in, 720, 0.20)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (zero-score omitted)", len(out))
	}
	for _, p := range out {
		if p.Score == 0 {
			t.Errorf("zero-score point survived: offset %d", p.OffsetSeconds)
		}
	}
	// Offset-sorted ascending.
	if out[0].OffsetSeconds != 60 || out[1].OffsetSeconds != 180 {
		t.Errorf("not offset-sorted: %d, %d", out[0].OffsetSeconds, out[1].OffsetSeconds)
	}
}

// TestDecimateUnderCapKeepsAll verifies that when non-zero points fit the cap
// no sampling occurs and the output is offset-sorted.
func TestDecimateUnderCapKeepsAll(t *testing.T) {
	in := []ReplayHeatmapPoint{
		mkPoint(180, 10),
		mkPoint(60, 20),
		mkPoint(0, 30),
		mkPoint(120, 40),
	}
	out := decimate(in, 720, 0.20)
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
	for i := 1; i < len(out); i++ {
		if out[i].OffsetSeconds <= out[i-1].OffsetSeconds {
			t.Errorf("not strictly offset-ascending at %d", i)
		}
	}
}

// TestDecimateEnforcesCap verifies Requirement 12.1/12.2: the output never
// exceeds maxPoints and always retains the highest-scoring points.
func TestDecimateEnforcesCap(t *testing.T) {
	const total = 2000
	const maxPoints = 720
	in := make([]ReplayHeatmapPoint, total)
	for i := 0; i < total; i++ {
		// Scores 1..total so every point is non-zero and the top ones are the
		// highest offsets.
		in[i] = mkPoint(i*defaultWindowSeconds, (i%99)+1)
	}
	// Inject a clearly-top cluster with the maximum score.
	for i := 0; i < 50; i++ {
		in[i].Score = 100
	}

	out := decimate(in, maxPoints, 0.20)
	if len(out) > maxPoints {
		t.Fatalf("len = %d exceeds cap %d", len(out), maxPoints)
	}
	if len(out) != maxPoints {
		t.Fatalf("len = %d, want exactly %d when many non-zero points exist", len(out), maxPoints)
	}

	// All 50 max-score points must be retained (top-20% always kept).
	got := make(map[int]int)
	for _, p := range out {
		got[p.OffsetSeconds] = p.Score
	}
	for i := 0; i < 50; i++ {
		off := i * defaultWindowSeconds
		if got[off] != 100 {
			t.Errorf("top point at offset %d (score 100) was dropped", off)
		}
	}

	// Offset-sorted ascending.
	for i := 1; i < len(out); i++ {
		if out[i].OffsetSeconds <= out[i-1].OffsetSeconds {
			t.Fatalf("output not offset-ascending at %d", i)
		}
	}
}

// TestDecimateDeterministic verifies Requirement 9.6: identical input yields
// bit-for-bit identical output across repeated calls.
func TestDecimateDeterministic(t *testing.T) {
	const total = 1500
	in := make([]ReplayHeatmapPoint, total)
	for i := 0; i < total; i++ {
		in[i] = mkPoint(i*defaultWindowSeconds, (i*7%97)+1)
	}
	a := decimate(in, 720, 0.20)
	b := decimate(in, 720, 0.20)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].OffsetSeconds != b[i].OffsetSeconds || a[i].Score != b[i].Score {
			t.Fatalf("non-deterministic at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// TestDecimateLongStreamCap simulates a >12h stream (more than 720 minutes of
// non-zero windows) and confirms the 720-point cap holds (Requirement 12.4).
func TestDecimateLongStreamCap(t *testing.T) {
	// 16 hours of per-minute windows = 960 windows, all non-zero.
	const total = 16 * 60
	in := make([]ReplayHeatmapPoint, total)
	for i := 0; i < total; i++ {
		in[i] = mkPoint(i*defaultWindowSeconds, (i%50)+1)
	}
	out := decimate(in, 720, 0.20)
	if len(out) > 720 {
		t.Fatalf("long-stream output %d exceeds 720-point cap", len(out))
	}
	if len(out) != 720 {
		t.Fatalf("long-stream output %d, want exactly 720", len(out))
	}
}

// TestUniformSampleSpread verifies the sampler returns distinct, offset-ordered
// points spread across the input.
func TestUniformSampleSpread(t *testing.T) {
	in := make([]ReplayHeatmapPoint, 100)
	for i := range in {
		in[i] = mkPoint(i*defaultWindowSeconds, 1)
	}
	out := uniformSample(in, 10)
	if len(out) != 10 {
		t.Fatalf("len = %d, want 10", len(out))
	}
	seen := make(map[int]struct{})
	prev := -1
	for _, p := range out {
		if _, dup := seen[p.OffsetSeconds]; dup {
			t.Errorf("duplicate sample offset %d", p.OffsetSeconds)
		}
		seen[p.OffsetSeconds] = struct{}{}
		if p.OffsetSeconds <= prev {
			t.Errorf("samples not offset-ascending: %d after %d", p.OffsetSeconds, prev)
		}
		prev = p.OffsetSeconds
	}
}
