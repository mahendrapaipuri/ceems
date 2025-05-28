//go:build !nolibvirt
// +build !nolibvirt

package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/cgroups/v3"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLibvirtCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--path.sysfs", "testdata/sys",
			"--collector.libvirt.swap-memory-metrics",
			"--collector.libvirt.psi-metrics",
			"--collector.libvirt.xml-dir", "testdata/qemu",
			"--collector.perf.hardware-events",
			"--collector.rdma.stats",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewLibvirtCollector(noOpLogger)
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

	err = collector.Stop(t.Context())
	require.NoError(t, err)
}

func TestLibvirtInstanceProps(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.libvirt.xml-dir", "testdata/qemu",
			"--collector.cgroups.force-version", "v2",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		logger:      noOpLogger,
		mode:        cgroups.Unified,
		mountPoints: []string{"testdata/sys/fs/cgroup/machine.slice"},
		idRegex:     libvirtCgroupPathRegex,
		isChild: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	noOpLogger := noOpLogger

	// Instantiate a new instance of gpuSMI struct
	gpu, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Attempt to get GPU devices
	err = gpu.Discover()
	require.NoError(t, err)

	c := libvirtCollector{
		gpuSMI:                        gpu,
		logger:                        noOpLogger,
		cgroupManager:                 cgManager,
		vGPUActivated:                 true,
		instanceDevicesCacheTTL:       500 * time.Millisecond,
		instanceDeviceslastUpdateTime: time.Now(),
		securityContexts:              make(map[string]*security.SecurityContext),
	}

	// Last update time
	lastUpdateTime := c.instanceDeviceslastUpdateTime

	// Add dummy security context
	cfg := &security.SCConfig{
		Name:   libvirtReadXMLCtx,
		Caps:   nil,
		Func:   readLibvirtXMLFile,
		Logger: c.logger,
	}
	c.securityContexts[libvirtReadXMLCtx], err = security.NewSecurityContext(cfg)
	require.NoError(t, err)

	expectedDeviceUnits := map[string][]ComputeUnit{
		"0": {
			{UUID: "b674a0a2-c300-4dc6-8c9c-65df16da6d69", NumShares: 1},
			{UUID: "2896bdd5-dbc2-4339-9d8e-ddd838bf35d3", NumShares: 1},
		},
		"1":  {{UUID: "57f2d45e-8ddf-4338-91df-62d0044ff1b5", NumShares: 1}},
		"3":  {{UUID: "b674a0a2-c300-4dc6-8c9c-65df16da6d69", NumShares: 1}},
		"8":  {{UUID: "57f2d45e-8ddf-4338-91df-62d0044ff1b5", NumShares: 1}},
		"9":  {{UUID: "2896bdd5-dbc2-4339-9d8e-ddd838bf35d3", NumShares: 1}},
		"11": {{UUID: "2896bdd5-dbc2-4339-9d8e-ddd838bf35d3", NumShares: 1}},
	}

	cgroups, err := c.instanceCgroups()
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"instance-00000001", "instance-00000002", "instance-00000003", "instance-00000004"}, c.previousInstanceIDs)
	assert.Len(t, cgroups, 4)

	for _, gpu := range c.gpuSMI.Devices {
		if gpu.Index != "" {
			assert.ElementsMatch(t, expectedDeviceUnits[gpu.Index], gpu.ComputeUnits, "GPU %s", gpu.Index)
		} else {
			for _, inst := range gpu.Instances {
				assert.ElementsMatch(t, expectedDeviceUnits[inst.Index], inst.ComputeUnits, "MIG %s", inst.Index)
			}
		}
	}

	// Sleep for 0.5 seconds to ensure we invalidate cache
	time.Sleep(500 * time.Millisecond)

	_, err = c.instanceCgroups()
	require.NoError(t, err)

	// Now check if lastUpdateTime is less than 0.5 se
	assert.Greater(t, c.instanceDeviceslastUpdateTime.Sub(lastUpdateTime), 500*time.Millisecond)
}

func TestInstancePropsCaching(t *testing.T) {
	path := t.TempDir()

	cgroupsPath := path + "/cgroups"
	err := os.Mkdir(cgroupsPath, 0o750)
	require.NoError(t, err)

	xmlFilePath := path + "/qemu"
	err = os.Mkdir(xmlFilePath, 0o750)
	require.NoError(t, err)

	_, err = CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", cgroupsPath,
			"--collector.libvirt.xml-dir", xmlFilePath,
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		logger:      noOpLogger,
		mode:        cgroups.Unified,
		root:        cgroupsPath,
		mountPoints: []string{cgroupsPath + "/cpuacct/machine.slice"},
		idRegex:     libvirtCgroupPathRegex,
		isChild: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	noOpLogger := noOpLogger

	// Instantiate a new instance of gpuSMI struct
	gpu, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Attempt to get GPU devices
	err = gpu.Discover()
	require.NoError(t, err)

	c := libvirtCollector{
		cgroupManager:                 cgManager,
		logger:                        noOpLogger,
		gpuSMI:                        gpu,
		vGPUActivated:                 true,
		instanceDevicesCacheTTL:       500 * time.Millisecond,
		instanceDeviceslastUpdateTime: time.Now(),
		securityContexts:              make(map[string]*security.SecurityContext),
	}

	// Add dummy security context
	cfg := &security.SCConfig{
		Name:   libvirtReadXMLCtx,
		Caps:   nil,
		Func:   readLibvirtXMLFile,
		Logger: c.logger,
	}
	c.securityContexts[libvirtReadXMLCtx], err = security.NewSecurityContext(cfg)
	require.NoError(t, err)

	// Add cgroups
	for i := range 20 {
		dir := fmt.Sprintf("%s/cpuacct/machine.slice/machine-qemu\x2d1\x2dinstance\x2d0000000%d.scope", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Binds GPUs to first n instances
	var iInstance int

	var fullGPUInstances []int

	for idev, dev := range c.gpuSMI.Devices {
		xmlContentPH := `<domain type='kvm'>
<name>instance-%[1]d</name>
<uuid>%[1]d</uuid>
<devices>
<hostdev mode='subsystem' type='pci' managed='yes'>
	<source>
		<address type='pci' domain='0x0000' bus='0x%[2]s' slot='0x0' function='0x0'/>
	</source>
</hostdev>
</devices>
</domain>`
		if !dev.VGPUEnabled && !dev.InstancesEnabled {
			xmlContent := fmt.Sprintf(xmlContentPH, idev, strconv.FormatUint(dev.BusID.bus, 16))
			err = os.WriteFile(
				fmt.Sprintf("%s/instance-0000000%d.xml", xmlFilePath, iInstance),
				[]byte(xmlContent),
				0o600,
			)
			require.NoError(t, err)

			fullGPUInstances = append(fullGPUInstances, idev)
			iInstance++
		}
	}

	// Now call get metrics which should populate instancePropsCache
	_, err = c.instanceCgroups()
	require.NoError(t, err)

	// Check if instancePropsCache has 20 instances and GPU ordinals are correct
	assert.Len(t, c.previousInstanceIDs, 20)

	for _, gpuID := range fullGPUInstances {
		assert.Equal(t, []ComputeUnit{{UUID: strconv.FormatInt(int64(gpuID), 10), NumShares: 1}}, c.gpuSMI.Devices[gpuID].ComputeUnits)
	}

	// Remove first 10 instances and add new 10 more instances
	for i := range 10 {
		dir := fmt.Sprintf("%s/cpuacct/machine.slice/machine-qemu\x2d1\x2dinstance\x2d0000000%d.scope", cgroupsPath, i)

		err = os.RemoveAll(dir)
		require.NoError(t, err)
	}

	for i := 19; i < 25; i++ {
		dir := fmt.Sprintf("%s/cpuacct/machine.slice/machine-qemu\x2d1\x2dinstance\x2d0000000%d.scope", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Now call again get metrics which should populate instancePropsCache
	_, err = c.instanceCgroups()
	require.NoError(t, err)

	// Check if instancePropsCache has only 15 instances and GPU ordinals are empty
	assert.Len(t, c.previousInstanceIDs, 15)

	for _, p := range c.gpuSMI.Devices {
		assert.Empty(t, p.ComputeUnits)
	}
}
