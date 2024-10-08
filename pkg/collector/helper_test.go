// Taken from node_exporter/collectors/paths_test.go and modified

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

var (
	expectedNvidiaSmiOutput = `index, name, uuid, bus_id
0, Tesla V100-SXM2-32GB, GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e, 00000000:07:00.0
1, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3, 00000000:0B:00.0
2, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3, 00000000:48:00.0
3, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3, 00000000:4C:00.0`
	expectedAmdSmiOutput = `device,Serial Number,PCI Bus,Card series,Card model,Card vendor,Card SKU
card0,20170000800c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card1,20170003580c,0000:C8:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card2,20180003050c,0000:8A:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card3,20170005280c,0000:8D:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317`
)

func getExpectedNvidiaDevs() map[int]Device {
	nvidiaDevs := make(map[int]Device, 4)
	nvidiaDevs[0] = Device{
		index: "0",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
		busID: BusID{domain: 0x0, bus: 0x7, slot: 0x0, function: 0x0},
		isMig: false,
	}
	nvidiaDevs[1] = Device{
		index: "1",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
		busID: BusID{domain: 0x0, bus: 0xb, slot: 0x0, function: 0x0},
		isMig: false,
	}
	nvidiaDevs[2] = Device{
		index: "2",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3",
		busID: BusID{domain: 0x0, bus: 0x48, slot: 0x0, function: 0x0},
		isMig: false,
	}
	nvidiaDevs[3] = Device{
		index: "3",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3",
		busID: BusID{domain: 0x0, bus: 0x4c, slot: 0x0, function: 0x0},
		isMig: false,
	}

	return nvidiaDevs
}

func getExpectedAmdDevs() map[int]Device {
	amdDevs := make(map[int]Device, 4)
	amdDevs[0] = Device{
		index: "0",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20170000800c",
		busID: BusID{domain: 0x0, bus: 0xc5, slot: 0x0, function: 0x0},
		isMig: false,
	}
	amdDevs[1] = Device{
		index: "1",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20170003580c",
		busID: BusID{domain: 0x0, bus: 0xc8, slot: 0x0, function: 0x0},
		isMig: false,
	}
	amdDevs[2] = Device{
		index: "2",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20180003050c",
		busID: BusID{domain: 0x0, bus: 0x8a, slot: 0x0, function: 0x0},
		isMig: false,
	}
	amdDevs[3] = Device{
		index: "3",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20170005280c",
		busID: BusID{domain: 0x0, bus: 0x8d, slot: 0x0, function: 0x0},
		isMig: false,
	}

	return amdDevs
}

func TestParseNvidiaSmiOutput(t *testing.T) {
	tempDir := t.TempDir()
	nvidiaSMIPath := filepath.Join(tempDir, "nvidia-smi")
	content := fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, expectedNvidiaSmiOutput)
	os.WriteFile(nvidiaSMIPath, []byte(content), 0o700) // #nosec
	gpuDevices, err := GetNvidiaGPUDevices(nvidiaSMIPath, log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, gpuDevices, getExpectedNvidiaDevs())
}

func TestParseAmdSmiOutput(t *testing.T) {
	tempDir := t.TempDir()
	amdSMIPath := filepath.Join(tempDir, "amd-smi")
	content := fmt.Sprintf(`#!/bin/bash
echo """%s"""	
`, expectedAmdSmiOutput)
	os.WriteFile(amdSMIPath, []byte(content), 0o700) // #nosec
	gpuDevices, err := GetAMDGPUDevices(amdSMIPath, log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, gpuDevices, getExpectedAmdDevs())
}

func TestParseBusIDPass(t *testing.T) {
	id := "00000000:AD:00.0"
	busID, err := parseBusID(id)
	require.NoError(t, err)

	expectedID := BusID{domain: 0x0, bus: 0xad, slot: 0x0, function: 0x0}

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
	d := Device{busID: BusID{domain: 0x0, bus: 0xad, slot: 0x0, function: 0x0}}

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
