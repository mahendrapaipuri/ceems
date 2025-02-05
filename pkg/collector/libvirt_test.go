//go:build !nolibvirt
// +build !nolibvirt

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

	collector, err := NewLibvirtCollector(slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestLibvirtInstanceProps(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.libvirt.xml-dir", "testdata/qemu",
			"--collector.cgroups.force-version", "v2",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		mode:        cgroups.Unified,
		mountPoints: []string{"testdata/sys/fs/cgroup/machine.slice"},
		idRegex:     libvirtCgroupPathRegex,
		isChild: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	noOpLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gpuDevs, err := GetGPUDevices("nvidia", noOpLogger)
	require.NoError(t, err)

	c := libvirtCollector{
		gpuDevs:                     gpuDevs,
		logger:                      noOpLogger,
		cgroupManager:               cgManager,
		vGPUActivated:               true,
		instancePropsCache:          make(map[string]instanceProps),
		instancePropsCacheTTL:       500 * time.Millisecond,
		instancePropslastUpdateTime: time.Now(),
		securityContexts:            make(map[string]*security.SecurityContext),
	}

	// Last update time
	lastUpdateTime := c.instancePropslastUpdateTime

	// Add dummy security context
	c.securityContexts[libvirtReadXMLCtx], err = security.NewSecurityContext(
		libvirtReadXMLCtx,
		nil,
		readLibvirtXMLFile,
		c.logger,
	)
	require.NoError(t, err)

	expectedProps := []instanceProps{
		{uuid: "57f2d45e-8ddf-4338-91df-62d0044ff1b5", gpuOrdinals: []string{"1", "8"}},
		{uuid: "b674a0a2-c300-4dc6-8c9c-65df16da6d69", gpuOrdinals: []string{"0", "3"}},
		{uuid: "2896bdd5-dbc2-4339-9d8e-ddd838bf35d3", gpuOrdinals: []string{"11", "9"}},
		{uuid: "4de89c5b-50d7-4d30-a630-14e135380fe8", gpuOrdinals: []string(nil)},
	}

	metrics, err := c.instanceMetrics()
	require.NoError(t, err)

	assert.EqualValues(t, expectedProps, metrics.instanceProps)

	// Sleep for 0.5 seconds to ensure we invalidate cache
	time.Sleep(500 * time.Millisecond)

	_, err = c.instanceMetrics()
	require.NoError(t, err)

	// Now check if lastUpdateTime is less than 0.5 se
	assert.Greater(t, c.instancePropslastUpdateTime.Sub(lastUpdateTime), 500*time.Millisecond)
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
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		mode:        cgroups.Unified,
		root:        cgroupsPath,
		mountPoints: []string{cgroupsPath + "/cpuacct/machine.slice"},
		idRegex:     libvirtCgroupPathRegex,
		isChild: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	noOpLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gpuDevs, err := GetGPUDevices("nvidia", noOpLogger)
	require.NoError(t, err)

	c := libvirtCollector{
		cgroupManager:               cgManager,
		logger:                      noOpLogger,
		gpuDevs:                     gpuDevs,
		vGPUActivated:               true,
		instancePropsCache:          make(map[string]instanceProps),
		instancePropsCacheTTL:       500 * time.Millisecond,
		instancePropslastUpdateTime: time.Now(),
		securityContexts:            make(map[string]*security.SecurityContext),
	}

	// Add dummy security context
	c.securityContexts[libvirtReadXMLCtx], err = security.NewSecurityContext(
		libvirtReadXMLCtx,
		nil,
		readLibvirtXMLFile,
		c.logger,
	)
	require.NoError(t, err)

	// Add cgroups
	for i := range 20 {
		dir := fmt.Sprintf("%s/cpuacct/machine.slice/machine-qemu\x2d1\x2dinstance\x2d0000000%d.scope", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Binds GPUs to first n instances
	var iInstance int

	var fullGPUInstances []string

	for _, dev := range gpuDevs {
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
		if !dev.vgpuEnabled && !dev.migEnabled {
			xmlContent := fmt.Sprintf(xmlContentPH, iInstance, strconv.FormatUint(dev.busID.bus, 16))
			err = os.WriteFile(
				fmt.Sprintf("%s/instance-0000000%d.xml", xmlFilePath, iInstance),
				[]byte(xmlContent),
				0o600,
			)
			require.NoError(t, err)

			fullGPUInstances = append(fullGPUInstances, dev.globalIndex)
			iInstance++
		}
	}

	// Now call get metrics which should populate instancePropsCache
	_, err = c.instanceMetrics()
	require.NoError(t, err)

	// Check if instancePropsCache has 20 instances and GPU ordinals are correct
	assert.Len(t, c.instancePropsCache, 20)

	for i, gpuIDString := range fullGPUInstances {
		instanceIDString := strconv.FormatInt(int64(i), 10)
		assert.Equal(t, []string{gpuIDString}, c.instancePropsCache["instance-0000000"+instanceIDString].gpuOrdinals)
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
	_, err = c.instanceMetrics()
	require.NoError(t, err)

	// Check if instancePropsCache has only 15 instances and GPU ordinals are empty
	assert.Len(t, c.instancePropsCache, 15)

	for _, p := range c.instancePropsCache {
		assert.Empty(t, p.gpuOrdinals)
	}
}
