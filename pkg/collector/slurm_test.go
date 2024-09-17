//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockGPUDevices() map[int]Device {
	devs := make(map[int]Device, 4)

	for i := 0; i <= 4; i++ {
		idxString := strconv.Itoa(i)
		devs[i] = Device{index: idxString, uuid: fmt.Sprintf("GPU-%d", i)}
	}

	return devs
}

func TestNewSlurmCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
			"--collector.slurm.swap-memory-metrics",
			"--collector.slurm.psi-metrics",
			"--collector.slurm.perf-hardware-events",
			"--collector.slurm.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewSlurmCollector(log.NewNopLogger())
	require.NoError(t, err)

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	err = collector.Update(metrics)
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

func TestSlurmJobPropsWithProlog(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
		idRegex:    slurmCgroupPathRegex,
		pathFilter: func(p string) bool {
			return strings.Contains(p, "/step_")
		},
	}

	c := slurmCollector{
		gpuDevs:       mockGPUDevices(),
		logger:        log.NewNopLogger(),
		cgroupManager: cgManager,
		jobPropsCache: make(map[string]props),
	}

	expectedProps := props{
		gpuOrdinals: []string{"0"},
		uuid:        "1009249",
	}

	metrics, err := c.discoverCgroups()
	require.NoError(t, err)

	var gotProps props

	for _, props := range metrics.jobProps {
		if props.uuid == expectedProps.uuid {
			gotProps = props
		}
	}

	assert.Equal(t, expectedProps, gotProps)
}

func TestSlurmJobPropsWithProcsFS(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--collector.cgroups.force-version", "v1",
		},
	)
	require.NoError(t, err)

	procFS, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Legacy,
		mountPoint: "testdata/sys/fs/cgroup/cpuacct/slurm",
		idRegex:    slurmCgroupPathRegex,
		pathFilter: func(p string) bool {
			return strings.Contains(p, "/step_")
		},
	}

	c := slurmCollector{
		cgroupManager: cgManager,
		gpuDevs:       mockGPUDevices(),
		logger:        log.NewNopLogger(),
		jobPropsCache: make(map[string]props),
		procFS:        procFS,
	}

	expectedProps := props{
		uuid:        "1009248",
		gpuOrdinals: []string{"2", "3"},
	}

	metrics, err := c.discoverCgroups()
	require.NoError(t, err)

	var gotProps props

	for _, props := range metrics.jobProps {
		if props.uuid == expectedProps.uuid {
			gotProps = props
		}
	}

	assert.Equal(t, expectedProps, gotProps)
}

func TestJobPropsCaching(t *testing.T) {
	path := t.TempDir()

	cgroupsPath := path + "/cgroups"
	err := os.Mkdir(cgroupsPath, 0o750)
	require.NoError(t, err)

	gpuMapFilePath := path + "/gpu-map"
	err = os.Mkdir(gpuMapFilePath, 0o750)
	require.NoError(t, err)

	_, err = CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", cgroupsPath,
			"--collector.slurm.gpu-job-map-path", gpuMapFilePath,
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Legacy,
		root:       cgroupsPath,
		idRegex:    slurmCgroupPathRegex,
		mountPoint: cgroupsPath + "/cpuacct/slurm",
		pathFilter: func(p string) bool {
			return false
		},
	}

	mockGPUDevs := mockGPUDevices()
	c := slurmCollector{
		cgroupManager: cgManager,
		logger:        log.NewNopLogger(),
		gpuDevs:       mockGPUDevs,
		jobPropsCache: make(map[string]props),
	}

	// Add cgroups
	for i := range 20 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Binds GPUs to first n jobs
	for igpu := range mockGPUDevs {
		err = os.WriteFile(fmt.Sprintf("%s/%d", gpuMapFilePath, igpu), []byte(strconv.FormatInt(int64(igpu), 10)), 0o600)
		require.NoError(t, err)
	}

	// Now call get metrics which should populate jobPropsCache
	_, err = c.discoverCgroups()
	require.NoError(t, err)

	// Check if jobPropsCache has 20 jobs and GPU ordinals are correct
	assert.Len(t, c.jobPropsCache, 20)

	for igpu := range mockGPUDevs {
		gpuIDString := strconv.FormatInt(int64(igpu), 10)
		assert.Equal(t, []string{gpuIDString}, c.jobPropsCache[gpuIDString].gpuOrdinals)
	}

	// Remove first 10 jobs and add new 10 more jobs
	for i := range 10 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.RemoveAll(dir)
		require.NoError(t, err)
	}

	for i := 19; i < 25; i++ {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Now call again get metrics which should populate jobPropsCache
	_, err = c.discoverCgroups()
	require.NoError(t, err)

	// Check if jobPropsCache has only 15 jobs and GPU ordinals are empty
	assert.Len(t, c.jobPropsCache, 15)

	for _, p := range c.jobPropsCache {
		assert.Empty(t, p.gpuOrdinals)
	}
}
