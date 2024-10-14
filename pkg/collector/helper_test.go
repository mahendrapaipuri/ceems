// Taken from node_exporter/collectors/paths_test.go and modified

package collector

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSantizeMetricName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "metric-name",
			expected: "metric_name",
		},
		{
			input:    "metric-name$",
			expected: "metric_name_",
		},
		{
			input:    "ns/metric-name",
			expected: "ns_metric_name",
		},
	}

	for _, tc := range testCases {
		got := SanitizeMetricName(tc.input)
		assert.Equal(t, tc.expected, got)
	}
}

func TestInode(t *testing.T) {
	absPath, err := filepath.Abs("testdata")
	require.NoError(t, err)

	inodeValue, err := inode(absPath)
	require.NoError(t, err)

	assert.Positive(t, inodeValue)
}
