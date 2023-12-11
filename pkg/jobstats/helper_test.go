package jobstats

import (
	"reflect"
	"testing"
)

type nodelistParserTest struct {
	nodelist string
	expected []string
}

var nodelistParserTests = []nodelistParserTest{
	{
		"compute-a-[0-1]", []string{"compute-a-0", "compute-a-1"},
	},
	{
		"compute-a-[0-1]-b-[3-4]",
		[]string{"compute-a-0-b-3", "compute-a-0-b-4", "compute-a-1-b-3", "compute-a-1-b-4"},
	},
	{
		"compute-a-[0-1]-b-[3-4],compute-c,compute-d",
		[]string{"compute-a-0-b-3", "compute-a-0-b-4",
			"compute-a-1-b-3", "compute-a-1-b-4", "compute-c", "compute-d"},
	},
}

func TestNodelistParser(t *testing.T) {
	for _, test := range nodelistParserTests {
		if output := NodelistParser(test.nodelist); !reflect.DeepEqual(output, test.expected) {
			t.Errorf("Expected %q not equal to output %q", test.expected, output)
		}
	}
}
