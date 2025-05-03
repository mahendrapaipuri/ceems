package helper

import (
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
)

func TestTimeToTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		time     string
		expected int64
	}{
		{
			name:     "time string in CET location",
			time:     "2024-11-12T15:23:02+0100",
			expected: 1731421382000,
		},
		{
			name:     "time string in DST",
			time:     "2024-10-03T12:51:40+0200",
			expected: 1727952700000,
		},
		{
			name:     "time string in UTC",
			time:     "2024-11-12T15:23:02+0000",
			expected: 1731424982000,
		},
	}

	for _, test := range tests {
		timeStamp := TimeToTimestamp(base.DatetimezoneLayout, test.time)
		assert.Equal(t, test.expected, timeStamp, test.name)
	}

	// Check failure case
	timeStamp := TimeToTimestamp(base.DatetimezoneLayout, "Unknown")
	assert.Equal(t, int64(0), timeStamp)
}

func TestChunkBy(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected [][]int
		size     int
	}{
		{
			name:     "chunk size less than length",
			input:    []int{1, 2, 3, 4, 5, 6},
			size:     3,
			expected: [][]int{{1, 2, 3}, {4, 5, 6}},
		},
		{
			name:     "chunk size more than length",
			input:    []int{1, 2, 3, 4, 5, 6},
			size:     10,
			expected: [][]int{{1, 2, 3, 4, 5, 6}},
		},
		{
			name:     "chunk size 0",
			input:    []int{1, 2, 3, 4, 5, 6},
			size:     0,
			expected: [][]int{{1, 2, 3, 4, 5, 6}},
		},
	}

	for _, test := range tests {
		got := ChunkBy(test.input, test.size)
		assert.Equal(t, test.expected, got, test.name)
	}
}
