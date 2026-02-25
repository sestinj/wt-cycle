package cycle

import "sort"

// NextNum finds the first unused positive integer given a set of existing numbers.
// Uses gap-filling: if 1,2,4 are taken, returns 3.
func NextNum(existing []int) int {
	if len(existing) == 0 {
		return 1
	}
	sorted := make([]int, len(existing))
	copy(sorted, existing)
	sort.Ints(sorted)

	// Deduplicate and filter non-positive
	deduped := make([]int, 0, len(sorted))
	prev := -1
	for _, n := range sorted {
		if n > 0 && n != prev {
			deduped = append(deduped, n)
			prev = n
		}
	}

	next := 1
	for _, n := range deduped {
		if n == next {
			next++
		} else if n > next {
			break
		}
	}
	return next
}
