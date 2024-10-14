package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getExpectedNvidiaDevs() []Device {
	return []Device{
		{
			localIndex:  "0",
			globalIndex: "0",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
			busID:       BusID{domain: 0x0, bus: 0x10, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: true,
		},
		{
			localIndex:  "1",
			globalIndex: "1",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
			busID:       BusID{domain: 0x0, bus: 0x15, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex: "2",
			name:       "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:       "GPU-956348bc-d43d-23ed-53d4-857749fa2b67",
			busID:      BusID{domain: 0x0, bus: 0x21, device: 0x0, function: 0x0},
			migInstances: []MIGInstance{
				{localIndex: 0x0, globalIndex: "2", computeInstID: 0x0, gpuInstID: 0x1, smFraction: 0.6},
				{localIndex: 0x1, globalIndex: "3", computeInstID: 0x0, gpuInstID: 0x5, smFraction: 0.2},
				{localIndex: 0x2, globalIndex: "4", computeInstID: 0x0, gpuInstID: 0xd, smFraction: 0.2},
			},
			migEnabled:  true,
			vgpuEnabled: true,
		},
		{
			localIndex: "3",
			name:       "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:       "GPU-feba7e40-d724-01ff-b00f-3a439a28a6c7",
			busID:      BusID{domain: 0x0, bus: 0x81, device: 0x0, function: 0x0},
			migInstances: []MIGInstance{
				{localIndex: 0x0, globalIndex: "5", computeInstID: 0x0, gpuInstID: 0x1, smFraction: 0.5714285714285714},
				{localIndex: 0x1, globalIndex: "6", computeInstID: 0x0, gpuInstID: 0x5, smFraction: 0.2857142857142857},
				{localIndex: 0x2, globalIndex: "7", computeInstID: 0x0, gpuInstID: 0x6, smFraction: 0.14285714285714285},
			},
			migEnabled:  true,
			vgpuEnabled: true,
		},
		{
			localIndex:  "4",
			globalIndex: "8",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3",
			busID:       BusID{domain: 0x0, bus: 0x83, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex:  "5",
			globalIndex: "9",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3",
			busID:       BusID{domain: 0x0, bus: 0x85, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: true,
		},
		{
			localIndex:  "6",
			globalIndex: "10",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-1d4d0f3e-b51a-4040-96e3-bf380f7c5728",
			busID:       BusID{domain: 0x0, bus: 0x87, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex:  "7",
			globalIndex: "11",
			name:        "NVIDIA A100-PCIE-40GB NVIDIA Ampere",
			uuid:        "GPU-6cc98505-fdde-461e-a93c-6935fba45a27",
			busID:       BusID{domain: 0x0, bus: 0x89, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
	}
}

func getExpectedAmdDevs() []Device {
	return []Device{
		{
			localIndex:  "0",
			globalIndex: "0",
			name:        "deon Instinct MI50 32GB",
			uuid:        "20170000800c",
			busID:       BusID{domain: 0x0, bus: 0xc5, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex:  "1",
			globalIndex: "1",
			name:        "deon Instinct MI50 32GB",
			uuid:        "20170003580c",
			busID:       BusID{domain: 0x0, bus: 0xc8, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex:  "2",
			globalIndex: "2",
			name:        "deon Instinct MI50 32GB",
			uuid:        "20180003050c",
			busID:       BusID{domain: 0x0, bus: 0x8a, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
		{
			localIndex:  "3",
			globalIndex: "3",
			name:        "deon Instinct MI50 32GB",
			uuid:        "20170005280c",
			busID:       BusID{domain: 0x0, bus: 0x8d, device: 0x0, function: 0x0},
			migEnabled:  false,
			vgpuEnabled: false,
		},
	}
}

func TestParseNvidiaSmiOutput(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
		},
	)
	require.NoError(t, err)

	gpuDevices, err := GetNvidiaGPUDevices(log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, getExpectedNvidiaDevs(), gpuDevices)
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
		},
	)
	require.NoError(t, err)

	gpuDevices, err := GetNvidiaGPUDevices(log.NewNopLogger())
	require.NoError(t, err)

	// Check if globalIndex for GPU 0 is empty and GPU 1 is 3
	assert.Empty(t, gpuDevices[0].globalIndex)
	assert.Equal(t, "3", gpuDevices[1].globalIndex)
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
		},
	)
	require.NoError(t, err)

	gpuDevices, err := GetNvidiaGPUDevices(log.NewNopLogger())
	require.NoError(t, err)

	// Check if globalIndex for GPU 1 is empty and GPU 0 is 0
	assert.Empty(t, gpuDevices[1].globalIndex)
	assert.Equal(t, "0", gpuDevices[0].globalIndex)
}

func TestParseAmdSmiOutput(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.rocm-smi-path", "testdata/rocm-smi",
		},
	)
	require.NoError(t, err)
	gpuDevices, err := GetAMDGPUDevices(log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, getExpectedAmdDevs(), gpuDevices)
}

func TestReindexGPUs(t *testing.T) {
	testCases := []struct {
		name         string
		devs         []Device
		expectedDevs []Device
		orderMap     string
	}{
		{
			devs: []Device{
				{
					globalIndex: "",
					localIndex:  "0",
					migInstances: []MIGInstance{
						{
							globalIndex: "0",
							gpuInstID:   3,
						},
						{
							globalIndex: "1",
							gpuInstID:   5,
						},
						{
							globalIndex: "2",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
				{
					globalIndex: "1",
					localIndex:  "1",
				},
			},
			expectedDevs: []Device{
				{
					globalIndex: "",
					localIndex:  "0",
					migInstances: []MIGInstance{
						{
							globalIndex: "1",
							gpuInstID:   3,
						},
						{
							globalIndex: "2",
							gpuInstID:   5,
						},
						{
							globalIndex: "3",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
				{
					globalIndex: "0",
					localIndex:  "1",
				},
			},
			orderMap: "0:1,1:0.3,2:0.5,3:0.9",
		},
		{
			devs: []Device{
				{
					globalIndex: "0",
					localIndex:  "0",
				},
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "1",
							gpuInstID:   3,
						},
						{
							globalIndex: "2",
							gpuInstID:   5,
						},
						{
							globalIndex: "3",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
			},
			expectedDevs: []Device{
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "0",
							gpuInstID:   3,
						},
						{
							globalIndex: "1",
							gpuInstID:   5,
						},
						{
							globalIndex: "2",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
				{
					globalIndex: "3",
					localIndex:  "0",
				},
			},
			orderMap: "0:1.3,1:1.5,2:1.9,3:0",
		},
		{
			devs: []Device{
				{
					globalIndex: "0",
					localIndex:  "0",
				},
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "1",
							gpuInstID:   3,
						},
						{
							globalIndex: "2",
							gpuInstID:   5,
						},
						{
							globalIndex: "3",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
				{
					globalIndex: "4",
					localIndex:  "2",
				},
			},
			expectedDevs: []Device{
				{
					globalIndex: "0",
					localIndex:  "0",
				},
				{
					globalIndex: "1",
					localIndex:  "2",
				},
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "2",
							gpuInstID:   3,
						},
						{
							globalIndex: "3",
							gpuInstID:   5,
						},
						{
							globalIndex: "4",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
			},
			orderMap: "0:0,1:2,2:1.3,3:1.5,4:1.9",
		},
		{
			devs: []Device{
				{
					globalIndex: "0",
					localIndex:  "0",
				},
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "1",
							gpuInstID:   3,
						},
						{
							globalIndex: "2",
							gpuInstID:   5,
						},
						{
							globalIndex: "3",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
				{
					globalIndex: "4",
					localIndex:  "2",
				},
			},
			expectedDevs: []Device{
				{
					globalIndex: "0",
					localIndex:  "0",
				},
				{
					globalIndex: "1",
					localIndex:  "2",
				},
				{
					globalIndex: "",
					localIndex:  "1",
					migInstances: []MIGInstance{
						{
							globalIndex: "2",
							gpuInstID:   3,
						},
						{
							globalIndex: "3",
							gpuInstID:   5,
						},
						{
							globalIndex: "4",
							gpuInstID:   9,
						},
					},
					migEnabled: true,
				},
			},
			orderMap: "0:0,1:2,2:1.3,3:1.5,4:1.9,5:3,6:3.3",
		},
	}

	for itc, tc := range testCases {
		newDevs := reindexGPUs(tc.orderMap, tc.devs)
		assert.ElementsMatch(t, tc.expectedDevs, newDevs, "Case %d", itc)
	}
}

func TestUpdateMdevs(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
		},
	)
	require.NoError(t, err)

	gpuDevices, err := GetNvidiaGPUDevices(log.NewNopLogger())
	require.NoError(t, err)

	expectedDevs := []Device{
		{
			localIndex: "0", globalIndex: "0", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
			busID:      BusID{domain: 0x0, bus: 0x10, device: 0x0, function: 0x0},
			mdevUUIDs:  []string{"c73f1fa6-489e-4834-9476-d70dabd98c40", "f9702ffa-fa28-414e-a52f-e7831fd5ce41"},
			migEnabled: false, vgpuEnabled: true,
		},
		{
			localIndex: "1", globalIndex: "1", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
			busID:      BusID{domain: 0x0, bus: 0x15, device: 0x0, function: 0x0},
			migEnabled: false, vgpuEnabled: false,
		},
		{
			localIndex: "2", globalIndex: "", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-956348bc-d43d-23ed-53d4-857749fa2b67",
			busID: BusID{domain: 0x0, bus: 0x21, device: 0x0, function: 0x0},
			migInstances: []MIGInstance{
				{localIndex: 0x0, globalIndex: "2", computeInstID: 0x0, gpuInstID: 0x1, smFraction: 0.6, mdevUUIDs: []string{"f0f4b97c-6580-48a6-ae1b-a807d6dfe08f"}},
				{localIndex: 0x1, globalIndex: "3", computeInstID: 0x0, gpuInstID: 0x5, smFraction: 0.2, mdevUUIDs: []string{"3b356d38-854e-48be-b376-00c72c7d119c", "5bb3bad7-ce3b-4aa5-84d7-b5b33cf9d45e"}},
				{localIndex: 0x2, globalIndex: "4", computeInstID: 0x0, gpuInstID: 0xd, smFraction: 0.2, mdevUUIDs: []string{}},
			},
			migEnabled: true, vgpuEnabled: true,
		},
		{
			localIndex: "3", globalIndex: "", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-feba7e40-d724-01ff-b00f-3a439a28a6c7",
			busID: BusID{domain: 0x0, bus: 0x81, device: 0x0, function: 0x0},
			migInstances: []MIGInstance{
				{localIndex: 0x0, globalIndex: "5", computeInstID: 0x0, gpuInstID: 0x1, smFraction: 0.5714285714285714, mdevUUIDs: []string{"4f84d324-5897-48f3-a4ef-94c9ddf23d78"}},
				{localIndex: 0x1, globalIndex: "6", computeInstID: 0x0, gpuInstID: 0x5, smFraction: 0.2857142857142857, mdevUUIDs: []string{"3058eb95-0899-4c3d-90e9-e20b6c14789f"}},
				{localIndex: 0x2, globalIndex: "7", computeInstID: 0x0, gpuInstID: 0x6, smFraction: 0.14285714285714285, mdevUUIDs: []string{"9f0d5993-9778-40c7-a721-3fec93d6b3a9"}},
			},
			migEnabled: true, vgpuEnabled: true,
		},
		{
			localIndex: "4", globalIndex: "8", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3",
			busID:      BusID{domain: 0x0, bus: 0x83, device: 0x0, function: 0x0},
			migEnabled: false, vgpuEnabled: false,
		},
		{
			localIndex: "5", globalIndex: "9", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3",
			busID:      BusID{domain: 0x0, bus: 0x85, device: 0x0, function: 0x0},
			mdevUUIDs:  []string{"64c3c4ae-44e1-45b8-8d46-5f76a1fa9824"},
			migEnabled: false, vgpuEnabled: true,
		},
		{
			localIndex: "6", globalIndex: "10", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-1d4d0f3e-b51a-4040-96e3-bf380f7c5728",
			busID:      BusID{domain: 0x0, bus: 0x87, device: 0x0, function: 0x0},
			migEnabled: false, vgpuEnabled: false,
		},
		{
			localIndex: "7", globalIndex: "11", name: "NVIDIA A100-PCIE-40GB NVIDIA Ampere", uuid: "GPU-6cc98505-fdde-461e-a93c-6935fba45a27",
			busID:      BusID{domain: 0x0, bus: 0x89, device: 0x0, function: 0x0},
			migEnabled: false, vgpuEnabled: false,
		},
	}

	// Now updates gpuDevices with mdevs
	updatedGPUDevs, err := updateGPUMdevs(gpuDevices)
	require.NoError(t, err)
	assert.EqualValues(t, expectedDevs, updatedGPUDevs)
}

func TestParseBusIDPass(t *testing.T) {
	id := "00000000:AD:00.0"
	busID, err := parseBusID(id)
	require.NoError(t, err)

	expectedID := BusID{domain: 0x0, bus: 0xad, device: 0x0, function: 0x0}

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
	d := Device{busID: BusID{domain: 0x0, bus: 0xad, device: 0x0, function: 0x0}}

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
