package collector

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/user"
	"slices"
	"testing"

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
			"--collector.cgroups.force-version", "v2",
			"--collector.ebpf.io-metrics",
			"--collector.ebpf.network-metrics",
		},
	)
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	collector, err := NewEbpfCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), cgManager)
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

	err = collector.Update(metrics, nil, "")
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

func TestActiveCgroupsV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// ebpf opts
	opts := ebpfOpts{
		vfsStatsEnabled: true,
		netStatsEnabled: true,
	}

	c := ebpfCollector{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		cgroupManager: cgManager,

		opts:              opts,
		cgroupIDUUIDCache: make(map[uint64]string),
		cgroupPathIDCache: make(map[string]uint64),
	}

	// Discover cgroups
	cgroups, err := cgManager.discover()
	require.NoError(t, err)

	// Get active cgroups
	c.discoverCgroups(cgroups)

	assert.Len(t, c.activeCgroupInodes, 117)
	assert.Len(t, c.cgroupIDUUIDCache, 117)
	assert.Len(t, c.cgroupPathIDCache, 117)

	// Get cgroup IDs
	var uuids []string
	for _, uuid := range c.cgroupIDUUIDCache {
		if !slices.Contains(uuids, uuid) {
			uuids = append(uuids, uuid)
		}
	}

	expectedUUIDs := []string{
		"1009248", "1009249", "1009250",
		"2009248", "2009249", "2009250",
		"3009248", "3009249", "3009250",
	}

	assert.ElementsMatch(t, expectedUUIDs, uuids)
}

func TestActiveCgroupsV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--collector.cgroups.force-version", "v1",
		},
	)
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// ebpf opts
	opts := ebpfOpts{
		vfsStatsEnabled: true,
		netStatsEnabled: true,
	}

	c := ebpfCollector{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		cgroupManager: cgManager,

		opts:              opts,
		cgroupIDUUIDCache: make(map[uint64]string),
		cgroupPathIDCache: make(map[string]uint64),
	}

	// Discover cgroups
	cgroups, err := cgManager.discover()
	require.NoError(t, err)

	// Get active cgroups
	c.discoverCgroups(cgroups)

	assert.Len(t, c.activeCgroupInodes, 18)
	assert.Len(t, c.cgroupIDUUIDCache, 18)
	assert.Len(t, c.cgroupPathIDCache, 18)

	// Get cgroup IDs
	var uuids []string
	for _, uuid := range c.cgroupIDUUIDCache {
		if !slices.Contains(uuids, uuid) {
			uuids = append(uuids, uuid)
		}
	}

	expectedUUIDs := []string{
		"1009248", "1009249", "1009250",
		"2009248", "2009249", "2009250",
		"3009248", "3009249", "3009250",
	}

	assert.ElementsMatch(t, expectedUUIDs, uuids)
}

func TestVFSBPFObjects(t *testing.T) {
	tests := []struct {
		name    string
		procfs  string
		version string
		obj     string
	}{
		{
			name:    "kernel >= 6.2",
			procfs:  t.TempDir(),
			version: "Ubuntu 6.5.0-35.35~22.04.1-generic 6.5.13",
			obj:     "bpf_vfs.o",
		},
		{
			name:    "kernel > 5.11 and kernel < 6.2",
			procfs:  t.TempDir(),
			version: "Ubuntu 5.19.0-35.35~22.04.1-generic 5.19.13",
			obj:     "bpf_vfs_v62.o",
		},
		{
			name:    "kernel < 5.11",
			procfs:  t.TempDir(),
			version: "Ubuntu 5.6.0-35.35~22.04.1-generic 5.6.13",
			obj:     "bpf_vfs_v511.o",
		},
	}

	for _, test := range tests {
		err := os.WriteFile(test.procfs+"/version_signature", []byte(test.version), 0o600)
		require.NoError(t, err)

		*procfsPath = test.procfs

		currentKernelVer, err := KernelVersion()
		require.NoError(t, err)

		obj := bpfVFSObjs(currentKernelVer)

		assert.Equal(t, test.obj, obj, test.name)
	}
}

func TestNetBPFObjects(t *testing.T) {
	tests := []struct {
		name    string
		procfs  string
		version string
		obj     string
	}{
		{
			name:    "kernel >= 6.5",
			procfs:  t.TempDir(),
			version: "Ubuntu 6.9.0-35.35~22.04.1-generic 6.5.13",
			obj:     "bpf_network.o",
		},
		{
			name:    "kernel > 5.19 and kernel < 6.5",
			procfs:  t.TempDir(),
			version: "Ubuntu 5.27.0-35.35~22.04.1-generic 5.19.13",
			obj:     "bpf_network_v64.o",
		},
		{
			name:    "kernel < 5.19",
			procfs:  t.TempDir(),
			version: "Ubuntu 5.6.0-35.35~22.04.1-generic 5.6.13",
			obj:     "bpf_network_v519.o",
		},
	}

	for _, test := range tests {
		err := os.WriteFile(test.procfs+"/version_signature", []byte(test.version), 0o600)
		require.NoError(t, err)

		*procfsPath = test.procfs

		currentKernelVer, err := KernelVersion()
		require.NoError(t, err)

		obj := bpfNetObjs(currentKernelVer)

		assert.Equal(t, test.obj, obj, test.name)
	}
}
