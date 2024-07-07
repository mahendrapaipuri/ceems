//go:build !nomemory
// +build !nomemory

package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemInfo(t *testing.T) {
	file, err := os.Open("testdata/proc/meminfo")
	require.NoError(t, err)
	defer file.Close()

	memInfo, err := parseMemInfo(file)
	require.NoError(t, err)

	want, got := 16042172416.0, memInfo["MemTotal_bytes"]
	assert.Equal(t, want, got)

	want, got = 16424894464.0, memInfo["DirectMap2M_bytes"]
	assert.Equal(t, want, got)
}
