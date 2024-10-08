//go:build !nolibvirt
// +build !nolibvirt

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
			"--collector.perf.hardware-events",
			"--collector.rdma.stats",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewLibvirtCollector(log.NewNopLogger())
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
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/machine.slice",
		idRegex:    libvirtCgroupPathRegex,
		pathFilter: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	c := libvirtCollector{
		gpuDevs:            mockGPUDevices(),
		logger:             log.NewNopLogger(),
		cgroupManager:      cgManager,
		instancePropsCache: make(map[string]instanceProps),
		securityContexts:   make(map[string]*security.SecurityContext),
	}

	// Add dummy security context
	c.securityContexts[libvirtReadXMLCtx], err = security.NewSecurityContext(
		libvirtReadXMLCtx,
		nil,
		readLibvirtXMLFile,
		c.logger,
	)
	require.NoError(t, err)

	expectedProps := instanceProps{
		gpuOrdinals: []string{"0", "1"},
		uuid:        "57f2d45e-8ddf-4338-91df-62d0044ff1b5",
	}

	metrics, err := c.discoverCgroups()
	require.NoError(t, err)

	var gotProps instanceProps

	for _, props := range metrics.instanceProps {
		if props.uuid == expectedProps.uuid {
			gotProps = props
		}
	}

	assert.Equal(t, expectedProps, gotProps)
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
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		root:       cgroupsPath,
		mountPoint: cgroupsPath + "/cpuacct/machine.slice",
		idRegex:    libvirtCgroupPathRegex,
		pathFilter: func(p string) bool {
			return strings.Contains(p, "/libvirt")
		},
	}

	mockGPUDevs := mockGPUDevices()
	c := libvirtCollector{
		cgroupManager:      cgManager,
		logger:             log.NewNopLogger(),
		gpuDevs:            mockGPUDevs,
		instancePropsCache: make(map[string]instanceProps),
		securityContexts:   make(map[string]*security.SecurityContext),
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

	// Binds GPUs to first n jobs
	for igpu := range mockGPUDevs {
		xmlContentPH := `<domain type='kvm'>
<name>instance-%[1]d</name>
<uuid>%[1]d</uuid>
<devices>
<hostdev mode='subsystem' type='pci' managed='yes'>
	<source>
	<address domain='domain' bus='bus' slot='slot' function='function'/>
	</source>
	<address type='pci' domain='0x0000' bus='0x%[2]s' slot='0x0' function='0x0'/>
</hostdev>
</devices>
</domain>`
		xmlContent := fmt.Sprintf(xmlContentPH, igpu, strconv.FormatUint(mockGPUDevs[igpu].busID.bus, 16))
		err = os.WriteFile(
			fmt.Sprintf("%s/instance-0000000%d.xml", xmlFilePath, igpu),
			[]byte(xmlContent),
			0o600,
		)
		require.NoError(t, err)
	}

	// Now call get metrics which should populate instancePropsCache
	_, err = c.discoverCgroups()
	require.NoError(t, err)

	// Check if instancePropsCache has 20 instances and GPU ordinals are correct
	assert.Len(t, c.instancePropsCache, 20)

	for igpu := range mockGPUDevs {
		gpuIDString := strconv.FormatInt(int64(igpu), 10)
		assert.Equal(t, []string{gpuIDString}, c.instancePropsCache["instance-0000000"+gpuIDString].gpuOrdinals)
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
	_, err = c.discoverCgroups()
	require.NoError(t, err)

	// Check if instancePropsCache has only 15 instances and GPU ordinals are empty
	assert.Len(t, c.instancePropsCache, 15)

	for _, p := range c.instancePropsCache {
		assert.Empty(t, p.gpuOrdinals)
	}
}
