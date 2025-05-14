package openstack

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	config_util "github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	start, _   = time.Parse(base.DatetimezoneLayout, "2024-10-15T16:15:00+0200")
	end, _     = time.Parse(base.DatetimezoneLayout, "2024-10-15T16:45:00+0200")
	current, _ = time.Parse(base.DatetimezoneLayout, "2024-10-15T16:45:00+0200")

	noOpLogger = slog.New(slog.DiscardHandler)

	expectedUnits = map[string]models.Unit{
		"d0d60434-4bf1-4eb1-9469-d7b38083a88f": {
			ResourceManager: "openstack",
			UUID:            "d0d60434-4bf1-4eb1-9469-d7b38083a88f",
			Name:            "new-vgpu-3",
			Project:         "admin",
			User:            "admin",
			CreatedAt:       "2024-10-15T15:32:25+0200",
			StartedAt:       "2024-10-15T15:32:43+0200",
			EndedAt:         "2024-10-15T16:32:09+0200",
			CreatedAtTS:     1728999145000,
			StartedAtTS:     1728999163000,
			EndedAtTS:       1729002729000,
			Elapsed:         "00:59:26",
			State:           "DELETED",
			Allocation: models.Generic{
				"disk":        1,
				"extra_specs": map[string]string{"hw_rng:allowed": "True", "resources:VGPU": "1"},
				"mem":         8192,
				"name":        "m10.vgpu",
				"swap":        0,
				"vcpus":       8,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 8.429568e+06,
				"alloc_cputime":    8232,
				"alloc_gpumemtime": 1029,
				"alloc_gputime":    1029,
				"walltime":         1029,
			},
			Tags: models.Generic{
				"az": "nova",
				// "hypervisor":     "gpu-node-4",
				"power_state":    "NOSTATE",
				"reservation_id": "r-rcywwpf9",
				"metadata":       map[string]string{},
				"tags":           []string{},
				// "server_groups":  "",
			},
		},
		"0687859c-b7b8-47ea-aa4c-74162f52fbfc": {
			ResourceManager: "openstack",
			UUID:            "0687859c-b7b8-47ea-aa4c-74162f52fbfc",
			Name:            "newer-2",
			Project:         "admin",
			User:            "admin",
			CreatedAt:       "2024-10-15T16:29:18+0200",
			StartedAt:       "2024-10-15T16:29:34+0200",
			EndedAt:         "N/A",
			CreatedAtTS:     1729002558000,
			StartedAtTS:     1729002574000,
			EndedAtTS:       0,
			Elapsed:         "00:15:26",
			State:           "ACTIVE",
			Allocation: models.Generic{
				"disk":        1,
				"extra_specs": map[string]string{"hw_rng:allowed": "True"},
				"mem":         256,
				"name":        "cirros256",
				"swap":        0,
				"vcpus":       1,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 237056,
				"alloc_cputime":    926,
				"alloc_gpumemtime": 0,
				"alloc_gputime":    0,
				"walltime":         926,
			},
			Tags: models.Generic{
				"az": "nova",
				// "hypervisor":     "cpu-node-4",
				"power_state":    "RUNNING",
				"reservation_id": "r-fius3pcg",
				"metadata":       map[string]string{},
				"tags":           []string{},
				// "server_groups":  "",
			},
		},
		"66c3eff0-52eb-45e2-a5da-5fe21c0ef3f3": {
			ResourceManager: "openstack",
			UUID:            "66c3eff0-52eb-45e2-a5da-5fe21c0ef3f3",
			Name:            "tp-21",
			Project:         "test-project-2",
			User:            "test-user-2",
			CreatedAt:       "2024-10-15T15:16:44+0200",
			StartedAt:       "2024-10-15T15:16:55+0200",
			EndedAt:         "N/A",
			CreatedAtTS:     1728998204000,
			StartedAtTS:     1728998215000,
			EndedAtTS:       0,
			Elapsed:         "01:28:05",
			State:           "ACTIVE",
			Allocation: models.Generic{
				"disk":        1,
				"extra_specs": map[string]string{"hw_rng:allowed": "True"},
				"mem":         192000,
				"name":        "m1.xl",
				"swap":        0,
				"vcpus":       128,
			},
			TotalTime: models.MetricMap{
				"alloc_cpumemtime": 3.456e+08,
				"alloc_cputime":    230400,
				"alloc_gpumemtime": 0,
				"alloc_gputime":    0,
				"walltime":         1800,
			},
			Tags: models.Generic{
				"az": "nova",
				// "hypervisor":     "cpu-big-node-4",
				"power_state":    "RUNNING",
				"reservation_id": "r-9ak0uvk9",
				"metadata":       map[string]string{},
				"tags":           []string{},
				// "server_groups":  "",
			},
		},
	}
	expectedUsers = []models.User{
		{UID: "adbc53ea724f4e2bb954e27725b6cf5b", Name: "admin", Projects: models.List{"admin", "demo"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "03b060551ecc488b8756c9f27258d71e", Name: "test-user-1", Projects: models.List{"test-project-1", "test-project-2", "test-project-3"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "5fd1986befa042a4b866944f5adbefeb", Name: "test-user-2", Projects: models.List{"test-project-2", "test-project-3"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "4223638a14e44980bf8557cd3ba14e76", Name: "test-user-3", Projects: models.List{"test-project-3"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "dc87e591c0d247d5ac04e873bd8a1646", Name: "test-user-4", Projects: models.List{"test-project-4"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
	}
	expectedProjects = []models.Project{
		{UID: "066a633fd999424faa3409ab60221fbf", Name: "admin", Users: models.List{"admin"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "706f9e5f3e174feebcce4e7f08a7b7e3", Name: "test-project-2", Users: models.List{"test-user-1", "test-user-2"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "9d87d46f8af54da2adc3e7b94c9d3c30", Name: "demo", Users: models.List{"admin"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "b964a9e51c0046a4a84d3f83a135a97c", Name: "test-project-4", Users: models.List{"test-user-4"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "bdb137e6ee6d427a899ac22de5d76b8c", Name: "test-project-3", Users: models.List{"test-user-1", "test-user-2", "test-user-3"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
		{UID: "cca105ea0cff426e96f096887b7f4b82", Name: "test-project-1", Users: models.List{"test-user-1"}, LastUpdatedAt: "2024-10-15T15:15:00+0200"},
	}
)

func mockErrorServer() *httptest.Server {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("KO"))
	}))

	return server
}

func mockOSComputeAPIServer() *httptest.Server {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "servers") {
			if tokens := r.Header[tokenHeaderName]; len(tokens) == 0 {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			var fileName string
			if _, ok := r.URL.Query()["deleted"]; ok {
				fileName = "deleted"
			} else {
				fileName = "servers"
			}

			if data, err := os.ReadFile(fmt.Sprintf("../../testdata/openstack/compute/%s.json", fileName)); err == nil {
				w.Write(data)

				return
			}
		} else if strings.Contains(r.URL.Path, "flavors") {
			if data, err := os.ReadFile("../../testdata/openstack/compute/flavors.json"); err == nil {
				w.Write(data)

				return
			}
		} else {
			w.Write([]byte("KO"))
		}
	}))

	return server
}

func mockOSIdentityAPIServer() *httptest.Server {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "users") {
			if tokens := r.Header[tokenHeaderName]; len(tokens) == 0 {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			if data, err := os.ReadFile("../../testdata/openstack/identity/users.json"); err == nil {
				w.Write(data)

				return
			}
		} else if strings.Contains(r.URL.Path, "users") {
			if tokens := r.Header[tokenHeaderName]; len(tokens) == 0 {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			pathParts := strings.Split(r.URL.Path, "/")

			userID := pathParts[len(pathParts)-2]
			if data, err := os.ReadFile(fmt.Sprintf("../../testdata/openstack/identity/%s.json", userID)); err == nil {
				w.Write(data)

				return
			}
		} else if strings.HasSuffix(r.URL.Path, "tokens") {
			decoder := json.NewDecoder(r.Body)

			var t map[string]interface{}

			if err := decoder.Decode(&t); err != nil {
				w.Write([]byte("KO"))

				return
			}

			w.Header().Add(subjTokenHeaderName, "apitokensecret")
			w.WriteHeader(http.StatusCreated)

			return
		} else {
			w.Write([]byte("KO"))
		}
	}))

	return server
}

func mockConfig(computeAPIURL, identityAPIURL string) (yaml.Node, error) {
	config := `
---
api_service_endpoints:
  compute: %s
  identity: %s
auth:
  identity:
    methods:
      - password
    password:
      user:
        name: admin
        password: supersecret`

	cfg := fmt.Sprintf(config, computeAPIURL, identityAPIURL)

	var extraConfig yaml.Node

	if err := yaml.Unmarshal([]byte(cfg), &extraConfig); err == nil {
		return extraConfig, nil
	} else {
		return yaml.Node{}, err
	}
}

func TestOpenstackFetcher(t *testing.T) {
	// Setup mock API servers
	computeAPIServer := mockOSComputeAPIServer()
	defer computeAPIServer.Close()

	identityAPIServer := mockOSIdentityAPIServer()
	defer identityAPIServer.Close()

	extraConfig, err := mockConfig(computeAPIServer.URL, identityAPIServer.URL)
	require.NoError(t, err)

	// mock config
	clusters := []models.Cluster{
		{
			ID:      "os-0",
			Manager: "openstack",
			Extra:   extraConfig,
		},
		{
			ID:      "os-1",
			Manager: "openstack",
			Extra:   extraConfig,
		},
	}

	ctx := t.Context()

	for _, cluster := range clusters {
		os, err := New(cluster, noOpLogger)
		require.NoError(t, err)

		units, err := os.FetchUnits(ctx, start, end)
		require.NoError(t, err)
		assert.Len(t, units[0].Units, 18)

		for uuid, expectedUnit := range expectedUnits {
			for _, gotUnit := range units[0].Units {
				if uuid == gotUnit.UUID {
					assert.Equal(t, expectedUnit, gotUnit, "Unit %s", uuid)

					break
				}
			}
		}

		users, projects, err := os.FetchUsersProjects(ctx, current)
		require.NoError(t, err)

		// Use expected LastUpdatedAt
		for i := range len(users[0].Users) {
			users[0].Users[i].LastUpdatedAt = expectedUsers[0].LastUpdatedAt
		}

		for i := range len(projects[0].Projects) {
			projects[0].Projects[i].LastUpdatedAt = expectedProjects[0].LastUpdatedAt
		}

		assert.Equal(t, expectedUsers, users[0].Users)
		assert.Equal(t, expectedProjects, projects[0].Projects)
	}
}

func TestOpenstackFetcherFail(t *testing.T) {
	// Setup mock API servers
	computeAPIServer := mockOSComputeAPIServer()

	identityAPIServer := mockOSIdentityAPIServer()

	extraConfig, err := mockConfig(computeAPIServer.URL, identityAPIServer.URL)
	require.NoError(t, err)

	// mock config
	cluster := models.Cluster{
		ID:      "os-0",
		Manager: "openstack",
		Extra:   extraConfig,
	}

	ctx := t.Context()
	os, err := New(cluster, noOpLogger)
	require.NoError(t, err)

	// Stop test servers to simulate when OS services are offline
	computeAPIServer.Close()
	identityAPIServer.Close()

	_, err = os.FetchUnits(ctx, start, end)
	require.Error(t, err)

	// Here we should not get an error as it will return cached data
	// that we created during struct instantiation
	_, _, err = os.FetchUsersProjects(ctx, current)
	require.NoError(t, err)
}

func TestOpenstackServiceError(t *testing.T) {
	errorServer := mockErrorServer()
	defer errorServer.Close()

	identityAPIServer := mockOSIdentityAPIServer()
	defer identityAPIServer.Close()

	extraConfig, err := mockConfig(errorServer.URL, identityAPIServer.URL)
	require.NoError(t, err)

	// mock config
	cluster := models.Cluster{
		ID:      "os-0",
		Manager: "openstack",
		Extra:   extraConfig,
	}

	// Add header
	cluster.Web.HTTPClientConfig.HTTPHeaders = &config_util.Headers{
		Headers: make(map[string]config_util.Header),
	}
	cluster.Web.HTTPClientConfig.HTTPHeaders.Headers[novaMicroVersionHeaders[0]] = config_util.Header{
		Values: []string{"latest"},
	}

	ctx := t.Context()
	os, err := New(cluster, noOpLogger)
	require.NoError(t, err)

	// Attempt to fetch instances and we should get an error
	_, err = os.FetchUnits(ctx, time.Now(), time.Now())
	require.Error(t, err)
}
