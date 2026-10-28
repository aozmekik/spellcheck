// Package spell checks English words and suggests corrections.
package spell

// osa returns the optimal string alignment distance between a and b: the
// fewest insertions, deletions, substitutions and swaps of adjacent runes that
// turn a into b, with no substring edited twice. Distances above limit are
// reported as limit+1, which lets the computation stop early.
func osa(a, b []rune, limit int) int {
	if diff := len(a) - len(b); diff > limit || -diff > limit {
		return limit + 1
	}

	// Three rolling rows of the edit-distance table; swaps look two rows back.
	// Words are short, so the rows usually fit in an array on the stack: this
	// runs once per candidate, often a thousand times per lookup.
	width := len(b) + 1
	var stack [3 * 32]int
	rows := stack[:]
	if 3*width > len(stack) {
		rows = make([]int, 3*width)
	}
	twoBack, prev, curr := rows[:width], rows[width:2*width], rows[2*width:3*width]
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		rowMin := i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d := min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d = min(d, twoBack[j-2]+1)
			}
			curr[j] = d
			rowMin = min(rowMin, d)
		}
		// Later rows can't drop below this minimum: a swap reaches two rows
		// back, but that row is at most one smaller and the swap costs one.
		if rowMin > limit {
			return limit + 1
		}
		twoBack, prev, curr = prev, curr, twoBack
	}
	return min(prev[len(b)], limit+1)
}
