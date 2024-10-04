//go:build !nordma
// +build !nordma

package collector

import (
	"context"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/sysfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRDMACollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.sysfs", "testdata/sys",
		"--collector.rdma.stats",
		"--collector.rdma.cmd", "testdata/rdma",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
		idRegex:    slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	collector, err := NewPerfCollector(log.NewNopLogger(), cgManager)
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

func TestDevMR(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Unified,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// Instantiate a new Proc FS
	procfs, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := rdmaCollector{
		logger:        log.NewNopLogger(),
		rdmaCmd:       "testdata/rdma",
		procfs:        procfs,
		cgroupManager: cgManager,
	}

	// Get cgroup IDs
	procCgroup, err := c.procCgroups()
	require.NoError(t, err)

	expectedMRs := map[string]*mr{
		"1320003": {2, 4194304, "mlx5_0"},
		"4824887": {2, 4194304, "mlx5_0"},
	}

	// Get MR stats
	mrs, err := c.devMR(procCgroup)
	require.NoError(t, err)
	assert.Equal(t, expectedMRs, mrs)
}

func TestDevCQ(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Unified,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// Instantiate a new Proc FS
	procfs, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := rdmaCollector{
		logger:        log.NewNopLogger(),
		rdmaCmd:       "testdata/rdma",
		procfs:        procfs,
		cgroupManager: cgManager,
	}

	// Get cgroup IDs
	procCgroup, err := c.procCgroups()
	require.NoError(t, err)

	expectedCQs := map[string]*cq{
		"1320003": {2, 8190, "mlx5_0"},
		"4824887": {2, 8190, "mlx5_0"},
	}

	// Get MR stats
	cqs, err := c.devCQ(procCgroup)
	require.NoError(t, err)
	assert.Equal(t, expectedCQs, cqs)
}

func TestLinkQP(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Unified,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// Instantiate a new Proc FS
	procfs, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := rdmaCollector{
		logger:        log.NewNopLogger(),
		rdmaCmd:       "testdata/rdma",
		procfs:        procfs,
		cgroupManager: cgManager,
		qpModes:       map[string]bool{"mlx5_0": true},
		hwCounters:    []string{"rx_write_requests", "rx_read_requests"},
	}

	// Get cgroup IDs
	procCgroup, err := c.procCgroups()
	require.NoError(t, err)

	expected := map[string]*qp{
		"1320003": {16, "mlx5_0", "1", map[string]uint64{"rx_read_requests": 0, "rx_write_requests": 41988882}},
		"4824887": {16, "mlx5_0", "1", map[string]uint64{"rx_write_requests": 0, "rx_read_requests": 0}},
	}

	// Get MR stats
	qps, err := c.linkQP(procCgroup)
	require.NoError(t, err)
	assert.Equal(t, expected, qps)
}

func TestLinkCountersSysWide(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Unified,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// Instantiate a new Proc FS
	sysfs, err := sysfs.NewFS(*sysPath)
	require.NoError(t, err)

	c := rdmaCollector{
		logger:        log.NewNopLogger(),
		sysfs:         sysfs,
		cgroupManager: cgManager,
		hwCounters:    []string{"rx_write_requests", "rx_read_requests"},
	}

	expected := map[string]map[string]uint64{
		"hfi1_0/1": {
			"port_constraint_errors_received_total":    0x0,
			"port_constraint_errors_transmitted_total": 0x0,
			"port_data_received_bytes_total":           0x1416445f428,
			"port_data_transmitted_bytes_total":        0xfec563343c,
			"port_discards_received_total":             0x0,
			"port_discards_transmitted_total":          0x0,
			"port_errors_received_total":               0x0,
			"port_packets_received_total":              0x2607abd3,
			"port_packets_transmitted_total":           0x21dfdb88,
			"state_id":                                 0x4,
		},
		"mlx4_0/1": {
			"port_constraint_errors_received_total":    0x0,
			"port_constraint_errors_transmitted_total": 0x0,
			"port_data_received_bytes_total":           0x21194bae4,
			"port_data_transmitted_bytes_total":        0x18b043df3c,
			"port_discards_received_total":             0x0,
			"port_discards_transmitted_total":          0x0,
			"port_errors_received_total":               0x0,
			"port_packets_received_total":              0x532195c,
			"port_packets_transmitted_total":           0x51c32e2,
			"state_id":                                 0x4,
		},
		"mlx4_0/2": {
			"port_constraint_errors_received_total":    0x0,
			"port_constraint_errors_transmitted_total": 0x0,
			"port_data_received_bytes_total":           0x24a9d24c0,
			"port_data_transmitted_bytes_total":        0x18b7b6d468,
			"port_discards_received_total":             0x0,
			"port_discards_transmitted_total":          0x0,
			"port_errors_received_total":               0x0,
			"port_packets_received_total":              0x5531960,
			"port_packets_transmitted_total":           0x5484702,
			"state_id":                                 0x4,
		},
		"mlx5_0/1": {
			"port_constraint_errors_received_total":    0x0,
			"port_constraint_errors_transmitted_total": 0x0,
			"port_data_received_bytes_total":           0x10e1a85288,
			"port_data_transmitted_bytes_total":        0xa7aeb10cfc0,
			"port_discards_received_total":             0x0,
			"port_discards_transmitted_total":          0x0,
			"port_errors_received_total":               0x0,
			"port_packets_received_total":              0x204c9520,
			"port_packets_transmitted_total":           0x28a29aec4,
			"state_id":                                 0x4,
		},
	}

	// Get MR stats
	counters, err := c.linkCountersSysWide()
	require.NoError(t, err)
	assert.Equal(t, expected, counters)
}
