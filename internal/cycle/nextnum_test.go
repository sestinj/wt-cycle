package cycle

import "testing"

func TestNextNum(t *testing.T) {
	tests := []struct {
		name     string
		existing []int
		want     int
	}{
		{"empty", nil, 1},
		{"single_1", []int{1}, 2},
		{"contiguous", []int{1, 2, 3}, 4},
		{"gap_at_start", []int{2, 3}, 1},
		{"gap_in_middle", []int{1, 2, 4, 5}, 3},
		{"duplicates", []int{1, 1, 2, 2, 3}, 4},
		{"unsorted", []int{5, 3, 1, 4, 2}, 6},
		{"large_numbers", []int{100, 200, 300}, 1},
		{"zero_and_negative", []int{-1, 0, 2, 3}, 1},
		{"single_gap", []int{1, 3}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextNum(tt.existing)
			if got != tt.want {
				t.Errorf("NextNum(%v) = %d, want %d", tt.existing, got, tt.want)
			}
		})
	}
}
