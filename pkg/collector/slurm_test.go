//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"fmt"
	"os"
	"slices"
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

func mockGPUDevices(numDevs int, migInst []int) []Device {
	devs := make([]Device, numDevs)

	migInstances := []GPUInstance{
		{InstanceIndex: 0, GPUInstID: 2},
		{InstanceIndex: 1, GPUInstID: 7},
		{InstanceIndex: 2, GPUInstID: 8},
		{InstanceIndex: 3, GPUInstID: 9},
	}

	idev := 0

	for i := range numDevs {
		devs[i] = Device{
			Minor: strconv.Itoa(i),
			UUID:  fmt.Sprintf("GPU-%d", i),
		}

		if slices.Contains(migInst, i) {
			for _, inst := range migInstances {
				inst.Index = strconv.Itoa(idev)
				devs[i].Instances = append(devs[i].Instances, inst)
				idev++
			}
		} else {
			devs[i].Index = strconv.Itoa(idev)
			idev++
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
			"--collector.gpu.type", "nvidia",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
			"--collector.slurm.gres-config-file", "testdata/gres.conf",
		},
	)
	require.NoError(t, err)

	collector, err := NewSlurmCollector(noOpLogger)
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
// 		logger:     noOpLogger,
// 		mode:       cgroups.Unified,
// 		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
// 		idRegex:    slurmCgroupPathRegex,
// 		ignoreCgroup: func(p string) bool {
// 			return strings.Contains(p, "/step_")
// 		},
// 	}

// 	c := slurmCollector{
// 		gpuDevs:       mockGPUDevices(),
// 		logger:        noOpLogger,
// 		cgroupManager: cgManager,
// 		jobPropsCache: make(map[string]jobProps),
// 	}

// 	expectedProps := jobProps{
// 		deviceIDs: []string{"0"},
// 		UUID:        "1009249",
// 	}

// 	metrics, err := c.jobMetrics()
// 	require.NoError(t, err)

// 	var gotProps jobProps

// 	for _, props := range metrics.jobProps {
// 		if props.UUID == expectedProps.UUID {
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
	cgManager, err := NewCgroupManager(slurm, noOpLogger)
	require.NoError(t, err)

	// GPU SMI
	gpuSMI := &GPUSMI{
		logger:  noOpLogger,
		Devices: mockGPUDevices(4, nil),
	}

	c := slurmCollector{
		cgroupManager:    cgManager,
		gpuSMI:           gpuSMI,
		logger:           noOpLogger,
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

	expectedJobIDs := []string{
		"1009248", "1009249", "1009250", "2009248", "2009249",
		"2009250", "3009248", "3009249", "3009250",
	}

	expectedJobDeviceMappers := map[string][]ComputeUnit{
		"0": {{UUID: "1009249"}},
		"1": {{UUID: "1009249"}},
		"2": {{UUID: "1009248"}},
		"3": {{UUID: "1009250"}},
	}

	expectedCgroupHosts := map[string]string{
		"2009248": "host0",
		"2009249": "host0",
		"2009250": "host0",
		"3009248": "host1",
		"3009249": "host1",
		"3009250": "host1",
	}

	cgroups, err := c.jobCgroups()
	require.NoError(t, err)

	for _, cgroup := range cgroups {
		assert.Equal(t, expectedCgroupHosts[cgroup.uuid], cgroup.hostname, cgroup.uuid)
	}

	assert.Equal(t, expectedJobIDs, c.previousJobIDs)

	for _, gpu := range c.gpuSMI.Devices {
		assert.Equal(t, expectedJobDeviceMappers[gpu.Index], gpu.ComputeUnits, "GPU %s", gpu.Index)
	}
}

func TestGRESSharesUpdate(t *testing.T) {
	tests := []struct {
		name     string
		migInst  []int
		jobGRES  map[*gres]string
		expected map[string][]ComputeUnit
	}{
		{
			name: "Few physical GPUs are used",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0"}, numShares: 8}: "1000",
				{deviceIDs: []string{"3"}, numShares: 8}: "999",
			},
			expected: map[string][]ComputeUnit{
				"0": {{UUID: "1000", NumShares: 8}},
				"3": {{UUID: "999", NumShares: 8}},
			},
		},
		{
			name: "All physical GPUs are used",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0"}, numShares: 8}: "1000",
				{deviceIDs: []string{"3"}, numShares: 8}: "999",
				{deviceIDs: []string{"1"}, numShares: 8}: "1001",
				{deviceIDs: []string{"2"}, numShares: 8}: "1002",
			},
			expected: map[string][]ComputeUnit{
				"0": {{UUID: "1000", NumShares: 8}},
				"3": {{UUID: "999", NumShares: 8}},
				"1": {{UUID: "1001", NumShares: 8}},
				"2": {{UUID: "1002", NumShares: 8}},
			},
		},
		{
			name: "Jobs using multiple physical GPUs",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 16}: "1000",
				{deviceIDs: []string{"1"}, numShares: 8}:       "1001",
				{deviceIDs: []string{"2"}, numShares: 8}:       "1002",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 8},
				},
				"1": {
					{UUID: "1001", NumShares: 8},
				},
				"2": {
					{UUID: "1002", NumShares: 8},
				},
				"3": {
					{UUID: "1000", NumShares: 8},
				},
			},
		},
		{
			name: "Jobs using multiple physical GPUs with unequal shares",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 12}: "1000",
				{deviceIDs: []string{"0"}, numShares: 4}:       "1001",
				{deviceIDs: []string{"1"}, numShares: 8}:       "1003",
				{deviceIDs: []string{"2"}, numShares: 8}:       "1002",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 4},
					{UUID: "1001", NumShares: 4},
				},
				"1": {
					{UUID: "1003", NumShares: 8},
				},
				"2": {
					{UUID: "1002", NumShares: 8},
				},
				"3": {
					{UUID: "1000", NumShares: 8},
				},
			},
		},
		{
			name: "Jobs using multiple physical GPUs with unequal shares",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 12}: "1000",
				{deviceIDs: []string{"0"}, numShares: 4}:       "1001",
				{deviceIDs: []string{"1", "2"}, numShares: 10}: "1003",
				{deviceIDs: []string{"2"}, numShares: 6}:       "1002",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 4},
					{UUID: "1001", NumShares: 4},
				},
				"1": {
					{UUID: "1003", NumShares: 8},
				},
				"2": {
					{UUID: "1002", NumShares: 6},
					{UUID: "1003", NumShares: 2},
				},
				"3": {
					{UUID: "1000", NumShares: 8},
				},
			},
		},
		{
			name: "Jobs using only multiple physical GPUs with unequal shares",
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 12}: "1000",
				{deviceIDs: []string{"1", "2"}, numShares: 10}: "1003",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 8},
				},
				"1": {
					{UUID: "1003", NumShares: 8},
				},
				"2": {
					{UUID: "1003", NumShares: 2},
				},
				"3": {
					{UUID: "1000", NumShares: 4},
				},
			},
		},
		{
			name:    "Jobs using only multiple MIG GPUs with unequal shares",
			migInst: []int{0, 1, 2, 3},
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 18}:   "1000",
				{deviceIDs: []string{"1", "2"}, numShares: 20}:   "1003",
				{deviceIDs: []string{"10"}, numShares: 2}:        "4003",
				{deviceIDs: []string{"10", "12"}, numShares: 20}: "2003",
				{deviceIDs: []string{"13", "14"}, numShares: 24}: "3003",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 16},
				},
				"1": {
					{UUID: "1003", NumShares: 16},
				},
				"2": {
					{UUID: "1003", NumShares: 4},
				},
				"3": {
					{UUID: "1000", NumShares: 2},
				},
				"10": {
					{UUID: "2003", NumShares: 14},
					{UUID: "4003", NumShares: 2},
				},
				"12": {
					{UUID: "2003", NumShares: 6},
				},
				"13": {
					{UUID: "3003", NumShares: 16},
				},
				"14": {
					{UUID: "3003", NumShares: 8},
				},
			},
		},
		{
			name:    "Jobs using mixed physical and MIG GPUs with unequal shares",
			migInst: []int{0, 3},
			jobGRES: map[*gres]string{
				{deviceIDs: []string{"0", "3"}, numShares: 28}: "1000",
				{deviceIDs: []string{"3"}, numShares: 4}:       "1004",
				{deviceIDs: []string{"1", "2"}, numShares: 30}: "1003",
				{deviceIDs: []string{"1"}, numShares: 2}:       "1005",
				{deviceIDs: []string{"4"}, numShares: 2}:       "4003",
				{deviceIDs: []string{"4"}, numShares: 4}:       "4005",
				{deviceIDs: []string{"5"}, numShares: 6}:       "5003",
				{deviceIDs: []string{"5"}, numShares: 1}:       "5006",
				{deviceIDs: []string{"6", "8"}, numShares: 20}: "2003",
				{deviceIDs: []string{"7", "9"}, numShares: 24}: "3003",
			},
			expected: map[string][]ComputeUnit{
				"0": {
					{UUID: "1000", NumShares: 16},
				},
				"1": {
					{UUID: "1005", NumShares: 2},
					{UUID: "1003", NumShares: 14},
				},
				"2": {
					{UUID: "1003", NumShares: 16},
				},
				"3": {
					{UUID: "1004", NumShares: 4},
					{UUID: "1000", NumShares: 12},
				},
				"4": {
					{UUID: "4003", NumShares: 2},
					{UUID: "4005", NumShares: 4},
				},
				"5": {
					{UUID: "5003", NumShares: 6},
					{UUID: "5006", NumShares: 1},
				},
				"6": {
					{UUID: "2003", NumShares: 16},
				},
				"7": {
					{UUID: "3003", NumShares: 16},
				},
				"8": {
					{UUID: "2003", NumShares: 4},
				},
				"9": {
					{UUID: "3003", NumShares: 8},
				},
			},
		},
	}

	for _, test := range tests {
		gpus := mockGPUDevices(4, test.migInst)

		// Update AvailableShares in GPUs
		for igpu := range gpus {
			gpus[igpu].AvailableShares = 8
			for iinst := range gpus[igpu].Instances {
				gpus[igpu].Instances[iinst].AvailableShares = 16
			}
		}

		c := slurmCollector{
			gpuSMI:           &GPUSMI{logger: noOpLogger, Devices: gpus},
			logger:           noOpLogger,
			securityContexts: make(map[string]*security.SecurityContext),
		}

		var gres []*gres
		for g, id := range test.jobGRES {
			gres = append(gres, g)

			for igpu, gpu := range c.gpuSMI.Devices {
				for _, index := range g.deviceIDs {
					if gpu.Index == index {
						c.gpuSMI.Devices[igpu].ComputeUnits = append(c.gpuSMI.Devices[igpu].ComputeUnits, ComputeUnit{UUID: id})
					}

					for iinst, inst := range gpu.Instances {
						if inst.Index == index {
							c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits = append(c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits, ComputeUnit{UUID: id})
						}
					}
				}
			}
		}

		c.updateGRESShares(gres, test.jobGRES)

		for _, gpu := range c.gpuSMI.Devices {
			if gpu.Index != "" {
				assert.ElementsMatch(t, test.expected[gpu.Index], gpu.ComputeUnits, test.name+gpu.Index)
			} else {
				for _, inst := range gpu.Instances {
					assert.ElementsMatch(t, test.expected[inst.Index], inst.ComputeUnits, test.name+inst.Index)
				}
			}
		}
	}
}

func TestJobDevicesCaching(t *testing.T) {
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
		logger:      noOpLogger,
		fs:          fs,
		mode:        cgroups.Legacy,
		root:        cgroupsPath,
		idRegex:     slurmCgroupV1PathRegex,
		mountPoints: []string{cgroupsPath + "/cpuacct/slurm"},
		isChild: func(p string) bool {
			return false
		},
	}

	mockGPUDevs := mockGPUDevices(5, []int{1})

	// Add shares for physical GPUs
	for igpu := range mockGPUDevs {
		if len(mockGPUDevs[igpu].Instances) > 0 {
			continue
		}

		mockGPUDevs[igpu].AvailableShares = 5
	}

	c := slurmCollector{
		cgroupManager:    cgManager,
		logger:           noOpLogger,
		gpuSMI:           &GPUSMI{logger: noOpLogger, Devices: mockGPUDevs},
		securityContexts: make(map[string]*security.SecurityContext),
		shardEnabled:     true,
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

	// Fake jobs
	mockJobs := []gres{
		{deviceIDs: []string{"0"}, numShares: 2},
		{deviceIDs: []string{"1", "2"}},
		{deviceIDs: []string{"3"}},
		{deviceIDs: []string{"4"}},
		{deviceIDs: []string{"0"}, numShares: 3},
		{deviceIDs: []string{"5", "6"}, numShares: 7},
		{deviceIDs: []string{"7"}, numShares: 3},
	}

	// Binds GPUs to first n jobs
	for ijob, gres := range mockJobs {
		dir := fmt.Sprintf("%s/%d", procFS, ijob)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)

		envs := []string{fmt.Sprintf("SLURM_JOB_ID=%d", ijob), "SLURM_JOB_GPUS=" + strings.Join(gres.deviceIDs, ",")}

		if gres.numShares > 0 {
			envs = append(envs, fmt.Sprintf("SLURM_SHARDS_ON_NODE=%d", gres.numShares))
		}

		err = os.WriteFile(
			dir+"/environ",
			[]byte(strings.Join(envs, "\000")+"\000"),
			0o600,
		)
		require.NoError(t, err)
	}

	// Now call get metrics which should populate jobPropsCache
	_, err = c.jobCgroups()
	require.NoError(t, err)

	// Check if jobPropsCache has 20 jobs and GPU ordinals are correct
	assert.Len(t, c.previousJobIDs, 20)

	expected := map[string][]ComputeUnit{
		"0": {{UUID: "0", NumShares: 2}, {UUID: "4", NumShares: 3}},
		"1": {{UUID: "1"}},
		"2": {{UUID: "1"}},
		"3": {{UUID: "2"}},
		"4": {{UUID: "3"}},
		"5": {{UUID: "5", NumShares: 5}},
		"6": {{UUID: "5", NumShares: 2}},
		"7": {{UUID: "6", NumShares: 3}},
	}

	for _, gpu := range mockGPUDevs {
		if gpu.Index != "" {
			assert.Equal(t, expected[gpu.Index], gpu.ComputeUnits)
		} else {
			for _, inst := range gpu.Instances {
				assert.Equal(t, expected[inst.Index], inst.ComputeUnits)
			}
		}
	}

	// Remove first 10 jobs and add new 20 more jobs
	for i := range 10 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.RemoveAll(dir)
		require.NoError(t, err)
	}

	for i := 19; i < 40; i++ {
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

	// Binds GPUs to first jobs 19 to 25
	for ijob, gres := range mockJobs {
		jobid := ijob + 19

		dir := fmt.Sprintf("%s/%d", procFS, jobid)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)

		envs := []string{fmt.Sprintf("SLURM_JOB_ID=%d", jobid), "SLURM_JOB_GPUS=" + strings.Join(gres.deviceIDs, ",")}

		if gres.numShares > 0 {
			envs = append(envs, fmt.Sprintf("SLURM_SHARDS_ON_NODE=%d", gres.numShares))
		}

		err = os.WriteFile(
			dir+"/environ",
			[]byte(strings.Join(envs, "\000")+"\000"),
			0o600,
		)
		require.NoError(t, err)
	}

	// Now call again get metrics which should populate jobPropsCache
	_, err = c.jobCgroups()
	require.NoError(t, err)

	// Check if jobPropsCache has only 30 jobs and GPU ordinals are empty
	assert.Len(t, c.previousJobIDs, 30)

	// New expected jobs
	expected = map[string][]ComputeUnit{
		"0": {{UUID: "19", NumShares: 2}, {UUID: "23", NumShares: 3}},
		"1": {{UUID: "20"}},
		"2": {{UUID: "20"}},
		"3": {{UUID: "21"}},
		"4": {{UUID: "22"}},
		"5": {{UUID: "24", NumShares: 5}},
		"6": {{UUID: "24", NumShares: 2}},
		"7": {{UUID: "25", NumShares: 3}},
	}

	for _, gpu := range mockGPUDevs {
		if gpu.Index != "" {
			assert.Equal(t, expected[gpu.Index], gpu.ComputeUnits)
		} else {
			for _, inst := range gpu.Instances {
				assert.Equal(t, expected[inst.Index], inst.ComputeUnits)
			}
		}
	}
}

func TestParseSLURMGRESConfig(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.procfs", "testdata/proc",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name         string
		content      string
		hostname     string
		gpus         []Device
		expectedBool map[string]bool
		expected     map[string]uint64
	}{
		{
			name: "Config with global shard",
			content: `# Configure four GPUs (with Sharding)
		AutoDetect=nvml
		Name=gpu Type=gp100 File=/dev/nvidia0 Cores=0,1
		Name=gpu Type=gp100 File=/dev/nvidia1 Cores=0,1
		Name=gpu Type=p6000 File=/dev/nvidia2 Cores=2,3
		Name=gpu Type=p6000 File=/dev/nvidia3 Cores=2,3
		# Set gres/shard Count value to 8 on each of the 4 available GPUs
		Name=shard Count=32`,
			hostname:     "compute-0",
			gpus:         mockGPUDevices(4, nil),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0": 32,
				"1": 32,
				"2": 32,
				"3": 32,
			},
		},
		{
			name: "Config with global MPS",
			content: `# Configure four GPUs (with MPS)
				AutoDetect=nvml
				Name=gpu Type=gp100 File=/dev/nvidia0 Cores=0,1
				Name=gpu Type=gp100 File=/dev/nvidia1 Cores=0,1
				Name=gpu Type=p6000 File=/dev/nvidia2 Cores=2,3
				Name=gpu Type=p6000 File=/dev/nvidia3 Cores=2,3
				# Set gres/mps Count value to 100 on each of the 4 available GPUs
				Name=mps Count=400`,
			hostname:     "compute-0",
			gpus:         mockGPUDevices(4, nil),
			expectedBool: map[string]bool{"mps": true},
			expected: map[string]uint64{
				"0": 400,
				"1": 400,
				"2": 400,
				"3": 400,
			},
		},
		{
			name: "Config with per GPU shard",
			content: `# Configure four different GPU types (with Sharding)
		AutoDetect=nvml
		Name=gpu Type=gtx1080 File=/dev/nvidia0 Cores=0,1
		Name=gpu Type=gtx1070 File=/dev/nvidia1 Cores=0,1
		Name=gpu Type=gtx1060 File=/dev/nvidia2 Cores=2,3
		Name=gpu Type=gtx1050 File=/dev/nvidia3 Cores=2,3
		Name=shard Count=8    File=/dev/nvidia0
		Name=shard Count=8    File=/dev/nvidia1
		Name=shard Count=8    File=/dev/nvidia2
		Name=shard Count=8    File=/dev/nvidia3`,
			hostname:     "compute-0",
			gpus:         mockGPUDevices(4, nil),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0": 8,
				"1": 8,
				"2": 8,
				"3": 8,
			},
		},
		{
			name: "Config with per GPU MPS",
			content: `# Configure four different GPU types (with MPS)
		AutoDetect=nvml
		Name=gpu Type=gtx1080 File=/dev/nvidia0 Cores=0,1
		Name=gpu Type=gtx1070 File=/dev/nvidia1 Cores=0,1
		Name=gpu Type=gtx1060 File=/dev/nvidia2 Cores=2,3
		Name=gpu Type=gtx1050 File=/dev/nvidia3 Cores=2,3
		Name=mps Count=1300   File=/dev/nvidia0
		Name=mps Count=1200   File=/dev/nvidia1
		Name=mps Count=1100   File=/dev/nvidia2
		Name=mps Count=1000   File=/dev/nvidia3`,
			hostname:     "compute-0",
			gpus:         mockGPUDevices(4, nil),
			expectedBool: map[string]bool{"mps": true},
			expected: map[string]uint64{
				"0": 1300,
				"1": 1200,
				"2": 1100,
				"3": 1000,
			},
		},
		{
			name: "Config with Shard and MPS",
			content: `# Configure four different GPU types (with MPS)
		AutoDetect=nvml
		Name=gpu Type=gtx1080 File=/dev/nvidia0 Cores=0,1
		Name=gpu Type=gtx1070 File=/dev/nvidia1 Cores=0,1
		Name=gpu Type=gtx1060 File=/dev/nvidia2 Cores=2,3
		Name=gpu Type=gtx1050 File=/dev/nvidia3 Cores=2,3
		Name=mps Count=1300   File=/dev/nvidia0
		Name=mps Count=1200   File=/dev/nvidia1
		Name=shard Count=8   File=/dev/nvidia2
		Name=shard Count=8   File=/dev/nvidia3`,
			hostname:     "compute-0",
			gpus:         mockGPUDevices(4, nil),
			expectedBool: map[string]bool{"mps": true, "shard": true},
			expected: map[string]uint64{
				"0": 1300,
				"1": 1200,
				"2": 8,
				"3": 8,
			},
		},
		{
			name: "Config with device range and nodename",
			content: `# Slurm GRES configuration
		#

		NodeName=compute-r3i[4-7]n[0-8],compute-r[6-10]i[0-7]n[0-8] Name=shard File=/dev/nvidia[0-1] Count=8
		NodeName=compute-r3i[4-7]n[0-8],compute-r[6-10]i[0-7]n[0-8] Name=shard File=/dev/nvidia[2-3] Count=8

		NodeName=compute-ia[801-831] Name=shard File=/dev/nvidia[0-3] Count=16
		NodeName=compute-ia[801-831] Name=shard File=/dev/nvidia[4-7] Count=32
		`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(8, nil),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0": 16,
				"1": 16,
				"2": 16,
				"3": 16,
				"4": 32,
				"5": 32,
				"6": 32,
				"7": 32,
			},
		},
		{
			name: "Config with MIG instances of 1 GPU and sharding",
			content: `# Configure four different GPU types
		AutoDetect=nvml
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap21
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap66
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap75
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap84
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap21
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap66
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap75
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap84`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(4, []int{0}),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0/2": 8,
				"0/7": 8,
				"0/8": 8,
				"0/9": 8,
			},
		},
		{
			name: "Config with MIG instances of 1 GPU and sharding with global count",
			content: `# Configure four different GPU types
		AutoDetect=nvml
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap21
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap66
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap75
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap84
		Name=shard Count=8`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(4, []int{0}),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0/2": 8,
				"0/7": 8,
				"0/8": 8,
				"0/9": 8,
				"1":   8,
				"2":   8,
				"3":   8,
			},
		},
		{
			name: "Config with MIG instances of multiple GPUs and sharding",
			content: `# Configure four different GPU types
		AutoDetect=nvml
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap21
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap66
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap75
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap84
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap156
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap201
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap210
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap219
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap21
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap66
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap75
		Name=shard Count=8   File=/dev/nvidia-caps/nvidia-cap84
		Name=shard Count=16   File=/dev/nvidia-caps/nvidia-cap156
		Name=shard Count=16   File=/dev/nvidia-caps/nvidia-cap201
		Name=shard Count=16   File=/dev/nvidia-caps/nvidia-cap210
		Name=shard Count=16   File=/dev/nvidia-caps/nvidia-cap219`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(4, []int{0, 1, 2, 3}),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0/2": 8,
				"0/7": 8,
				"0/8": 8,
				"0/9": 8,
				"1/2": 16,
				"1/7": 16,
				"1/8": 16,
				"1/9": 16,
			},
		},
		{
			name: "Config with MIG instances of multiple GPUs and sharding and Multiplefiles",
			content: `# Configure four different GPU types
		AutoDetect=nvml
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap21
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap66
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap75
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap84
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap156
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap201
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap210
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap219
		Name=shard Count=8   MultipleFiles=/dev/nvidia-caps/nvidia-cap21,/dev/nvidia-caps/nvidia-cap156
		Name=shard Count=8   MultipleFiles=/dev/nvidia-caps/nvidia-cap66,/dev/nvidia-caps/nvidia-cap201
		Name=shard Count=8   MultipleFiles=/dev/nvidia-caps/nvidia-cap75,/dev/nvidia-caps/nvidia-cap210
		Name=shard Count=16   MultipleFiles=/dev/nvidia-caps/nvidia-cap84,/dev/nvidia-caps/nvidia-cap219`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(4, []int{0, 1, 2, 3}),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0/2": 8,
				"0/7": 8,
				"0/8": 8,
				"0/9": 16,
				"1/2": 8,
				"1/7": 8,
				"1/8": 8,
				"1/9": 16,
			},
		},
		{
			name: "Config with MIG instances of multiple GPUs and sharding and Multiplefiles and range",
			content: `# Configure four different GPU types
		AutoDetect=nvml
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap21
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap66
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap75
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap84
		Name=gpu Type=4g.24gb File=/dev/nvidia-caps/nvidia-cap156
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap201
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap210
		Name=gpu Type=1g.6gb File=/dev/nvidia-caps/nvidia-cap219
		Name=shard Count=8   MultipleFiles=/dev/nvidia-caps/nvidia-cap[21,156]
		Name=shard Count=16   MultipleFiles=/dev/nvidia-caps/nvidia-cap[66,201]
		Name=shard Count=8   MultipleFiles=/dev/nvidia-caps/nvidia-cap[75,210]
		Name=shard Count=16   MultipleFiles=/dev/nvidia-caps/nvidia-cap[84,219]`,
			hostname:     "compute-ia827",
			gpus:         mockGPUDevices(4, []int{0, 1, 2, 3}),
			expectedBool: map[string]bool{"shard": true},
			expected: map[string]uint64{
				"0/2": 8,
				"0/7": 16,
				"0/8": 8,
				"0/9": 16,
				"1/2": 8,
				"1/7": 16,
				"1/8": 8,
				"1/9": 16,
			},
		},
	}

	for _, test := range tests {
		have := make(map[string]uint64)

		for _, st := range []string{"shard", "mps"} {
			updatedGPUs, enabled := updateGPUAvailableShares(test.content, st, test.hostname, test.gpus)
			assert.Equal(t, test.expectedBool[st], enabled, test.name)

			for _, g := range updatedGPUs {
				if len(g.Instances) > 0 {
					for _, inst := range g.Instances {
						if inst.AvailableShares > 0 {
							have[fmt.Sprintf("%s/%d", g.Minor, inst.GPUInstID)] = inst.AvailableShares
						}
					}
				}

				if g.AvailableShares > 0 {
					have[g.Minor] = g.AvailableShares
				}
			}
		}

		assert.Equal(t, test.expected, have, test.name)
	}
}
