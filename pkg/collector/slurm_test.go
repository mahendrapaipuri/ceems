//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockGPUDevices() []Device {
	devs := make([]Device, 5)

	busIDs := []BusID{
		{domain: 0, bus: 7, device: 0, function: 0},
		{domain: 0, bus: 11, device: 0, function: 0},
		{domain: 0, bus: 72, device: 0, function: 0},
		{domain: 0, bus: 76, device: 0, function: 0},
		{domain: 0, bus: 77, device: 0, function: 0},
	}

	for i := range 4 {
		devs[i] = Device{
			globalIndex: strconv.Itoa(i),
			uuid:        fmt.Sprintf("GPU-%d", i),
			busID:       busIDs[i],
		}
	}

	return devs
}

func TestNewSlurmCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--path.sysfs", "testdata/sys",
			"--collector.slurm.swap-memory-metrics",
			"--collector.slurm.psi-metrics",
			"--collector.perf.hardware-events",
			"--collector.rdma.stats",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewSlurmCollector(slog.New(slog.NewTextHandler(io.Discard, nil)))
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

// func TestSlurmJobPropsWithProlog(t *testing.T) {
// 	_, err := CEEMSExporterApp.Parse(
// 		[]string{
// 			"--path.cgroupfs", "testdata/sys/fs/cgroup",
// 			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
// 			"--collector.cgroups.force-version", "v2",
// 		},
// 	)
// 	require.NoError(t, err)

// 	// cgroup Manager
// 	cgManager := &cgroupManager{
// 		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
// 		mode:       cgroups.Unified,
// 		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
// 		idRegex:    slurmCgroupPathRegex,
// 		ignoreCgroup: func(p string) bool {
// 			return strings.Contains(p, "/step_")
// 		},
// 	}

// 	c := slurmCollector{
// 		gpuDevs:       mockGPUDevices(),
// 		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
// 		cgroupManager: cgManager,
// 		jobPropsCache: make(map[string]jobProps),
// 	}

// 	expectedProps := jobProps{
// 		gpuOrdinals: []string{"0"},
// 		uuid:        "1009249",
// 	}

// 	metrics, err := c.jobMetrics()
// 	require.NoError(t, err)

// 	var gotProps jobProps

// 	for _, props := range metrics.jobProps {
// 		if props.uuid == expectedProps.uuid {
// 			gotProps = props
// 		}
// 	}

// 	assert.Equal(t, expectedProps, gotProps)
// }

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

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	c := slurmCollector{
		cgroupManager:    cgManager,
		gpuDevs:          mockGPUDevices(),
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		jobPropsCache:    make(map[string]jobProps),
		procFS:           procFS,
		securityContexts: make(map[string]*security.SecurityContext),
	}

	// Add dummy security context
	c.securityContexts[slurmReadProcCtx], err = security.NewSecurityContext(
		slurmReadProcCtx,
		nil,
		readProcEnvirons,
		c.logger,
	)
	require.NoError(t, err)

	expectedProps := []jobProps{
		{
			uuid:        "1009248",
			gpuOrdinals: []string{"2", "3"},
		},
		{
			uuid:        "1009249",
			gpuOrdinals: []string{"0"},
		},
		{
			uuid:        "1009250",
			gpuOrdinals: []string{"1"},
		},
	}

	metrics, err := c.jobMetrics()
	require.NoError(t, err)

	assert.Equal(t, expectedProps, metrics.jobProps)
}

func TestJobPropsCaching(t *testing.T) {
	path := t.TempDir()

	cgroupsPath := path + "/cgroups"
	err := os.Mkdir(cgroupsPath, 0o750)
	require.NoError(t, err)

	procFS := path + "/proc"
	err = os.Mkdir(procFS, 0o750)
	require.NoError(t, err)

	fs, err := procfs.NewFS(procFS)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		fs:         fs,
		mode:       cgroups.Legacy,
		root:       cgroupsPath,
		idRegex:    slurmCgroupPathRegex,
		mountPoint: cgroupsPath + "/cpuacct/slurm",
		isChild: func(p string) bool {
			return false
		},
	}

	mockGPUDevs := mockGPUDevices()
	c := slurmCollector{
		cgroupManager:    cgManager,
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		gpuDevs:          mockGPUDevs,
		jobPropsCache:    make(map[string]jobProps),
		securityContexts: make(map[string]*security.SecurityContext),
	}

	// Add dummy security context
	c.securityContexts[slurmReadProcCtx], err = security.NewSecurityContext(
		slurmReadProcCtx,
		nil,
		readProcEnvirons,
		c.logger,
	)
	require.NoError(t, err)

	// Add cgroups
	for i := range 20 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)

		err = os.WriteFile(
			dir+"/cgroup.procs",
			[]byte(fmt.Sprintf("%d\n", i)),
			0o600,
		)
		require.NoError(t, err)
	}

	// Binds GPUs to first n jobs
	for igpu := range mockGPUDevs {
		dir := fmt.Sprintf("%s/%d", procFS, igpu)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)

		err = os.WriteFile(
			dir+"/environ",
			[]byte(strings.Join([]string{fmt.Sprintf("SLURM_JOB_ID=%d", igpu), fmt.Sprintf("SLURM_JOB_GPUS=%d", igpu)}, "\000")+"\000"),
			0o600,
		)
		require.NoError(t, err)
	}

	// Now call get metrics which should populate jobPropsCache
	_, err = c.jobMetrics()
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
	_, err = c.jobMetrics()
	require.NoError(t, err)

	// Check if jobPropsCache has only 15 jobs and GPU ordinals are empty
	assert.Len(t, c.jobPropsCache, 15)

	for _, p := range c.jobPropsCache {
		assert.Empty(t, p.gpuOrdinals)
	}
}
