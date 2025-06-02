//go:build !noredfish
// +build !noredfish

package collector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRedfishServer() *httptest.Server {
	validTokens := []string{"token1", "token2", "token3"}

	var tokenCounter int

	hasToken := func(r *http.Request) bool {
		return slices.Contains(validTokens, r.Header.Get("X-Auth-Token"))
	}

	remove := func(s []string, e string) []string {
		var idx int

		for i, ele := range s {
			if ele == e {
				idx = i

				break
			}
		}

		s[idx] = s[len(s)-1]

		return s[:len(s)-1]
	}

	// Test redfish server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			if data, err := os.ReadFile("testdata/redfish/service_root.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Sessions" {
			// Write response headers with X-Auth-Token set
			w.Header().Add("X-Auth-Token", validTokens[tokenCounter])
			w.Header().Add("Location", fmt.Sprintf("/redfish/v1/SessionService/Sessions/%s/", validTokens[tokenCounter]))
			w.WriteHeader(http.StatusCreated)

			tokenCounter++

			// Reset counter when exceeded validTokens len
			if tokenCounter > len(validTokens) {
				tokenCounter = 0
			}
		} else if r.URL.Path == "/redfish/v1/Chassis" {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			if data, err := os.ReadFile("testdata/redfish/chassis_collection.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-1" {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			if data, err := os.ReadFile("testdata/redfish/chassis_1.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-1/Power" {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			if data, err := os.ReadFile("testdata/redfish/chassis_1_power.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-2" {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			if data, err := os.ReadFile("testdata/redfish/chassis_2.json"); err == nil {
				w.Write(data)

				return
			}
		} else if r.URL.Path == "/redfish/v1/Chassis/Chassis-2/Power" {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			if data, err := os.ReadFile("testdata/redfish/chassis_2_power.json"); err == nil {
				w.Write(data)

				return
			}
		} else if strings.HasPrefix(r.URL.Path, "/redfish/v1/SessionService/Sessions") {
			if !hasToken(r) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			// Get token from session ID
			token := strings.Split(r.URL.Path, "/")[5]

			// Remove this token from valid tokens
			validTokens = remove(validTokens, token)

			// Reset counter when exceeded validTokens len
			if tokenCounter > len(validTokens) {
				tokenCounter = 0
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))

	return server
}

func TestNewRedfishCollector(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a dummy Redfish server
	server := testRedfishServer()
	defer server.Close()

	// Get address and port from server.URL
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	for _, cfgName := range []string{"redfish_web_config", "redfish_web"} {
		// Make config file
		configFileTmpl := `
---
%s:
  protocol: http
  hostname: %s
  port: %s
  username: admin
  password: secret
  use_session_token: true`

		configPath := filepath.Join(tmpDir, "config.yml")
		configFile := fmt.Sprintf(configFileTmpl, cfgName, serverURL.Hostname(), serverURL.Port())
		os.WriteFile(configPath, []byte(configFile), 0o600)

		_, err = CEEMSExporterApp.Parse(
			[]string{
				"--collector.redfish.web-config", configPath,
			},
		)
		require.NoError(t, err)

		collector, err := NewRedfishCollector(noOpLogger)
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

		err = collector.Stop(t.Context())
		require.NoError(t, err)
	}
}

func TestNewRedfishCollectorWithExternalURL(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a dummy Redfish server
	server := testRedfishServer()
	defer server.Close()

	// Make config file
	configFileTmpl := `
---
redfish_web_config:
  protocol: http
  hostname: bmc-0
  port: 5000
  username: admin
  password: secret
  external_url: %s
  use_session_token: true`

	configPath := filepath.Join(tmpDir, "config.yml")
	configFile := fmt.Sprintf(configFileTmpl, server.URL)
	os.WriteFile(configPath, []byte(configFile), 0o600)

	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.redfish.web-config", configPath,
		},
	)
	require.NoError(t, err)

	collector, err := NewRedfishCollector(noOpLogger)
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

	err = collector.Stop(t.Context())
	require.NoError(t, err)
}

func TestPowerReadings(t *testing.T) {
	// Start a dummy Redfish server
	server := testRedfishServer()

	config := gofish.ClientConfig{
		Endpoint: server.URL,
		Username: "admin",
		Password: "secret",
	}
	redfishClient, err := gofish.Connect(config)
	require.NoError(t, err)

	// Get all available chassis
	chassis, err := redfishClient.Service.Chassis()
	require.NoError(t, err)

	collector := &redfishCollector{
		logger:      noOpLogger,
		config:      &config,
		chassis:     chassis,
		client:      redfishClient,
		cachedPower: make(map[string]*redfish.Power, len(chassis)),
	}

	// Expected power readings
	expected := map[string]map[string]float64{
		"avg":     {"Chassis_1": 365, "Chassis_2": 1734},
		"current": {"Chassis_1": 397, "Chassis_2": 1696},
		"max":     {"Chassis_1": 609, "Chassis_2": 2155},
		"min":     {"Chassis_1": 326, "Chassis_2": 588},
	}

	// Get power readings
	got := collector.powerReadings()
	assert.Equal(t, expected, got)

	// Simulate token invalidation
	collector.client.Logout()

	// Now make a new request which should give us error
	for _, chass := range collector.chassis {
		_, err = chass.Power()
		require.Error(t, err)
	}

	// Set cachedPower to nil so we get nil response in next request
	collector.cachedPower = make(map[string]*redfish.Power)

	// Get power readings which should be zero
	zeroExpected := map[string]map[string]float64{
		"current": make(map[string]float64),
		"min":     make(map[string]float64),
		"max":     make(map[string]float64),
		"avg":     make(map[string]float64),
	}
	got = collector.powerReadings()
	assert.Equal(t, zeroExpected, got)

	// The next request should give us correct values as client
	// will be reinitialised in the last request
	got = collector.powerReadings()
	assert.Equal(t, expected, got)

	// Stop test redfish server
	server.Close()

	// Make request again and we should get value from cached value
	got = collector.powerReadings()
	assert.Equal(t, expected, got)
}
