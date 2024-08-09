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
	expectedNvidiaSmiOutput = `index, name, uuid
0, Tesla V100-SXM2-32GB, GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e
1, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3
2, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3
3, Tesla V100-SXM2-32GB, GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3`
	expectedAmdSmiOutput = `device,Serial Number,Card series,Card model,Card vendor,Card SKU
card0,20170000800c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card1,20170003580c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card2,20180003050c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
card3,20170005280c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317`
)

func getExpectedNvidiaDevs() map[int]Device {
	nvidiaDevs := make(map[int]Device, 4)
	nvidiaDevs[0] = Device{
		index: "0",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
		isMig: false,
	}
	nvidiaDevs[1] = Device{
		index: "1",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
		isMig: false,
	}
	nvidiaDevs[2] = Device{
		index: "2",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3",
		isMig: false,
	}
	nvidiaDevs[3] = Device{
		index: "3",
		name:  "Tesla V100-SXM2-32GB",
		uuid:  "GPU-61a65011-6571-a64n-5ab8-66cbb6f7f9c3",
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
		isMig: false,
	}
	amdDevs[1] = Device{
		index: "1",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20170003580c",
		isMig: false,
	}
	amdDevs[2] = Device{
		index: "2",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20180003050c",
		isMig: false,
	}
	amdDevs[3] = Device{
		index: "3",
		name:  "deon Instinct MI50 32GB",
		uuid:  "20170005280c",
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
