package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var minNvGPUPower, maxNvGPUPower, minAMDGPUPower, maxAMDGPUPower float64

type Device struct {
	UUID    string
	IID     string
	PCIAddr string
}

// DCGM collector.
type dcgmCollector struct {
	devices        []Device
	gpuUtil        *prometheus.Desc
	gpuMemFree     *prometheus.Desc
	gpuMemUsed     *prometheus.Desc
	gpuPower       *prometheus.Desc
	gpuPowerInst   *prometheus.Desc
	gpuSMActive    *prometheus.Desc
	gpuSMOcc       *prometheus.Desc
	gpuGREngActive *prometheus.Desc
}

func randFloat(minVal, maxVal float64) float64 {
	return minVal + rand.Float64()*(maxVal-minVal) //nolint:gosec
}

func newDCGMCollector() *dcgmCollector {
	devices := []Device{
		{"GPU-956348bc-d43d-23ed-53d4-857749fa2b67", "0", "00000000:15:00.0"},
		{"GPU-956348bc-d43d-23ed-53d4-857749fa2b67", "1", "00000000:15:00.0"},
		{"GPU-feba7e40-d724-01ff-b00f-3a439a28a6c7", "", "00000000:16:00.0"},
		{"GPU-61a65011-6571-a6d2-5th8-66cbb6f7f9c3", "", "00000000:17:00.0"},
		{"GPU-1d4d0f3e-b51a-4040-96e3-bf380f7c5728", "", "00000000:18:00.0"},
	}

	return &dcgmCollector{
		devices: devices,
		gpuUtil: prometheus.NewDesc("DCGM_FI_DEV_GPU_UTIL",
			"GPU utilization",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuMemUsed: prometheus.NewDesc("DCGM_FI_DEV_FB_USED",
			"GPU memory used",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuMemFree: prometheus.NewDesc("DCGM_FI_DEV_FB_FREE",
			"GPU memory free",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuPower: prometheus.NewDesc("DCGM_FI_DEV_POWER_USAGE",
			"GPU power",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuPowerInst: prometheus.NewDesc("DCGM_FI_DEV_POWER_USAGE_INSTANT",
			"GPU power",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuSMActive: prometheus.NewDesc("DCGM_FI_PROF_SM_ACTIVE",
			"GPU SM active",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuSMOcc: prometheus.NewDesc("DCGM_FI_PROF_SM_OCCUPANCY",
			"GPU SM occupancy",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
		gpuGREngActive: prometheus.NewDesc("DCGM_FI_PROF_GR_ENGINE_ACTIVE",
			"GPU GR engien active",
			[]string{"Hostname", "UUID", "GPU_I_ID", "device", "gpu", "pci_bus_id", "modelName"}, nil,
		),
	}
}

// Describe writes all descriptors to the prometheus desc channel.
func (collector *dcgmCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.gpuUtil
	ch <- collector.gpuMemUsed
	ch <- collector.gpuMemFree
	ch <- collector.gpuPower
	ch <- collector.gpuSMActive
	ch <- collector.gpuSMOcc
	ch <- collector.gpuGREngActive
}

// Collect implements required collect function for all promehteus collectors.
func (collector *dcgmCollector) Collect(ch chan<- prometheus.Metric) {
	for idev, dev := range collector.devices {
		ch <- prometheus.MustNewConstMetric(
			collector.gpuUtil, prometheus.GaugeValue, 100*rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuMemUsed, prometheus.GaugeValue, 100*rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuMemFree, prometheus.GaugeValue, 100*rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)

		power := randFloat(minNvGPUPower, maxNvGPUPower)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuPower, prometheus.GaugeValue, power, "host", dev.UUID, dev.IID,
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuPowerInst, prometheus.GaugeValue, power, "host", dev.UUID, dev.IID,
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuSMActive, prometheus.GaugeValue, rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuSMOcc, prometheus.GaugeValue, rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuGREngActive, prometheus.GaugeValue, rand.Float64(), "host", dev.UUID, dev.IID, //nolint:gosec
			fmt.Sprintf("nvidia%d", idev), strconv.Itoa(idev), dev.PCIAddr, "NVIDIA A100 80GiB",
		)
	}
}

// AMD SMI collector.
type amdSMICollector struct {
	devices    []Device
	gpuUtil    *prometheus.Desc
	gpuMemUtil *prometheus.Desc
	gpuPower   *prometheus.Desc
}

func newAMDSMICollector() *amdSMICollector {
	devices := []Device{
		{"20170000800c", "", "00000000:15:00.0"},
		{"20170003580c", "", "00000000:16:00.0"},
		{"20180003050c", "", "00000000:17:00.0"},
		{"20170005280c", "", "00000000:18:00.0"},
	}

	return &amdSMICollector{
		devices: devices,
		gpuUtil: prometheus.NewDesc("amd_gpu_use_percent",
			"GPU utilization",
			[]string{"gpu_use_percent", "productname"}, nil,
		),
		gpuMemUtil: prometheus.NewDesc("amd_gpu_memory_use_percent",
			"GPU memory used",
			[]string{"gpu_memory_use_percent", "productname"}, nil,
		),
		gpuPower: prometheus.NewDesc("amd_gpu_power",
			"GPU power",
			[]string{"gpu_power", "productname"}, nil,
		),
	}
}

// Describe writes all descriptors to the prometheus desc channel.
func (collector *amdSMICollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.gpuUtil
	ch <- collector.gpuMemUtil
	ch <- collector.gpuPower
}

// Collect implements required collect function for all promehteus collectors.
func (collector *amdSMICollector) Collect(ch chan<- prometheus.Metric) {
	for idev := range collector.devices {
		ch <- prometheus.MustNewConstMetric(
			collector.gpuUtil, prometheus.GaugeValue, 100*rand.Float64(), strconv.Itoa(idev), //nolint:gosec
			"Advanced Micro Devices Inc",
		)
		ch <- prometheus.MustNewConstMetric(
			collector.gpuMemUtil, prometheus.GaugeValue, 100*rand.Float64(), strconv.Itoa(idev), //nolint:gosec
			"Advanced Micro Devices Inc",
		)
		// GPU power reported in micro Watts
		ch <- prometheus.MustNewConstMetric(
			collector.gpuPower, prometheus.GaugeValue, randFloat(minAMDGPUPower, maxAMDGPUPower), strconv.Itoa(idev),
			"Advanced Micro Devices Inc",
		)
	}
}

func dcgmExporter(ctx context.Context) {
	dcgm := newDCGMCollector()
	dcgmRegistry := prometheus.NewRegistry()
	dcgmRegistry.MustRegister(dcgm)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", promhttp.HandlerFor(dcgmRegistry, promhttp.HandlerOpts{}).ServeHTTP)

	// Start server
	server := &http.Server{
		Addr:              ":9400",
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           mux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Pyroscope server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func amdSMIExporter(ctx context.Context) {
	amdSMI := newAMDSMICollector()
	amdRegistry := prometheus.NewRegistry()
	amdRegistry.MustRegister(amdSMI)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", promhttp.HandlerFor(amdRegistry, promhttp.HandlerOpts{}).ServeHTTP)

	// Start server
	server := &http.Server{
		Addr:              ":9500",
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           mux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Pyroscope server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.Println("Starting fake exporters")

	args := os.Args[1:]

	// Registering our handler functions, and creating paths.
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	// For e2e tests use constant power usage for reproducibility
	if slices.Contains(args, "test-mode") {
		minNvGPUPower = 200.0
		maxNvGPUPower = 200.0
		minAMDGPUPower = 100000000.0
		maxAMDGPUPower = 100000000.0
	} else {
		minNvGPUPower = 60.0
		maxNvGPUPower = 700.0
		minAMDGPUPower = 30000000.0
		maxAMDGPUPower = 500000000.0
	}

	if slices.Contains(args, "dcgm") {
		go func() {
			dcgmExporter(ctx)
		}()
	}

	if slices.Contains(args, "amd-smi") {
		go func() {
			amdSMIExporter(ctx)
		}()
	}

	sig := <-sigs
	log.Println(sig)

	cancel()

	log.Println("Fake exporters have been stopped")
}
