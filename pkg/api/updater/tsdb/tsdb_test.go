package tsdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func mockTSDBServer() *httptest.Server {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "vector",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "1",
					},
					"value": []interface{}{
						12345, "1.1",
					},
				},
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "2",
					},
					"value": []interface{}{
						12345, "2.2",
					},
				},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))

	return server
}

func mockInstanceConfig(url string) (updater.Instance, error) {
	config := `
---
cutoff_duration: 2m
queries:
    avg_cpu_usage: 
      usage: foo
    avg_cpu_mem_usage:
      usage: foo
    total_cpu_energy_usage_kwh:
      usage: foo
    total_cpu_emissions_gms:
      usage: foo
    avg_gpu_usage:
      usage: foo
    avg_gpu_mem_usage:
      usage: foo
    total_gpu_energy_usage_kwh:
      usage: foo
    total_gpu_emissions_gms:
      usage: foo
    total_io_write_stats:
      bytes: foo
      requests: bar
    total_io_read_stats:
      bytes: foo
      requests: bar
    total_ingress_stats:
      bytes: foo
      packets: bar
      drops: foo
      errors: bar
    total_outgress_stats:
      bytes: foo
      packets: bar
      drops: foo
      errors: bar`

	var extraConfig yaml.Node

	if err := yaml.Unmarshal([]byte(config), &extraConfig); err != nil {
		return updater.Instance{}, fmt.Errorf("failed to unmarshall config: %w\n", err)
	}

	// Make mock config
	return updater.Instance{
		ID:      "default",
		Updater: "tsdb",
		Web: models.WebConfig{
			URL: url,
		},
		Extra: extraConfig,
	}, nil
}

func TestTSDBUpdateSuccessSingleInstance(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock instance config
	instance, err := mockInstanceConfig(server.URL)
	require.NoError(t, err)

	// Current time
	currTime := time.Now()

	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID:       "default",
				Updaters: []string{"default"},
			},
			Units: []models.Unit{
				{
					UUID:        "1",
					StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(3000),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
				{
					UUID:        "2",
					StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(3000),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
				{
					UUID:        "3",
					StartedAtTS: currTime.Add(-30 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(30),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
			},
		},
	}
	expectedUnits := []models.Unit{
		{
			UUID:        "1",
			StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(3000),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			AveCPUUsage:         models.MetricMap{"usage": models.JSONFloat(1.1)},
			AveCPUMemUsage:      models.MetricMap{"usage": models.JSONFloat(1.1)},
			TotalCPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(1.1)},
			TotalCPUEmissions:   models.MetricMap{"usage": models.JSONFloat(1.1)},
			AveGPUUsage:         models.MetricMap{"usage": models.JSONFloat(1.1)},
			AveGPUMemUsage:      models.MetricMap{"usage": models.JSONFloat(1.1)},
			TotalGPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(1.1)},
			TotalGPUEmissions:   models.MetricMap{"usage": models.JSONFloat(1.1)},
			TotalIOWriteStats:   models.MetricMap{"bytes": models.JSONFloat(1.1), "requests": models.JSONFloat(1.1)},
			TotalIOReadStats:    models.MetricMap{"bytes": models.JSONFloat(1.1), "requests": models.JSONFloat(1.1)},
			TotalIngressStats: models.MetricMap{
				"bytes":   models.JSONFloat(1.1),
				"packets": models.JSONFloat(1.1),
				"drops":   models.JSONFloat(1.1),
				"errors":  models.JSONFloat(1.1),
			},
			TotalOutgressStats: models.MetricMap{
				"bytes":   models.JSONFloat(1.1),
				"packets": models.JSONFloat(1.1),
				"drops":   models.JSONFloat(1.1),
				"errors":  models.JSONFloat(1.1),
			},
		},
		{
			UUID:        "2",
			StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(3000),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			AveCPUUsage:         models.MetricMap{"usage": models.JSONFloat(2.2)},
			AveCPUMemUsage:      models.MetricMap{"usage": models.JSONFloat(2.2)},
			TotalCPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(2.2)},
			TotalCPUEmissions:   models.MetricMap{"usage": models.JSONFloat(2.2)},
			AveGPUUsage:         models.MetricMap{"usage": models.JSONFloat(2.2)},
			AveGPUMemUsage:      models.MetricMap{"usage": models.JSONFloat(2.2)},
			TotalGPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(2.2)},
			TotalGPUEmissions:   models.MetricMap{"usage": models.JSONFloat(2.2)},
			TotalIOWriteStats:   models.MetricMap{"bytes": models.JSONFloat(2.2), "requests": models.JSONFloat(2.2)},
			TotalIOReadStats:    models.MetricMap{"bytes": models.JSONFloat(2.2), "requests": models.JSONFloat(2.2)},
			TotalIngressStats: models.MetricMap{
				"bytes":   models.JSONFloat(2.2),
				"packets": models.JSONFloat(2.2),
				"drops":   models.JSONFloat(2.2),
				"errors":  models.JSONFloat(2.2),
			},
			TotalOutgressStats: models.MetricMap{
				"bytes":   models.JSONFloat(2.2),
				"packets": models.JSONFloat(2.2),
				"drops":   models.JSONFloat(2.2),
				"errors":  models.JSONFloat(2.2),
			},
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-30 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(30),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			Ignore:              1,
			AveCPUUsage:         models.MetricMap{},
			AveCPUMemUsage:      models.MetricMap{},
			TotalCPUEnergyUsage: models.MetricMap{},
			TotalCPUEmissions:   models.MetricMap{},
			AveGPUUsage:         models.MetricMap{},
			AveGPUMemUsage:      models.MetricMap{},
			TotalGPUEnergyUsage: models.MetricMap{},
			TotalGPUEmissions:   models.MetricMap{},
			TotalIOWriteStats:   models.MetricMap{},
			TotalIOReadStats:    models.MetricMap{},
			TotalIngressStats:   models.MetricMap{},
			TotalOutgressStats:  models.MetricMap{},
		},
	}

	tsdb, err := New(instance, log.NewNopLogger())
	require.NoError(t, err)

	updatedUnits := tsdb.Update(context.Background(), time.Now().Add(-5*time.Minute), time.Now(), units)
	for i := range len(expectedUnits) {
		assert.Equal(t, expectedUnits[i], updatedUnits[0].Units[i], fmt.Sprintf("Unit: %d", i))
	}
}

func TestTSDBUpdateFailMaxDuration(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock instance config
	instance, err := mockInstanceConfig(server.URL)
	require.NoError(t, err)

	// Current time
	currTime := time.Now()
	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID:       "default",
				Updaters: []string{"default"},
			},
			Units: []models.Unit{
				{
					UUID:        "1",
					StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(3000),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
				{
					UUID:        "2",
					StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(3000),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
				{
					UUID:        "3",
					StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
					EndedAtTS:   currTime.UnixMilli(),
					TotalTime: models.MetricMap{
						"walltime":         models.JSONFloat(3),
						"alloc_cputime":    models.JSONFloat(0),
						"alloc_cpumemtime": models.JSONFloat(0),
						"alloc_gputime":    models.JSONFloat(0),
						"alloc_gpumemtime": models.JSONFloat(0),
					},
				},
			},
		},
	}
	expectedUnits := []models.Unit{
		{
			UUID:        "1",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(3000),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			Ignore: 1,
		},
		{
			UUID:        "2",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(3000),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			Ignore: 1,
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(3),
				"alloc_cputime":    models.JSONFloat(0),
				"alloc_cpumemtime": models.JSONFloat(0),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			Ignore: 1,
		},
	}

	tsdb, err := New(instance, log.NewNopLogger())
	require.NoError(t, err)

	updatedUnits := tsdb.Update(context.Background(), time.Now().Add(-1*time.Minute), time.Now(), units)
	assert.Equal(t, expectedUnits, updatedUnits[0].Units)
}

func TestTSDBUpdateFailNoUnits(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock instance config
	instance, err := mockInstanceConfig(server.URL)
	require.NoError(t, err)

	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID:       "default",
				Updaters: []string{"default"},
			},
		},
	}

	tsdb, err := New(instance, log.NewNopLogger())
	require.NoError(t, err)

	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(context.Background(), time.Now().Add(-5*time.Minute), time.Now(), units)
	assert.Empty(t, updatedUnits[0].Units)
}

func TestTSDBUpdateFailNoTSDB(t *testing.T) {
	// Start test server
	server := mockTSDBServer()

	// Make mock instance config
	instance, err := mockInstanceConfig(server.URL)
	require.NoError(t, err)

	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID:       "default",
				Updaters: []string{"default"},
			},
			Units: []models.Unit{
				{UUID: "1", EndedAtTS: int64(10000), TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(3000),
					"alloc_cputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(0),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_gpumemtime": models.JSONFloat(0),
				}},
				{UUID: "2", EndedAtTS: int64(10000), TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(3000),
					"alloc_cputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(0),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_gpumemtime": models.JSONFloat(0),
				}},
				{UUID: "3", EndedAtTS: int64(10000), TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(30),
					"alloc_cputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(0),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_gpumemtime": models.JSONFloat(0),
				}},
			},
		},
	}

	expectedUnits := units

	tsdb, err := New(instance, log.NewNopLogger())
	require.NoError(t, err)

	// Stop TSDB server
	server.Close()

	updatedUnits := tsdb.Update(context.Background(), time.Now().Add(-5*time.Minute), time.Now(), units)
	assert.Equal(t, expectedUnits, updatedUnits)
}
