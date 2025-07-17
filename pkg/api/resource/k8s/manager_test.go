package k8s

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	start, _   = time.Parse(base.DatetimezoneLayout, "2025-07-07T12:00:00+0200")
	end, _     = time.Parse(base.DatetimezoneLayout, "2025-07-07T12:15:00+0200")
	current, _ = time.Parse(base.DatetimezoneLayout, "2025-07-07T12:15:00+0200")

	noOpLogger    = slog.New(slog.DiscardHandler)
	expectedUnits = map[string]models.Unit{
		"3a61e77f-1538-476b-8231-5af9eed40fdc": {
			ClusterID:       "k8s-0",
			ResourceManager: "k8s",
			UUID:            "3a61e77f-1538-476b-8231-5af9eed40fdc",
			Name:            "pod31",
			Project:         "ns3",
			Group:           "",
			User:            "kusr3",
			CreatedAt:       "2025-07-07T11:16:56+0200",
			StartedAt:       "N/A",
			EndedAt:         "N/A",
			CreatedAtTS:     1751879816000,
			StartedAtTS:     0,
			EndedAtTS:       0,
			Elapsed:         "N/A",
			State:           "Pending",
			Allocation: models.Generic{
				"mem": 3.221225472e+09, "nvidia.com/gpu": 1.0, "nvidia.com/mig-4g.20gb": 2.0, "vcpus": 3.0,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 0, "alloc_cputime": 0, "alloc_gpumemtime": 0, "alloc_gputime": 0, "walltime": 0,
			},
			Tags: models.Generic{
				"annotations": map[string]string{"ceems.io/created-by": "kusr3"}, "qos": "Burstable",
			},
		},
		"6c22124f-e9a7-450b-8915-9bf3e0716d78": {
			ClusterID:       "k8s-0",
			ResourceManager: "k8s",
			UUID:            "6c22124f-e9a7-450b-8915-9bf3e0716d78",
			Name:            "pod11",
			Project:         "ns1",
			Group:           "",
			User:            "kusr1",
			CreatedAt:       "2025-07-07T10:56:56+0200",
			StartedAt:       "2025-07-07T10:56:58+0200",
			EndedAt:         "N/A",
			CreatedAtTS:     1751878616000,
			StartedAtTS:     1751878618000,
			EndedAtTS:       0,
			Elapsed:         "01:18:04",
			State:           "Running/Ready",
			Allocation: models.Generic{
				"mem": 1.048576e+08, "nvidia.com/gpu": 2.0, "nvidia.com/mig-1g.5gb": 1.0, "vcpus": 0.1,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 9.437184e+10, "alloc_cputime": 90, "alloc_gpumemtime": 2700, "alloc_gputime": 2700, "walltime": 900,
			},
			Tags: models.Generic{
				"annotations": map[string]string{"ceems.io/created-by": "kusr1"}, "qos": "BestEffort",
			},
		},
		"483168fc-b347-4aa2-a9fa-9e3d220ba4c5": {
			ClusterID:       "k8s-0",
			ResourceManager: "k8s",
			UUID:            "483168fc-b347-4aa2-a9fa-9e3d220ba4c5",
			Name:            "pod21",
			Project:         "ns2",
			Group:           "",
			User:            "ns2:" + base.UnknownUser,
			CreatedAt:       "2025-07-07T11:56:56+0200",
			StartedAt:       "2025-07-07T11:56:58+0200",
			EndedAt:         "N/A",
			CreatedAtTS:     1751882216000,
			StartedAtTS:     1751882218000,
			EndedAtTS:       0,
			Elapsed:         "00:18:04",
			State:           "Running/Ready",
			Allocation: models.Generic{
				"mem": 2.097152e+08, "nvidia.com/mig-4g.20gb": 4.0, "vcpus": 0.2,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 1.8874368e+11, "alloc_cputime": 180, "alloc_gpumemtime": 3600, "alloc_gputime": 3600, "walltime": 900,
			},
			Tags: models.Generic{
				"annotations": map[string]string{"ceems.io/created-by": "system:serviceaccount", "ceems.io/project": "ns2"}, "qos": "Guaranteed",
			},
		},
		"6232f0c5-57fa-409a-b026-0919f60e24a6": {
			ClusterID:       "k8s-0",
			ResourceManager: "k8s",
			UUID:            "6232f0c5-57fa-409a-b026-0919f60e24a6",
			Name:            "pod22",
			Project:         "ns2",
			Group:           "",
			User:            "kusr2",
			CreatedAt:       "2025-07-07T11:26:56+0200",
			StartedAt:       "2025-07-07T11:26:58+0200",
			EndedAt:         "2025-07-07T12:10:58+0200",
			CreatedAtTS:     1751880416000,
			StartedAtTS:     1751880418000,
			EndedAtTS:       1751883058000,
			Elapsed:         "00:44:02",
			State:           "Succeeded",
			Allocation: models.Generic{
				"mem": 3.221225472e+09, "nvidia.com/mig-1g.5gb": 1.0, "nvidia.com/mig-4g.20gb": 2.0, "vcpus": 3.0,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 2.119566360576e+12, "alloc_cputime": 1974, "alloc_gpumemtime": 1974, "alloc_gputime": 1974, "walltime": 658,
			},
			Tags: models.Generic{
				"annotations": map[string]string{"ceems.io/created-by": "kusr2"}, "qos": "Burstable",
			},
		},
	}
	expectedUsers = []models.User{
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "rb3", Projects: models.List{[]string{"ns3"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "rb1", Projects: models.List{[]string{"ns1", "ns2"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "rb2", Projects: models.List{[]string{"ns2"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "kusr1", Projects: models.List{[]string{"ns1"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "kusr2", Projects: models.List{[]string{"ns2"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "kusr3", Projects: models.List{[]string{"ns3"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "file1", Projects: models.List{[]string{"ns1"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "file2", Projects: models.List{[]string{"ns1"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "file3", Projects: models.List{[]string{"ns3"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "ns2:" + base.UnknownUser, Projects: models.List{[]string{"ns2"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
	}
	expectedProjects = []models.Project{
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "ns3", Users: models.List{[]string{"file3", "kusr3", "rb3"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "ns1", Users: models.List{[]string{"file1", "file2", "kusr1", "rb1"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
		{ClusterID: "k8s-0", ResourceManager: "k8s", Name: "ns2", Users: models.List{[]string{"kusr2", "ns2:" + base.UnknownUser, "rb1", "rb2"}}, LastUpdatedAt: "2025-07-07T12:15:00+0200"},
	}
)

func mockK8sAPIServer() *httptest.Server {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "pods") {
			if data, err := os.ReadFile("../../../collector/testdata/k8s/pods-metadata.json"); err == nil {
				w.Header().Add("Content-Type", "application/json")
				w.Header().Add("Content-Type", "application/vnd.kubernetes.protobuf")
				w.Write(data)

				return
			}
		} else if strings.HasSuffix(r.URL.Path, "rolebindings") {
			if data, err := os.ReadFile("../../../collector/testdata/k8s/rolebindings.json"); err == nil {
				w.Header().Add("Content-Type", "application/json")
				w.Header().Add("Content-Type", "application/vnd.kubernetes.protobuf")
				w.Write(data)

				return
			}
		} else {
			w.Write([]byte("KO"))
		}
	}))

	return server
}

func mockConfig(tmpDir, apiURL string) (yaml.Node, string, error) {
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
	kubeconfig := filepath.Join(tmpDir, "kubeconfig")

	content = fmt.Sprintf(content, apiURL)

	err := os.WriteFile(kubeconfig, []byte(content), 0o700) //nolint:gosec
	if err != nil {
		return yaml.Node{}, "", err
	}

	mainConfig := `
---
ceems_api_server:
  data:
    update_interval: 15m 
`
	mainConfigFile := filepath.Join(tmpDir, "config.yaml")

	err = os.WriteFile(mainConfigFile, []byte(mainConfig), 0o700) //nolint:gosec
	if err != nil {
		return yaml.Node{}, "", err
	}

	usersDB := `
---
users:
  ns1:
    - file1
    - file2
  ns3:
    - file3 
`
	usersDBFile := filepath.Join(tmpDir, "users_db.yaml")

	err = os.WriteFile(usersDBFile, []byte(usersDB), 0o700) //nolint:gosec
	if err != nil {
		return yaml.Node{}, "", err
	}

	config := `
---
kubeconfig_file: %s
ns_users_list_file: %s
gpu_resource_names:
  - nvidia.com/gpu
  - nvidia.com/mig-4g.20gb
  - nvidia.com/mig-1g.5gb
project_annotations:
  - ceems.io/project`

	cfg := fmt.Sprintf(config, kubeconfig, usersDBFile)

	var extraConfig yaml.Node

	if err := yaml.Unmarshal([]byte(cfg), &extraConfig); err == nil {
		return extraConfig, mainConfigFile, nil
	} else {
		return yaml.Node{}, mainConfigFile, err
	}
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()

	// Start mock k8s API server
	server := mockK8sAPIServer()
	defer server.Close()

	// Create mock config
	cfg, mainConfigFile, err := mockConfig(tmpDir, server.URL)
	require.NoError(t, err)

	// mock config
	cluster := models.Cluster{
		ID:      "k8s-0",
		Manager: "k8s",
		Extra:   cfg,
	}

	base.ConfigFilePath = mainConfigFile

	_, err = New(cluster, noOpLogger)
	require.NoError(t, err)
}

func TestFetches(t *testing.T) {
	tmpDir := t.TempDir()

	// Start mock k8s API server
	server := mockK8sAPIServer()
	defer server.Close()

	// Create mock config
	cfg, mainConfigFile, err := mockConfig(tmpDir, server.URL)
	require.NoError(t, err)

	// mock config
	cluster := models.Cluster{
		ID:      "k8s-0",
		Manager: "k8s",
		Extra:   cfg,
	}

	base.ConfigFilePath = mainConfigFile

	c, err := New(cluster, noOpLogger)
	require.NoError(t, err)

	clusterUnits, err := c.FetchUnits(t.Context(), start, end)
	require.NoError(t, err)

	gotUnits := make(map[string]models.Unit)
	for _, unit := range clusterUnits[0].Units {
		gotUnits[unit.UUID] = unit
	}

	assert.Equal(t, expectedUnits, gotUnits)

	clusterUsers, clusterProjects, err := c.FetchUsersProjects(t.Context(), current)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedUsers, clusterUsers[0].Users)
	assert.ElementsMatch(t, expectedProjects, clusterProjects[0].Projects)
}
