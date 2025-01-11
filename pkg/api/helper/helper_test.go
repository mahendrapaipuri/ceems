package helper

import (
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
)

func TestNodelistParser(t *testing.T) {
	tests := []struct {
		nodelist string
		expected []string
	}{
		{
			"compute-a-0", []string{"compute-a-0"},
		},
		{
			"compute-a-[0-1]", []string{"compute-a-0", "compute-a-1"},
		},
		{
			"compute-a-[0-1,5-6]", []string{"compute-a-0", "compute-a-1", "compute-a-5", "compute-a-6"},
		},
		{
			"compute-a-[0-1]-b-[3-4]",
			[]string{"compute-a-0-b-3", "compute-a-0-b-4", "compute-a-1-b-3", "compute-a-1-b-4"},
		},
		{
			"compute-a-[0-1,3,5-6]-b-[3-4,5,7-9]",
			[]string{
				"compute-a-0-b-3",
				"compute-a-0-b-4",
				"compute-a-0-b-5",
				"compute-a-0-b-7",
				"compute-a-0-b-8",
				"compute-a-0-b-9",
				"compute-a-1-b-3",
				"compute-a-1-b-4",
				"compute-a-1-b-5",
				"compute-a-1-b-7",
				"compute-a-1-b-8",
				"compute-a-1-b-9",
				"compute-a-3-b-3",
				"compute-a-3-b-4",
				"compute-a-3-b-5",
				"compute-a-3-b-7",
				"compute-a-3-b-8",
				"compute-a-3-b-9",
				"compute-a-5-b-3",
				"compute-a-5-b-4",
				"compute-a-5-b-5",
				"compute-a-5-b-7",
				"compute-a-5-b-8",
				"compute-a-5-b-9",
				"compute-a-6-b-3",
				"compute-a-6-b-4",
				"compute-a-6-b-5",
				"compute-a-6-b-7",
				"compute-a-6-b-8",
				"compute-a-6-b-9",
			},
		},
		{
			"compute-a-[0-1]-b-[3-4],compute-c,compute-d",
			[]string{
				"compute-a-0-b-3", "compute-a-0-b-4",
				"compute-a-1-b-3", "compute-a-1-b-4", "compute-c", "compute-d",
			},
		},
		{
			"compute-a-[0-2,5,7-9]-b-[3-4,7,9-12],compute-c,compute-d",
			[]string{
				"compute-a-0-b-3",
				"compute-a-0-b-4",
				"compute-a-0-b-7",
				"compute-a-0-b-9",
				"compute-a-0-b-10",
				"compute-a-0-b-11",
				"compute-a-0-b-12",
				"compute-a-1-b-3",
				"compute-a-1-b-4",
				"compute-a-1-b-7",
				"compute-a-1-b-9",
				"compute-a-1-b-10",
				"compute-a-1-b-11",
				"compute-a-1-b-12",
				"compute-a-2-b-3",
				"compute-a-2-b-4",
				"compute-a-2-b-7",
				"compute-a-2-b-9",
				"compute-a-2-b-10",
				"compute-a-2-b-11",
				"compute-a-2-b-12",
				"compute-a-5-b-3",
				"compute-a-5-b-4",
				"compute-a-5-b-7",
				"compute-a-5-b-9",
				"compute-a-5-b-10",
				"compute-a-5-b-11",
				"compute-a-5-b-12",
				"compute-a-7-b-3",
				"compute-a-7-b-4",
				"compute-a-7-b-7",
				"compute-a-7-b-9",
				"compute-a-7-b-10",
				"compute-a-7-b-11",
				"compute-a-7-b-12",
				"compute-a-8-b-3",
				"compute-a-8-b-4",
				"compute-a-8-b-7",
				"compute-a-8-b-9",
				"compute-a-8-b-10",
				"compute-a-8-b-11",
				"compute-a-8-b-12",
				"compute-a-9-b-3",
				"compute-a-9-b-4",
				"compute-a-9-b-7",
				"compute-a-9-b-9",
				"compute-a-9-b-10",
				"compute-a-9-b-11",
				"compute-a-9-b-12",
				"compute-c",
				"compute-d",
			},
		},
	}

	for _, test := range tests {
		output := NodelistParser(test.nodelist)
		assert.Equal(t, test.expected, output)
	}
}

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
