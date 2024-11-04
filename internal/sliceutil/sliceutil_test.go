package sliceutil_test

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/sliceutil"
)

func TestFindValueByIndex(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		index    int
		expected int
	}{
		{
			name:     "Index within range",
			slice:    []int{1, 2, 3, 4, 5},
			index:    2,
			expected: 3,
		},
		{
			name:     "Index out of range",
			slice:    []int{1, 2, 3, 4, 5},
			index:    10,
			expected: 0, // default value of int
		},
		{
			name:     "Negative index",
			slice:    []int{1, 2, 3, 4, 5},
			index:    -1,
			expected: 0, // default value of int
		},
		{
			name:     "Empty slice",
			slice:    []int{},
			index:    0,
			expected: 0, // default value of int
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sliceutil.FindValueByIndex(tt.slice, tt.index)
			if result != tt.expected {
				t.Errorf("FindValueByIndex(%v, %d) = %v; want %v", tt.slice, tt.index, result, tt.expected)
			}
		})
	}
}
