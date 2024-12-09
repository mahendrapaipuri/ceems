//go:build !noredfish
// +build !noredfish

package collector

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRedfishServer(checkHeader bool) *httptest.Server {
	// Test redfish server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkHeader {
			if len(r.Header[http.CanonicalHeaderKey(realIPHeaderName)]) == 0 {
				w.WriteHeader(http.StatusBadRequest)

				return
			}
		}

		if r.URL.Path == "/redfish/v1/" {
			if data, err := os.ReadFile("testdata/redfish/service_root.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis" {
			if data, err := os.ReadFile("testdata/redfish/chassis_collection.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-1" {
			if data, err := os.ReadFile("testdata/redfish/chassis.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-1/Power" {
			if data, err := os.ReadFile("testdata/redfish/chassis_power.json"); err == nil {
				w.Write(data)

				return
			}
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))

	return server
}

func TestNewRedfishCollector(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a dummy Redfish server
	server := testRedfishServer(true)
	defer server.Close()

	// Make config file
	configFileTmpl := `
---
redfish_web_config:
  url: %s
  use_session_token: true`

	configPath := filepath.Join(tmpDir, "config.yml")
	configFile := fmt.Sprintf(configFileTmpl, server.URL)
	os.WriteFile(configPath, []byte(configFile), 0o600)

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.redfish.web-config", configPath,
			"--collector.redfish.send-real-ip-header",
			"--collector.redfish.local-ip-address", "127.0.0.1",
		},
	)
	require.NoError(t, err)

	collector, err := NewRedfishCollector(slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestPowerReadings(t *testing.T) {
	// Start a dummy Redfish server
	server := testRedfishServer(false)

	config := gofish.ClientConfig{
		Endpoint: server.URL,
	}
	redfishClient, err := gofish.Connect(config)
	require.NoError(t, err)

	// Get all available chassis
	chassis, err := redfishClient.Service.Chassis()
	require.NoError(t, err)

	collector := &redfishCollector{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		chassis:     chassis,
		client:      redfishClient,
		cachedPower: make(map[string]*redfish.Power, len(chassis)),
	}

	// Expected power readings
	expected := map[string]map[string]float64{
		"avg":     {"Chassis_1": 365},
		"current": {"Chassis_1": 397},
		"max":     {"Chassis_1": 609},
		"min":     {"Chassis_1": 326},
	}

	// Get power readings
	got := collector.powerReadings()
	assert.EqualValues(t, expected, got)

	// Stop test redfish server
	server.Close()

	// Make request again and we should get value from cached value
	got = collector.powerReadings()
	assert.EqualValues(t, expected, got)
}
