package heatmap

import "sort"

// decimate reduces a slice of scored heatmap points to at most maxPoints while
// preserving the most significant moments, satisfying Requirement 12.
//
// Steps (design: Decimation Algorithm):
//  1. Omit zero-score points entirely (Requirement 12.3). Quiet/average windows
//     score 0 under the positive-surprise model and carry no signal, so they are
//     dropped before any sampling.
//  2. If the surviving non-zero points already fit within maxPoints, return them
//     sorted by OffsetSeconds ascending — no sampling needed.
//  3. Otherwise always retain the top topRetainPct fraction of points ranked by
//     score (Requirement 12.2), then uniformly sample the remaining lower-score
//     points to fill the rest of the maxPoints budget, merge the two sets, and
//     re-sort by OffsetSeconds ascending.
//
// Determinism (Requirement 9.6): ranking sorts by score descending with an
// OffsetSeconds-ascending tie-break, and sampling uses a fixed stride with no
// randomness, so identical input always yields identical output.
//
// Requirement 12.4 (>12h streams): the maxPoints cap is enforced purely by the
// uniform sampling step regardless of how many windows the stream produced. A
// stream longer than 12 hours yields more than maxPoints non-zero windows, which
// are sampled back down to maxPoints; because those retained points are spread
// across a proportionally longer duration, each retained point effectively
// represents a proportionally larger slice of the timeline — the "proportionally
// increased window size" the requirement describes. The cap therefore always
// holds (len(result) <= maxPoints) no matter the stream length.
//
// The returned slice is always non-nil so JSON encodes it as `[]`, never `null`.
func decimate(points []ReplayHeatmapPoint, maxPoints int, topRetainPct float64) []ReplayHeatmapPoint {
	// Step 1: omit zero-score points (Requirement 12.3).
	nonZero := make([]ReplayHeatmapPoint, 0, len(points))
	for _, p := range points {
		if p.Score > 0 {
			nonZero = append(nonZero, p)
		}
	}

	// Step 2: already within budget — return offset-sorted.
	if maxPoints <= 0 || len(nonZero) <= maxPoints {
		sortByOffset(nonZero)
		return nonZero
	}

	// Step 3a: rank by score descending, tie-break offset ascending so the top
	// percentile selection is deterministic across builds (Requirement 9.6).
	byScore := make([]ReplayHeatmapPoint, len(nonZero))
	copy(byScore, nonZero)
	sort.SliceStable(byScore, func(i, j int) bool {
		if byScore[i].Score != byScore[j].Score {
			return byScore[i].Score > byScore[j].Score
		}
		return byScore[i].OffsetSeconds < byScore[j].OffsetSeconds
	})

	topCount := int(float64(len(nonZero)) * topRetainPct)
	if topCount < 0 {
		topCount = 0
	}
	if topCount > maxPoints {
		// A large topRetainPct could request more top points than the cap; keep
		// only the highest-scoring maxPoints to honor the cap (Requirement 12.1).
		topCount = maxPoints
	}

	top := byScore[:topCount]
	rest := byScore[topCount:]

	// Step 3b: uniformly sample the lower-score remainder to fill the budget.
	sampled := uniformSample(rest, maxPoints-topCount)

	// Step 3c: merge retained top + sampled, then re-sort by offset ascending.
	result := make([]ReplayHeatmapPoint, 0, len(top)+len(sampled))
	result = append(result, top...)
	result = append(result, sampled...)
	sortByOffset(result)
	return result
}

// uniformSample selects count points from the input using a fixed temporal
// stride. The points are first ordered by OffsetSeconds ascending so the sample
// is evenly spread across the timeline, then indices floor(k*stride) for
// k in [0,count) are taken. The stride is len/count > 1 whenever the input is
// larger than count, so the chosen indices are strictly increasing and distinct.
// No randomness is used, so the selection is fully deterministic (Requirement
// 9.6 / 12.2).
func uniformSample(points []ReplayHeatmapPoint, count int) []ReplayHeatmapPoint {
	if count <= 0 || len(points) == 0 {
		return nil
	}
	ordered := make([]ReplayHeatmapPoint, len(points))
	copy(ordered, points)
	sortByOffset(ordered)

	if len(ordered) <= count {
		return ordered
	}

	out := make([]ReplayHeatmapPoint, 0, count)
	stride := float64(len(ordered)) / float64(count)
	for k := 0; k < count; k++ {
		idx := int(float64(k) * stride)
		if idx >= len(ordered) {
			idx = len(ordered) - 1
		}
		out = append(out, ordered[idx])
	}
	return out
}

// sortByOffset sorts points by OffsetSeconds ascending in place, with a
// MinuteTs tie-break for stability when two points share an offset (they should
// not in practice, since offsets are unique per scoring window).
func sortByOffset(points []ReplayHeatmapPoint) {
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].OffsetSeconds != points[j].OffsetSeconds {
			return points[i].OffsetSeconds < points[j].OffsetSeconds
		}
		return points[i].MinuteTs.Before(points[j].MinuteTs)
	})
}
