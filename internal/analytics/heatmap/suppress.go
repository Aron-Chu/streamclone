package heatmap

// suppressPeaks performs non-max suppression on a score sequence, retaining only
// local maxima within the given radius (Requirement 9.5). Scores below the
// threshold are left untouched; a score at or above the threshold is zeroed
// unless it is the local maximum across the window [i-radius, i+radius].
//
// Ties are broken deterministically by index: among equal scores within the
// radius, only the lowest-index occurrence is retained. This guarantees that no
// two scores >= threshold remain within radius of each other in the output
// (Property 9). All comparisons use the original input scores, so suppression
// decisions are independent of evaluation order.
//
// A non-positive radius means each qualifying score only competes with itself
// and is therefore always retained. The input slice is not mutated; a new slice
// is returned.
func suppressPeaks(scores []float64, threshold int, radius int) []float64 {
	result := make([]float64, len(scores))
	copy(result, scores)

	t := float64(threshold)
	for i := range scores {
		if scores[i] < t {
			continue
		}

		lo := max(0, i-radius)
		hi := min(len(scores)-1, i+radius)

		isMax := true
		for j := lo; j <= hi; j++ {
			if j == i {
				continue
			}
			// A strictly larger neighbour, or an equal neighbour at a lower
			// index, outranks this window. Tie-breaking on index ensures only
			// one of a set of equal peaks survives within the radius.
			if scores[j] > scores[i] || (scores[j] == scores[i] && j < i) {
				isMax = false
				break
			}
		}

		if !isMax {
			result[i] = 0
		}
	}
	return result
}
