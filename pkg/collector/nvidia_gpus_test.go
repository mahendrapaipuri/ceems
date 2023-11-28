//go:build !nonvidia
// +build !nonvidia

package collector

import (
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
)

var (
	devices = []Device{{name: "fakeGpu1",
		uuid:  "GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e",
		isMig: false}, {name: "fakeGpu2",
		uuid:  "GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3",
		isMig: false}}
)

func TestNvidiaJobGpuMap(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--collector.nvidia.gpu.stat.path", "fixtures/gpustat"}); err != nil {
		t.Fatal(err)
	}
	c := nvidiaGpuJobMapCollector{devices: devices, logger: log.NewNopLogger()}
	gpuJobMapper, _ := c.getJobId()
	if gpuJobMapper["GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e"] != 10000 {
		t.Fatalf("Expected Job ID is %d: \nGot %f", 10000, gpuJobMapper["GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e"])
	}
	if gpuJobMapper["GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3"] != 11000 {
		t.Fatalf("Expected Job ID is %d: \nGot %f", 11000, gpuJobMapper["GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3"])
	}
}
