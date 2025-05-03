//go:build !nok8s
// +build !nok8s

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	ceems_k8s "github.com/mahendrapaipuri/ceems/pkg/k8s"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func TestNewK8sCollector(t *testing.T) {
	tmpDir := t.TempDir()

	// Test k8s API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if data, err := os.ReadFile("testdata/k8s/pods-metadata.json"); err == nil {
			w.Header().Add("Content-Type", "application/json")
			w.Header().Add("Content-Type", "application/vnd.kubernetes.protobuf")
			w.Write(data)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("KO"))
	}))

	defer server.Close()

	content := `
apiVersion: v1
clusters:
- cluster:
    server: %s
  name: foo-cluster
contexts:
- context:
    cluster: foo-cluster
    user: foo-user
    namespace: bar
  name: foo-context
current-context: foo-context
kind: Config
users:
- name: foo-user
  user:
    token: blue-token
`
	kubeConfigFile := filepath.Join(tmpDir, "kubeconfig")

	err := os.WriteFile(kubeConfigFile, []byte(fmt.Sprintf(content, server.URL)), 0o700) //nolint:gosec
	require.NoError(t, err)

	// Read pod resource response json
	podResourceContent, err := os.ReadFile("testdata/k8s/nvidia-pod-resources.json")
	require.NoError(t, err)

	var podResourcesResp podresourcesapi.ListPodResourcesResponse
	err = json.Unmarshal(podResourceContent, &podResourcesResp)
	require.NoError(t, err)

	// Create fake kubelet socket server
	socketFile := filepath.Join(tmpDir, "kubelet.sock")
	kubelet, err := ceems_k8s.FakeKubeletServer(tmpDir, &podResourcesResp, nil)
	require.NoError(t, err)

	defer kubelet.Server.Stop()

	_, err = CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--path.sysfs", "testdata/sys",
			"--collector.k8s.psi-metrics",
			"--collector.k8s.kubelet-socket-file", socketFile,
			"--collector.perf.hardware-events",
			"--collector.rdma.stats",
			"--collector.gpu.type", "nvidia",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.cgroups.force-version", "v2",
			"--collector.k8s.kube-config-file", kubeConfigFile,
		},
	)
	require.NoError(t, err)

	collector, err := NewK8sCollector(noOpLogger)
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

func TestK8sPodDevices(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--collector.cgroups.force-version", "v2",
			"--collector.gpu.type", "nvidia",
			"--collector.gpu.nvidia-smi-path", "testdata/nvidia-smi",
		},
	)
	require.NoError(t, err)

	// Read pod resource response json
	content, err := os.ReadFile("testdata/k8s/nvidia-pod-resources.json")
	require.NoError(t, err)

	var podResourcesResp podresourcesapi.ListPodResourcesResponse
	err = json.Unmarshal(content, &podResourcesResp)
	require.NoError(t, err)

	// Read pods metadata response json
	content, err = os.ReadFile("testdata/k8s/pods-metadata.json")
	require.NoError(t, err)

	var podsMetadata v1.PodList
	err = json.Unmarshal(content, &podsMetadata)
	require.NoError(t, err)

	var podsMetadataRTObj []runtime.Object
	for _, v := range podsMetadata.Items {
		podsMetadataRTObj = append(podsMetadataRTObj, &v)
	}

	// Make fake client
	fakeClientset := fake.NewClientset(podsMetadataRTObj...)

	// Create fake kubelet socket server
	socketDir := t.TempDir()
	kubelet, err := ceems_k8s.FakeKubeletServer(socketDir, &podResourcesResp, nil)
	require.NoError(t, err)

	defer kubelet.Server.Stop()

	// Create connection
	conn, err := ceems_k8s.ConnectToServer(filepath.Join(socketDir, "kubelet.sock"))
	require.NoError(t, err)

	defer conn.Close()

	kubeletClient := podresourcesapi.NewPodResourcesListerClient(conn)

	client := &ceems_k8s.Client{
		Logger:            noOpLogger,
		Clientset:         fakeClientset,
		PodResourceClient: kubeletClient,
	}

	// Get GPU devs
	gpu, err := NewGPUSMI(nil, noOpLogger)
	require.NoError(t, err)

	// Attempt to get GPU devices
	err = gpu.Discover()
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager(k8s, noOpLogger)
	require.NoError(t, err)

	c := k8sCollector{
		cgroupManager: cgManager,
		gpuSMI:        gpu,
		logger:        noOpLogger,
		k8sClient:     client,
	}

	cgroups, err := c.podCgroups()
	require.NoError(t, err)

	expectedPodIDs := []string{
		"3a61e77f-1538-476b-8231-5af9eed40fdc",
		"6232f0c5-57fa-409a-b026-0919f60e24a6",
		"964d7f07-f686-4ddc-991b-719febfba554",
		"c316ac07-9b95-4d1f-9f54-8138d2058f7a",
		"d3240201-ca7a-4bf0-944f-f788edc0e433",
		"483168fc-b347-4aa2-a9fa-9e3d220ba4c5",
		"6d06282c-0377-4527-9a0f-9968bc9c4102",
		"9e524754-b3df-44e2-bb72-1fa05a9119ee",
		"6c22124f-e9a7-450b-8915-9bf3e0716d78",
		"7a303ecc-ce90-4d0c-bb56-49d6bdad19f7",
	}

	assert.ElementsMatch(t, expectedPodIDs, c.previousPodUIDs)

	// Check number of pods
	assert.Len(t, cgroups, 10)

	// Expected compute units
	expectedPodDeviceMappsers := map[string][]ComputeUnit{
		"0": {{UUID: "6c22124f-e9a7-450b-8915-9bf3e0716d78", NumShares: 1}},
		"1": {{UUID: "6c22124f-e9a7-450b-8915-9bf3e0716d78", NumShares: 1}},
		"2": {},
		"3": {{UUID: "6c22124f-e9a7-450b-8915-9bf3e0716d78", NumShares: 1}},
		"4": {},
		"5": {
			{UUID: "483168fc-b347-4aa2-a9fa-9e3d220ba4c5", NumShares: 4},
			{UUID: "6232f0c5-57fa-409a-b026-0919f60e24a6", NumShares: 2},
			{UUID: "3a61e77f-1538-476b-8231-5af9eed40fdc", NumShares: 2},
		},
		"6": {},
		"7": {{UUID: "6232f0c5-57fa-409a-b026-0919f60e24a6", NumShares: 1}},
		"8": {
			{UUID: "6232f0c5-57fa-409a-b026-0919f60e24a6", NumShares: 2},
			{UUID: "3a61e77f-1538-476b-8231-5af9eed40fdc", NumShares: 2},
		},
		"9":  {{UUID: "3a61e77f-1538-476b-8231-5af9eed40fdc", NumShares: 3}},
		"10": {},
		"11": {{UUID: "3a61e77f-1538-476b-8231-5af9eed40fdc", NumShares: 1}},
	}

	for _, gpu := range c.gpuSMI.Devices {
		assert.ElementsMatch(t, expectedPodDeviceMappsers[gpu.Index], gpu.ComputeUnits, "GPU %s", gpu.Index)

		for _, inst := range gpu.Instances {
			assert.ElementsMatch(t, expectedPodDeviceMappsers[inst.Index], inst.ComputeUnits, "MIG %s", inst.Index)
		}
	}
}

// func TestK8sConfig(t *testing.T) {
// 	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if data, err := os.ReadFile("testdata/k8s/pods-metadata.json"); err == nil {
// 			w.Header().Add("Content-Type", "application/json")
// 			w.Header().Add("Content-Type", "application/vnd.kubernetes.protobuf")
// 			w.Write(data)

// 			return
// 		}
// 	}))

// 	defer server.Close()

// 	content := `
// apiVersion: v1
// clusters:
// - cluster:
//     server: %s`

// 	tmpfile, err := os.CreateTemp(t.TempDir(), "kubeconfig")
// 	require.NoError(t, err)

// 	err = os.WriteFile(tmpfile.Name(), []byte(fmt.Sprintf(content, server.URL)), 0o700) //nolint:gosec
// 	require.NoError(t, err)

// 	c, err := ceems_k8s.New(tmpfile.Name(), "", noOpLogger)
// 	require.NoError(t, err)

// 	fmt.Println(c.Pods(context.Background(), "", metav1.ListOptions{}))

// 	assert.Fail(t, "AAA")
// }
