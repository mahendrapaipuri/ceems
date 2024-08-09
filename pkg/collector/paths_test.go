// Taken from node_exporter/collectors/paths_test.go and modified

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSysPath(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.sysfs", "/sys"})
	require.NoError(t, err)

	got, want := sysFilePath("somefile"), "/sys/somefile"
	assert.Equal(t, want, got)

	got, want = sysFilePath("some/file"), "/sys/some/file"
	assert.Equal(t, want, got)
}

func TestCustomSysPath(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.sysfs", "./../some/./place/"})
	require.NoError(t, err)

	got, want := sysFilePath("somefile"), "../some/place/somefile"
	assert.Equal(t, want, got)

	got, want = sysFilePath("some/file"), "../some/place/some/file"
	assert.Equal(t, want, got)
}

func TestDefaultCgroupPath(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.cgroupfs", "/sys/fs/cgroup"})
	require.NoError(t, err)

	got, want := cgroupFilePath("somefile"), "/sys/fs/cgroup/somefile"
	assert.Equal(t, want, got)

	got, want = cgroupFilePath("some/file"), "/sys/fs/cgroup/some/file"
	assert.Equal(t, want, got)
}

func TestCustomCgroupPath(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.cgroupfs", "./../some/./place/"})
	require.NoError(t, err)

	got, want := cgroupFilePath("somefile"), "../some/place/somefile"
	assert.Equal(t, want, got)

	got, want = cgroupFilePath("some/file"), "../some/place/some/file"
	assert.Equal(t, want, got)
}
