package collector

import (
	"context"
	"os/user"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// func mockVFSSpec() *ebpf.CollectionSpec {
// 	var mnt [64]uint8
// 	// mock mount
// 	copy(mnt[:], "/home/test")

// 	return &ebpf.CollectionSpec{
// 		Maps: map[string]*ebpf.MapSpec{
// 			"write_accumulator": {
// 				Type:       ebpf.Hash,
// 				KeySize:    68,
// 				ValueSize:  24,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: bpfVfsEventKey{
// 							Cid: uint32(1234),
// 							Mnt: mnt,
// 						},
// 						Value: bpfVfsRwEvent{
// 							Calls:  uint64(10),
// 							Bytes:  uint64(10000),
// 							Errors: uint64(1),
// 						},
// 					},
// 				},
// 			},
// 			"read_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: bpfVfsEventKey{
// 							Cid: uint32(1234),
// 							Mnt: mnt,
// 						},
// 						Value: bpfVfsRwEvent{
// 							Calls:  uint64(20),
// 							Bytes:  uint64(20000),
// 							Errors: uint64(2),
// 						},
// 					},
// 				},
// 			},
// 			"open_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: uint32(1234),
// 						Value: bpfVfsInodeEvent{
// 							Calls:  uint64(30),
// 							Errors: uint64(3),
// 						},
// 					},
// 				},
// 			},
// 			"create_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: uint32(1234),
// 						Value: bpfVfsInodeEvent{
// 							Calls:  uint64(40),
// 							Errors: uint64(4),
// 						},
// 					},
// 				},
// 			},
// 			"unlink_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: uint32(1234),
// 						Value: bpfVfsInodeEvent{
// 							Calls:  uint64(50),
// 							Errors: uint64(5),
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}
// }

// func mockNetSpec() *ebpf.CollectionSpec {
// 	var dev [16]uint8
// 	// mock mount
// 	copy(dev[:], "eno1")

// 	return &ebpf.CollectionSpec{
// 		Maps: map[string]*ebpf.MapSpec{
// 			"ingress_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: bpfNetEventKey{
// 							Cid: uint32(1234),
// 							Dev: dev,
// 						},
// 						Value: bpfNetEvent{
// 							Packets: uint64(10),
// 							Bytes:   uint64(10000),
// 						},
// 					},
// 				},
// 			},
// 			"egress_accumulator": {
// 				Type:       ebpf.Hash,
// 				MaxEntries: 1,
// 				Contents: []ebpf.MapKV{
// 					{
// 						Key: bpfNetEventKey{
// 							Cid: uint32(1234),
// 							Dev: dev,
// 						},
// 						Value: bpfNetEvent{
// 							Packets: uint64(20),
// 							Bytes:   uint64(20000),
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}
// }

func skipUnprivileged(t *testing.T) {
	t.Helper()

	// Get current user
	currentUser, err := user.Current()
	require.NoError(t, err)

	if currentUser.Uid != "0" {
		t.Skip("Skipping testing due to lack of privileges")
	}
}

func TestNewEbpfCollector(t *testing.T) {
	skipUnprivileged(t)

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.force-cgroups-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewEbpfCollector(log.NewNopLogger())
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

func TestActiveCgroupsV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	c := ebpfCollector{
		cgroupFS:     slurmCgroupFS(*cgroupfsPath, "", "v2"),
		logger:       log.NewNopLogger(),
		inodesMap:    make(map[uint64]string),
		inodesRevMap: make(map[string]uint64),
	}

	// Get active cgroups
	err = c.getActiveCgroups()
	require.NoError(t, err)

	assert.Len(t, c.activeCgroups, 3)
	assert.Len(t, c.inodesMap, 3)
	assert.Len(t, c.inodesRevMap, 3)

	// Get cgroup IDs
	var uuids []string
	for uuid := range c.inodesRevMap {
		uuids = append(uuids, uuid)
	}

	assert.ElementsMatch(t, []string{"1009248", "1009249", "1009250"}, uuids)
}

func TestActiveCgroupsV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	c := ebpfCollector{
		cgroupFS:     slurmCgroupFS(*cgroupfsPath, "cpuacct", "v1"),
		logger:       log.NewNopLogger(),
		inodesMap:    make(map[uint64]string),
		inodesRevMap: make(map[string]uint64),
	}

	// Get active cgroups
	err = c.getActiveCgroups()
	require.NoError(t, err)

	assert.Len(t, c.activeCgroups, 3)
	assert.Len(t, c.inodesMap, 3)
	assert.Len(t, c.inodesRevMap, 3)

	// Get cgroup IDs
	var uuids []string
	for uuid := range c.inodesRevMap {
		uuids = append(uuids, uuid)
	}

	assert.ElementsMatch(t, []string{"1009248", "1009249", "1009250"}, uuids)
}
