package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	ceems_k8s "github.com/mahendrapaipuri/ceems/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func getExpectedNvidiaDevs() []Device {
	return []Device{
		{
			Minor:            "0",
			Index:            "0",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
			BusID:            BusID{domain: 0x0, bus: 0x10, device: 0x0, function: 0x0, pathName: "00000000:10:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      true,
		},
		{
			Minor:            "1",
			Index:            "1",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
			BusID:            BusID{domain: 0x0, bus: 0x15, device: 0x0, function: 0x0, pathName: "00000000:15:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
		{
			Minor:    "2",
			vendorID: 1,
			Name:     "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:     "GPU-956348bc-d43d-23ed-53d4-857749fa2b67",
			BusID:    BusID{domain: 0x0, bus: 0x21, device: 0x0, function: 0x0, pathName: "00000000:21:00.0"},
			Instances: []GPUInstance{
				{InstanceIndex: 0x0, Index: "2", ComputeInstID: 0x0, GPUInstID: 0x1, UUID: "MIG-ce2e805f-ce8e-5cf7-8132-176167d87d24", SMFraction: 0.3888888888888889, NumSMs: 42},
				{InstanceIndex: 0x1, Index: "3", ComputeInstID: 0x0, GPUInstID: 0x5, UUID: "MIG-2cc993d7-588c-5c28-b454-b3851897e3d7", SMFraction: 0.12962962962962962, NumSMs: 14},
				{InstanceIndex: 0x2, Index: "4", ComputeInstID: 0x0, GPUInstID: 0xd, UUID: "MIG-4bd078f2-f9bb-5bfb-8695-774674f75e96", SMFraction: 0.12962962962962962, NumSMs: 14},
			},
			NumSMs:           108,
			InstancesEnabled: true,
			VGPUEnabled:      true,
		},
		{
			Minor:    "3",
			vendorID: 1,
			Name:     "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:     "GPU-feba7e40-d724-01ff-b00f-3a439a28a6c7",
			BusID:    BusID{domain: 0x0, bus: 0x81, device: 0x0, function: 0x0, pathName: "00000000:81:00.0"},
			Instances: []GPUInstance{
				{InstanceIndex: 0x0, Index: "5", ComputeInstID: 0x0, GPUInstID: 0x1, UUID: "MIG-4894e267-46d0-557e-b826-500e978d88d1", SMFraction: 0.5185185185185185, NumSMs: 56},
				{InstanceIndex: 0x1, Index: "6", ComputeInstID: 0x0, GPUInstID: 0x5, UUID: "MIG-ed3d4e0a-516b-5cdf-a202-6239aa536031", SMFraction: 0.25925925925925924, NumSMs: 28},
				{InstanceIndex: 0x2, Index: "7", ComputeInstID: 0x0, GPUInstID: 0x6, UUID: "MIG-017c61e4-656c-5059-b7b1-276506580e3c", SMFraction: 0.12962962962962962, NumSMs: 14},
			},
			NumSMs:           108,
			InstancesEnabled: true,
			VGPUEnabled:      true,
		},
		{
			Minor:            "4",
			Index:            "8",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3",
			BusID:            BusID{domain: 0x0, bus: 0x83, device: 0x0, function: 0x0, pathName: "00000000:83:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
		{
			Minor:            "5",
			Index:            "9",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3",
			BusID:            BusID{domain: 0x0, bus: 0x85, device: 0x0, function: 0x0, pathName: "00000000:85:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      true,
		},
		{
			Minor:            "6",
			Index:            "10",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-1d4d0f3e-b51a-4040-96e3-bf380f7c5728",
			BusID:            BusID{domain: 0x0, bus: 0x87, device: 0x0, function: 0x0, pathName: "00000000:87:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
		{
			Minor:            "7",
			Index:            "11",
			vendorID:         nvidia,
			Name:             "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			UUID:             "GPU-6cc98505-fdde-461e-a93c-6935fba45a27",
			BusID:            BusID{domain: 0x0, bus: 0x89, device: 0x0, function: 0x0, pathName: "00000000:89:00.0"},
			NumSMs:           108,
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
	}
}

func getExpectedAmdDevs() []Device {
	return []Device{
		{
			Minor:            "0",
			Index:            "0",
			vendorID:         amd,
			Name:             "Advanced Micro Devices, Inc. [AMD/ATI]",
			UUID:             "20170000800c",
			NumSMs:           304,
			BusID:            BusID{domain: 0x0, bus: 0xc5, device: 0x0, function: 0x0, pathName: "0000:c5:00.0"},
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
		{
			Minor:            "1",
			vendorID:         amd,
			Name:             "Advanced Micro Devices, Inc. [AMD/ATI]",
			UUID:             "20170003580c",
			BusID:            BusID{domain: 0x0, bus: 0xc8, device: 0x0, function: 0x0, pathName: "0000:c8:00.0"},
			InstancesEnabled: true,
			Instances: []GPUInstance{
				{InstanceIndex: 0x0, Index: "1", GPUInstID: 0x0, NumSMs: 38, SMFraction: 0.125, UUID: "0000:c8:00.0"},
				{InstanceIndex: 0x1, Index: "2", GPUInstID: 0x1, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.7"},
				{InstanceIndex: 0x2, Index: "3", GPUInstID: 0x2, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.8"},
				{InstanceIndex: 0x3, Index: "4", GPUInstID: 0x3, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.9"},
				{InstanceIndex: 0x4, Index: "5", GPUInstID: 0x4, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.10"},
				{InstanceIndex: 0x5, Index: "6", GPUInstID: 0x5, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.11"},
				{InstanceIndex: 0x6, Index: "7", GPUInstID: 0x6, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.12"},
				{InstanceIndex: 0x7, Index: "8", GPUInstID: 0x7, NumSMs: 38, SMFraction: 0.125, UUID: "amdgpu_xcp.13"},
			},
			VGPUEnabled: false,
		},
		{
			Minor:            "2",
			vendorID:         amd,
			Name:             "Advanced Micro Devices, Inc. [AMD/ATI]",
			UUID:             "20180003050c",
			BusID:            BusID{domain: 0x0, bus: 0x8a, device: 0x0, function: 0x0, pathName: "0000:8a:00.0"},
			InstancesEnabled: true,
			Instances: []GPUInstance{
				{InstanceIndex: 0x0, Index: "9", GPUInstID: 0x0, NumSMs: 152, SMFraction: 0.5, UUID: "0000:8a:00.0"},
				{InstanceIndex: 0x1, Index: "10", GPUInstID: 0x1, NumSMs: 152, SMFraction: 0.5, UUID: "amdgpu_xcp.14"},
			},
			VGPUEnabled: false,
		},
		{
			Minor:            "3",
			Index:            "11",
			vendorID:         amd,
			Name:             "Advanced Micro Devices, Inc. [AMD/ATI]",
			UUID:             "20170005280c",
			NumSMs:           304,
			BusID:            BusID{domain: 0x0, bus: 0x8d, device: 0x0, function: 0x0, pathName: "0000:8d:00.0"},
			InstancesEnabled: false,
			VGPUEnabled:      false,
		},
	}
}

func TestNewGPUSMI(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.rocm-smi-path", "testdata/rocm-smi",
			"--path.sysfs", "testdata/sys",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Expected vendors
	assert.Len(t, g.vendors, 2)

	// Check smi commands are correctly populated
	for _, v := range g.vendors {
		var smiCmd string
		if v.id == nvidia {
			smiCmd = "nvidia-smi"
		} else {
			smiCmd = "rocm-smi"
		}

		assert.Contains(t, v.smiCmd, smiCmd)
	}
}

func TestNewGPUSMIWithK8s(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.rocm-smi-path", "testdata/rocm-smi",
			"--path.sysfs", "testdata/sys",
		},
	)
	require.NoError(t, err)

	gpuPods := []runtime.Object{
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod11",
				UID:       "uid11",
				Namespace: "nvidia-gpu-operator",
				Labels: map[string]string{
					"app": "gpu-feature-discovery",
				},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod12",
				UID:       "uid12",
				Namespace: "amd-gpu-operator",
				Labels: map[string]string{
					"app.kubernetes.io/name": "metrics-exporter",
				},
			},
		},
	}

	// Make fake client
	fakeClientset := fake.NewSimpleClientset(gpuPods...)

	// Make k8s client
	client := &ceems_k8s.Client{
		Logger:    noOpLogger,
		Clientset: fakeClientset,
	}

	g, err := NewGPUSMI(client, noOpLogger)
	require.NoError(t, err)

	// Expected vendors
	assert.Len(t, g.vendors, 2)

	// Check smi commands are correctly populated
	for _, v := range g.vendors {
		var smiCmd, ns, pod string
		if v.id == nvidia {
			smiCmd = "nvidia-smi"
			ns = "nvidia-gpu-operator"
			pod = "pod11"
		} else {
			smiCmd = "rocm-smi"
			ns = "amd-gpu-operator"
			pod = "pod12"
		}

		assert.Contains(t, v.smiCmd, smiCmd)
		assert.Equal(t, ns, v.k8sNS)
		assert.Equal(t, pod, v.k8sPod)
	}
}

func TestDiscoverGPUs(t *testing.T) {
	tempDir := t.TempDir()
	nvidiaSMIPath := filepath.Join(tempDir, "nvidia-smi")
	content := `#!/bin/bash
exit 1`
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", nvidiaSMIPath,
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	var gpuErr error

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// First attempt should be error as there is nvidia-smi command exits with 1
	go func() {
		defer wg.Done()

		gpuErr = g.Discover()
	}()

	time.Sleep(time.Second)

	// Read testdata/nvidia-smi content and write to test nvidia-smi file
	nvidiaSMIContent, err := os.ReadFile("testdata/nvidia-smi")
	require.NoError(t, err)

	os.WriteFile(nvidiaSMIPath, nvidiaSMIContent, 0o700) //nolint:gosec

	wg.Wait()

	require.NoError(t, gpuErr)
	assert.Equal(t, getExpectedNvidiaDevs(), g.Devices)
}

func TestParseNvidiaSmiOutput(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Select nvidia vendor
	for _, vendor := range g.vendors {
		if vendor.id == nvidia {
			gpuDevices, err := g.nvidiaGPUDevices(vendor)
			require.NoError(t, err)
			assert.Equal(t, getExpectedNvidiaDevs(), gpuDevices)
		}
	}
}

func TestNvidiaMIGAtLowerAddr(t *testing.T) {
	nvidiaSmiLog := `<?xml version="1.0" ?>
<!DOCTYPE nvidia_smi_log SYSTEM "nvsmi_device_v12.dtd">
<nvidia_smi_log>
	<timestamp>Fri Oct 11 18:24:09 2024</timestamp>
	<driver_version>535.129.03</driver_version>
	<cuda_version>12.2</cuda_version>
	<attached_gpus>2</attached_gpus>
	<gpu id=\"00000000:10:00.0\">
		<mig_mode>
				<current_mig>Enabled</current_mig>
				<pending_mig>Enabled</pending_mig>
		</mig_mode>
		<mig_devices>
				<mig_device>
					<index>0</index>
					<gpu_instance_id>1</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
				<mig_device>
					<index>1</index>
					<gpu_instance_id>5</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
				<mig_device>
					<index>2</index>
					<gpu_instance_id>5</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
		</mig_devices>
	</gpu>
	<gpu id=\"00000000:15:00.0\">
		<mig_mode>
				<current_mig>N/A</current_mig>
				<pending_mig>N/A</pending_mig>
		</mig_mode>
		<mig_devices>
				None
		</mig_devices>
	</gpu>
</nvidia_smi_log>`
	tempDir := t.TempDir()
	nvidiaSMIPath := filepath.Join(tempDir, "nvidia-smi")
	content := fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, nvidiaSmiLog)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", nvidiaSMIPath,
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Select nvidia vendor
	for _, vendor := range g.vendors {
		if vendor.id == nvidia {
			gpuDevices, err := g.nvidiaGPUDevices(vendor)
			require.NoError(t, err)

			// Check if Index for GPU 0 is empty and GPU 1 is 3
			assert.Empty(t, gpuDevices[0].Index)
			assert.Equal(t, "3", gpuDevices[1].Index)
		}
	}
}

func TestNvidiaMIGAtHigherAddr(t *testing.T) {
	nvidiaSmiLog := `<?xml version="1.0" ?>
<!DOCTYPE nvidia_smi_log SYSTEM "nvsmi_device_v12.dtd">
<nvidia_smi_log>
	<timestamp>Fri Oct 11 18:24:09 2024</timestamp>
	<driver_version>535.129.03</driver_version>
	<cuda_version>12.2</cuda_version>
	<attached_gpus>2</attached_gpus>
	<gpu id=\"00000000:15:00.0\">
		<mig_mode>
				<current_mig>N/A</current_mig>
				<pending_mig>N/A</pending_mig>
		</mig_mode>
		<mig_devices>
				None
		</mig_devices>
	</gpu>
	<gpu id=\"00000000:10:00.0\">
		<mig_mode>
				<current_mig>Enabled</current_mig>
				<pending_mig>Enabled</pending_mig>
		</mig_mode>
		<mig_devices>
				<mig_device>
					<index>0</index>
					<gpu_instance_id>1</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
				<mig_device>
					<index>1</index>
					<gpu_instance_id>5</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
				<mig_device>
					<index>2</index>
					<gpu_instance_id>5</gpu_instance_id>
					<compute_instance_id>0</compute_instance_id>
				</mig_device>
		</mig_devices>
	</gpu>
</nvidia_smi_log>`
	tempDir := t.TempDir()
	nvidiaSMIPath := filepath.Join(tempDir, "nvidia-smi")
	content := fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, nvidiaSmiLog)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", nvidiaSMIPath,
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Select nvidia vendor
	for _, vendor := range g.vendors {
		if vendor.id == nvidia {
			gpuDevices, err := g.nvidiaGPUDevices(vendor)
			require.NoError(t, err)

			// Check if Index for GPU 1 is empty and GPU 0 is 0
			assert.Empty(t, gpuDevices[1].Index)
			assert.Equal(t, "0", gpuDevices[0].Index)
		}
	}
}

func TestParseRocmSmiOutput(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.rocm-smi-path", "testdata/rocm-smi",
			"--collector.gpu.type", "amd",
			"--path.sysfs", "testdata/sys",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Select amd vendor
	for _, vendor := range g.vendors {
		if vendor.id == amd {
			gpuDevices, err := g.amdGPUDevices(vendor)
			require.NoError(t, err)

			assert.Equal(t, getExpectedAmdDevs(), gpuDevices)
		}
	}
}

func TestParseAmdSmiOutput(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.amd-smi-path", "testdata/amd-smi",
			"--collector.gpu.type", "amd",
			"--path.sysfs", "testdata/sys",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Select amd vendor
	for _, vendor := range g.vendors {
		if vendor.id == amd {
			gpuDevices, err := g.amdGPUDevices(vendor)
			require.NoError(t, err)

			assert.Equal(t, getExpectedAmdDevs(), gpuDevices)
		}
	}
}

func TestDetectDevices(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.sysfs", "testdata/sys",
		},
	)
	require.NoError(t, err)

	gpuVendors, err := detectVendors()
	require.NoError(t, err)

	var vendorNames []string
	for _, vendor := range gpuVendors {
		vendorNames = append(vendorNames, vendor.name)
	}

	assert.ElementsMatch(t, []string{"amd", "nvidia"}, vendorNames)
}

func TestReindexGPUs(t *testing.T) {
	testCases := []struct {
		Name         string
		devs         []Device
		expectedDevs []Device
		orderMap     string
	}{
		{
			devs: []Device{
				{
					Index: "",
					Minor: "0",
					Instances: []GPUInstance{
						{
							Index:     "0",
							GPUInstID: 3,
						},
						{
							Index:     "1",
							GPUInstID: 5,
						},
						{
							Index:     "2",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
				{
					Index: "1",
					Minor: "1",
				},
			},
			expectedDevs: []Device{
				{
					Index: "",
					Minor: "0",
					Instances: []GPUInstance{
						{
							Index:     "1",
							GPUInstID: 3,
						},
						{
							Index:     "2",
							GPUInstID: 5,
						},
						{
							Index:     "3",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
				{
					Index: "0",
					Minor: "1",
				},
			},
			orderMap: "0:1,1:0.3,2:0.5,3:0.9",
		},
		{
			devs: []Device{
				{
					Index: "0",
					Minor: "0",
				},
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "1",
							GPUInstID: 3,
						},
						{
							Index:     "2",
							GPUInstID: 5,
						},
						{
							Index:     "3",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
			},
			expectedDevs: []Device{
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "0",
							GPUInstID: 3,
						},
						{
							Index:     "1",
							GPUInstID: 5,
						},
						{
							Index:     "2",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
				{
					Index: "3",
					Minor: "0",
				},
			},
			orderMap: "0:1.3,1:1.5,2:1.9,3:0",
		},
		{
			devs: []Device{
				{
					Index: "0",
					Minor: "0",
				},
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "1",
							GPUInstID: 3,
						},
						{
							Index:     "2",
							GPUInstID: 5,
						},
						{
							Index:     "3",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
				{
					Index: "4",
					Minor: "2",
				},
			},
			expectedDevs: []Device{
				{
					Index: "0",
					Minor: "0",
				},
				{
					Index: "1",
					Minor: "2",
				},
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "2",
							GPUInstID: 3,
						},
						{
							Index:     "3",
							GPUInstID: 5,
						},
						{
							Index:     "4",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
			},
			orderMap: "0:0,1:2,2:1.3,3:1.5,4:1.9",
		},
		{
			devs: []Device{
				{
					Index: "0",
					Minor: "0",
				},
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "1",
							GPUInstID: 3,
						},
						{
							Index:     "2",
							GPUInstID: 5,
						},
						{
							Index:     "3",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
				{
					Index: "4",
					Minor: "2",
				},
			},
			expectedDevs: []Device{
				{
					Index: "0",
					Minor: "0",
				},
				{
					Index: "1",
					Minor: "2",
				},
				{
					Index: "",
					Minor: "1",
					Instances: []GPUInstance{
						{
							Index:     "2",
							GPUInstID: 3,
						},
						{
							Index:     "3",
							GPUInstID: 5,
						},
						{
							Index:     "4",
							GPUInstID: 9,
						},
					},
					InstancesEnabled: true,
				},
			},
			orderMap: "0:0,1:2,2:1.3,3:1.5,4:1.9,5:3,6:3.3",
		},
	}

	noOpLogger := noOpLogger

	for itc, tc := range testCases {
		g := GPUSMI{
			logger:  noOpLogger,
			Devices: tc.devs,
		}
		g.ReindexGPUs(tc.orderMap)
		assert.ElementsMatch(t, tc.expectedDevs, g.Devices, "Case %d", itc)
	}
}

func TestUpdateMdevs(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	var gpuDevices []Device

	// Select amd vendor
	for _, vendor := range g.vendors {
		if vendor.id == nvidia {
			gpuDevices, err = g.nvidiaGPUDevices(vendor)
			require.NoError(t, err)

			assert.Equal(t, getExpectedNvidiaDevs(), gpuDevices)
		}
	}

	g.Devices = gpuDevices

	expectedMdevs := map[string][]string{
		"0": {"c73f1fa6-489e-4834-9476-d70dabd98c40", "f9702ffa-fa28-414e-a52f-e7831fd5ce41"},
		"2": {"f0f4b97c-6580-48a6-ae1b-a807d6dfe08f"},
		"3": {"3b356d38-854e-48be-b376-00c72c7d119c", "5bb3bad7-ce3b-4aa5-84d7-b5b33cf9d45e"},
		"5": {"4f84d324-5897-48f3-a4ef-94c9ddf23d78"},
		"6": {"3058eb95-0899-4c3d-90e9-e20b6c14789f"},
		"7": {"9f0d5993-9778-40c7-a721-3fec93d6b3a9"},
		"9": {"64c3c4ae-44e1-45b8-8d46-5f76a1fa9824"},
	}

	// Now updates gpuDevices with mdevs
	err = g.UpdateGPUMdevs()
	require.NoError(t, err)

	// Make a map of Index to mdevs
	gotMdevs := make(map[string][]string)

	for _, gpu := range g.Devices {
		if gpu.Index != "" {
			if len(gpu.MdevUUIDs) > 0 {
				gotMdevs[gpu.Index] = gpu.MdevUUIDs
			}
		} else {
			for _, inst := range gpu.Instances {
				if len(inst.MdevUUIDs) > 0 {
					gotMdevs[inst.Index] = inst.MdevUUIDs
				}
			}
		}
	}

	assert.Equal(t, expectedMdevs, gotMdevs)
}

func TestUpdateMdevsEviction(t *testing.T) {
	nvidiaVGPULog := `GPU 00000000:10:00.0
    Active vGPUs                      : 1
    vGPU ID                           : 3251634213
        MDEV UUID                     : c73f1fa6-489e-4834-9476-d70dabd98c40
        GPU Instance ID               : N/A

		`
	tempDir := t.TempDir()
	nvidiaSMIPath := filepath.Join(tempDir, "nvidia-smi")
	content := fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, nvidiaVGPULog)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", nvidiaSMIPath,
			"--collector.gpu.type", "nvidia",
		},
	)
	require.NoError(t, err)

	// Set devices
	devs := []Device{
		{
			BusID:       BusID{0x0, 0x10, 0x0, 0x0, ""},
			VGPUEnabled: true,
		},
	}

	g, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	g.Devices = devs

	// Now updates gpuDevices with mdevs
	err = g.UpdateGPUMdevs()
	require.NoError(t, err)
	assert.Equal(t, []string{"c73f1fa6-489e-4834-9476-d70dabd98c40"}, g.Devices[0].MdevUUIDs)

	// Update nvidia-smi output to simulate a new mdev addition
	nvidiaVGPULog = `GPU 00000000:10:00.0
    Active vGPUs                      : 2
    vGPU ID                           : 3251634213
        MDEV UUID                     : c73f1fa6-489e-4834-9476-d70dabd98c40
        GPU Instance ID               : N/A

	vGPU ID                           : 3251634214
        MDEV UUID                     : 741ac383-27e9-49a9-9955-b513ad2e2e16
        GPU Instance ID               : N/A

		`
	content = fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, nvidiaVGPULog)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	// Now update gpuDevices again with mdevs
	err = g.UpdateGPUMdevs()
	require.NoError(t, err)
	assert.Equal(t, []string{"c73f1fa6-489e-4834-9476-d70dabd98c40", "741ac383-27e9-49a9-9955-b513ad2e2e16"}, g.Devices[0].MdevUUIDs)

	// Update nvidia-smi output to simulate removal of an existing mdev
	nvidiaVGPULog = `GPU 00000000:10:00.0
    Active vGPUs                      : 1
	vGPU ID                           : 3251634214
        MDEV UUID                     : 741ac383-27e9-49a9-9955-b513ad2e2e16
        GPU Instance ID               : N/A

		`
	content = fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, nvidiaVGPULog)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec

	// Now update gpuDevices again with mdevs
	err = g.UpdateGPUMdevs()
	require.NoError(t, err)
	assert.Equal(t, []string{"741ac383-27e9-49a9-9955-b513ad2e2e16"}, g.Devices[0].MdevUUIDs)
}

// func TestParseAMDDevPropertiesFromPCIDevices(t *testing.T) {
// 	_, err := CEEMSExporterApp.Parse(
// 		[]string{
// 			"--path.sysfs", "testdata/sys",
// 		},
// 	)
// 	require.NoError(t, err)

// 	_, err = parseAMDDevPropertiesFromPCIDevices()
// 	require.NoError(t, err)

// 	assert.Fail(t, "QQQ")
// }

func TestParseBusIDPass(t *testing.T) {
	id := "00000000:AD:00.0"
	busID, err := parseBusID(id)
	require.NoError(t, err)

	expectedID := BusID{domain: 0x0, bus: 0xad, device: 0x0, function: 0x0, pathName: "00000000:ad:00.0"}

	assert.Equal(t, expectedID, busID)
}

func TestParseBusIDFail(t *testing.T) {
	// Missing component
	id := "00000000:AD:00"
	_, err := parseBusID(id)
	require.Error(t, err)

	// Malformed ID
	id = "00000000:AD:00:4"
	_, err = parseBusID(id)
	require.Error(t, err)

	// Not Hex
	id = "ggggggg:AD:00:0"
	_, err = parseBusID(id)
	require.Error(t, err)
}

func TestCompareBusIDs(t *testing.T) {
	// Sample Device
	d := Device{BusID: BusID{domain: 0x0, bus: 0xad, device: 0x0, function: 0x0}}

	// Test ID - pass
	id := "00000000:AD:00.0"
	assert.True(t, d.CompareBusID(id))

	// Test ID - fail
	id = "00000000:AD:0A.0"
	assert.False(t, d.CompareBusID(id))

	// Test ID - error fail
	id = "00000000:AD:00"
	assert.False(t, d.CompareBusID(id))
}
