package lib

import (
	"lib/assert"
	"testing"
)

func TestBinarySearch(t *testing.T) {
	arr := []int{1, 3, 5, 7}

	input := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	expectedPos := []int{-1, 0, 0, 1, 1, 2, 2, 3, 3}
	expectedFound := []bool{false, true, false, true, false, true, false, true, false}

	for i, v := range input {
		pos, found := BinarySearch(arr, v)
		assert.Equal(t, expectedPos[i], pos)
		assert.Equal(t, expectedFound[i], found)
	}
}
