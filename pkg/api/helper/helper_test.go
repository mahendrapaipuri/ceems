package helper

import (
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
)

type nodelistParserTest struct {
	nodelist string
	expected []string
}

var nodelistParserTests = []nodelistParserTest{
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

func TestNodelistParser(t *testing.T) {
	for _, test := range nodelistParserTests {
		output := NodelistParser(test.nodelist)
		assert.Equal(t, test.expected, output)
	}
}

func TestTimeToTimestamp(t *testing.T) {
	expectedTimeStamp := 1136239445000
	timeFormat := base.DatetimeLayout + "-0700"
	timeStamp := TimeToTimestamp(timeFormat, "2006-01-02T15:04:05-0700")
	assert.Equal(t, int64(expectedTimeStamp), timeStamp)

	// Check failure case
	timeStamp = TimeToTimestamp(timeFormat, "2006-01-0215:04:05-0700")
	assert.Equal(t, int64(0), timeStamp)
}

func TestChunkBy(t *testing.T) {
	expectedChunks := [][]int{{1, 2, 3}, {4, 5, 6}}
	inputSlice := []int{1, 2, 3, 4, 5, 6}
	chunks := ChunkBy(inputSlice, 3)
	assert.Equal(t, expectedChunks, chunks)
}
