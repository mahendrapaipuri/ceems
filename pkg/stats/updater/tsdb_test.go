package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/mahendrapaipuri/ceems/pkg/stats/models"
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

func TestTSDBUpdateSuccess(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	if _, err := base.CEEMSServerApp.Parse(
		[]string{
			"--tsdb.web.url", server.URL,
			"--tsdb.data.cutoff.duration", "2m",
		},
	); err != nil {
		t.Fatal(err)
	}

	// Current time
	currTime := time.Now()

	units := []models.Unit{
		{
			UUID:       "1",
			StartTS:    currTime.Add(-3000 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(3000),
		},
		{
			UUID:       "2",
			StartTS:    currTime.Add(-3000 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(3000),
		},
		{
			UUID:       "3",
			StartTS:    currTime.Add(-30 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(30),
		},
	}
	expectedUnits := []models.Unit{
		{
			UUID:                "1",
			StartTS:             currTime.Add(-3000 * time.Second).UnixMilli(),
			EndTS:               currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         1.1,
			AveCPUMemUsage:      1.1,
			TotalCPUEnergyUsage: 1.1,
			TotalCPUEmissions:   1.1,
			AveGPUUsage:         1.1,
			AveGPUMemUsage:      1.1,
			TotalGPUEnergyUsage: 1.1,
			TotalGPUEmissions:   1.1,
		},
		{
			UUID:                "2",
			StartTS:             currTime.Add(-3000 * time.Second).UnixMilli(),
			EndTS:               currTime.UnixMilli(),
			ElapsedRaw:          int64(3000),
			AveCPUUsage:         2.2,
			AveCPUMemUsage:      2.2,
			TotalCPUEnergyUsage: 2.2,
			TotalCPUEmissions:   2.2,
			AveGPUUsage:         2.2,
			AveGPUMemUsage:      2.2,
			TotalGPUEnergyUsage: 2.2,
			TotalGPUEmissions:   2.2,
		},
		{
			UUID:       "3",
			StartTS:    currTime.Add(-30 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(30),
			Ignore:     1,
		},
	}

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(time.Now(), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailMaxDuration(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	if _, err := base.CEEMSServerApp.Parse(
		[]string{
			"--tsdb.web.url", server.URL,
			"--tsdb.data.cutoff.duration", "2m",
		},
	); err != nil {
		t.Fatal(err)
	}

	// Current time
	currTime := time.Now()

	units := []models.Unit{
		{
			UUID:       "1",
			StartTS:    currTime.Add(-3 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(3000),
		},
		{
			UUID:       "2",
			StartTS:    currTime.Add(-3 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(3000),
		},
		{
			UUID:       "3",
			StartTS:    currTime.Add(-3 * time.Second).UnixMilli(),
			EndTS:      currTime.UnixMilli(),
			ElapsedRaw: int64(30),
		},
	}
	expectedUnits := units

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(time.Now(), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailNoUnits(t *testing.T) {
	// Start test server
	server := mockTSDBServer()
	defer server.Close()

	if _, err := base.CEEMSServerApp.Parse(
		[]string{
			"--tsdb.web.url", server.URL,
			"--tsdb.data.cutoff.duration", "2m",
		},
	); err != nil {
		t.Fatal(err)
	}

	units := []models.Unit{}
	expectedUnits := []models.Unit{}

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}

	updatedUnits := tsdb.Update(time.Now(), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}

func TestTSDBUpdateFailNoTSDB(t *testing.T) {
	// Start test server
	server := mockTSDBServer()

	if _, err := base.CEEMSServerApp.Parse(
		[]string{
			"--tsdb.web.url", server.URL,
			"--tsdb.data.cutoff.duration", "2m",
		},
	); err != nil {
		t.Fatal(err)
	}

	units := []models.Unit{
		{UUID: "1", EndTS: int64(10000), ElapsedRaw: int64(3000)},
		{UUID: "2", EndTS: int64(10000), ElapsedRaw: int64(3000)},
		{UUID: "3", EndTS: int64(10000), ElapsedRaw: int64(30)},
	}
	expectedUnits := units

	tsdb, err := NewTSDBUpdater(log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB updater instance")
	}
	// Stop TSDB server
	server.Close()

	updatedUnits := tsdb.Update(time.Now(), time.Now(), units)
	if !reflect.DeepEqual(updatedUnits, expectedUnits) {
		t.Errorf("expected %#v \n got %#v", expectedUnits, updatedUnits)
	}
}
