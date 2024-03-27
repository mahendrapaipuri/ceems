package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
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

func mockConfig(tmpDir string, serverURL string) string {
	configFileTmpl := `
---
web_url: %s
cutoff_duration: %s
queries:
  avg_cpu_usage: foo
  avg_cpu_mem_usage: foo
  total_cpu_energy_usage_kwh: foo
  total_cpu_emissions_gms: foo
  avg_gpu_usage: foo
  avg_gpu_mem_usage: foo
  total_gpu_energy_usage_kwh: foo
  total_gpu_emissions_gms: foo
  total_io_write_hot_gb: foo
  total_io_read_hot_gb: foo
  total_io_write_cold_gb: foo
  total_io_read_cold_gb: foo
  total_ingress_in_gb: foo
  total_outgress_in_gb: foo`

	configFile := fmt.Sprintf(configFileTmpl, serverURL, "2m")
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0644)
	return configPath
}

func TestTSDBUpdateSuccess(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock config
	configPath := mockConfig(t.TempDir(), server.URL)
	if _, err := base.CEEMSServerApp.Parse([]string{"--tsdb.config.file", configPath}); err != nil {
		t.Fatal(err)
	}

	// Current time
	currTime := time.Now()

	units := []models.Unit{
		{
			UUID:        "1",
			StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(3000),
		},
		{
			UUID:        "2",
			StartedAtTS: currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(3000),
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-30 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(30),
		},
	}
	expectedUnits := []models.Unit{
		{
			UUID:                "1",
			StartedAtTS:         currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:           currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         1.1,
			AveCPUMemUsage:      1.1,
			TotalCPUEnergyUsage: 1.1,
			TotalCPUEmissions:   1.1,
			AveGPUUsage:         1.1,
			AveGPUMemUsage:      1.1,
			TotalGPUEnergyUsage: 1.1,
			TotalGPUEmissions:   1.1,
			TotalIOWriteHot:     1.1,
			TotalIOWriteCold:    1.1,
			TotalIOReadHot:      1.1,
			TotalIOReadCold:     1.1,
			TotalIngress:        1.1,
			TotalOutgress:       1.1,
		},
		{
			UUID:                "2",
			StartedAtTS:         currTime.Add(-3000 * time.Second).UnixMilli(),
			EndedAtTS:           currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         2.2,
			AveCPUMemUsage:      2.2,
			TotalCPUEnergyUsage: 2.2,
			TotalCPUEmissions:   2.2,
			AveGPUUsage:         2.2,
			AveGPUMemUsage:      2.2,
			TotalGPUEnergyUsage: 2.2,
			TotalGPUEmissions:   2.2,
			TotalIOWriteHot:     2.2,
			TotalIOWriteCold:    2.2,
			TotalIOReadHot:      2.2,
			TotalIOReadCold:     2.2,
			TotalIngress:        2.2,
			TotalOutgress:       2.2,
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-30 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(30),
			Ignore:      1,
		},
	}

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance: %s", err)
	}

	updatedUnits := tsdb.Update(time.Now().Add(-5*time.Minute), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailMaxDuration(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock config
	configPath := mockConfig(t.TempDir(), server.URL)
	if _, err := base.CEEMSServerApp.Parse([]string{"--tsdb.config.file", configPath}); err != nil {
		t.Fatal(err)
	}

	// Current time
	currTime := time.Now()
	units := []models.Unit{
		{
			UUID:        "1",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(3000),
		},
		{
			UUID:        "2",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(3000),
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(30),
		},
	}
	expectedUnits := []models.Unit{
		{
			UUID:                "1",
			StartedAtTS:         currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:           currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         1.1,
			AveCPUMemUsage:      1.1,
			TotalCPUEnergyUsage: 1.1,
			TotalCPUEmissions:   1.1,
			AveGPUUsage:         1.1,
			AveGPUMemUsage:      1.1,
			TotalGPUEnergyUsage: 1.1,
			TotalGPUEmissions:   1.1,
			TotalIOWriteHot:     1.1,
			TotalIOWriteCold:    1.1,
			TotalIOReadHot:      1.1,
			TotalIOReadCold:     1.1,
			TotalIngress:        1.1,
			TotalOutgress:       1.1,
		},
		{
			UUID:                "2",
			StartedAtTS:         currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:           currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         2.2,
			AveCPUMemUsage:      2.2,
			TotalCPUEnergyUsage: 2.2,
			TotalCPUEmissions:   2.2,
			AveGPUUsage:         2.2,
			AveGPUMemUsage:      2.2,
			TotalGPUEnergyUsage: 2.2,
			TotalGPUEmissions:   2.2,
			TotalIOWriteHot:     2.2,
			TotalIOWriteCold:    2.2,
			TotalIOReadHot:      2.2,
			TotalIOReadCold:     2.2,
			TotalIngress:        2.2,
			TotalOutgress:       2.2,
		},
		{
			UUID:        "3",
			StartedAtTS: currTime.Add(-3 * time.Second).UnixMilli(),
			EndedAtTS:   currTime.UnixMilli(),
			ElapsedRaw:  int64(30),
			Ignore:      1,
		},
	}

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(time.Now().Add(-5*time.Minute), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailNoUnits(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	// Make mock config
	configPath := mockConfig(t.TempDir(), server.URL)
	if _, err := base.CEEMSServerApp.Parse([]string{"--tsdb.config.file", configPath}); err != nil {
		t.Fatal(err)
	}

	units := []models.Unit{}
	expectedUnits := []models.Unit{}

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(time.Now().Add(-5*time.Minute), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailNoTSDB(t *testing.T) {
	// Start test server
	server := mockTSDBServer()

	// Make mock config
	configPath := mockConfig(t.TempDir(), server.URL)
	if _, err := base.CEEMSServerApp.Parse([]string{"--tsdb.config.file", configPath}); err != nil {
		t.Fatal(err)
	}

	units := []models.Unit{
		{UUID: "1", EndedAtTS: int64(10000), ElapsedRaw: int64(3000)},
		{UUID: "2", EndedAtTS: int64(10000), ElapsedRaw: int64(3000)},
		{UUID: "3", EndedAtTS: int64(10000), ElapsedRaw: int64(30)},
	}
	expectedUnits := units

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}
	// Stop TSDB server
	server.Close()

	updatedUnits := tsdb.Update(time.Now().Add(-5*time.Minute), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}
