package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
)

const mockCEEMSLBAppName = "mockApp"

var mockCEEMSLBApp = *kingpin.New(
	mockCEEMSLBAppName,
	"Mock Load Balancer App.",
)

func queryLB(address string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
	if err != nil {
		return err
	}

	req.Header.Add("X-Grafana-User", "usr1")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func makeConfigFile(configFile string, tmpDir string) string {
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0600)
	return configPath
}

func TestCEEMSLBMainSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dummy-response"))
	}))
	defer server.Close()

	// Make config file
	configFileTmpl := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: "default"
      tsdb_urls:
        - %s`

	configFile := fmt.Sprintf(configFileTmpl, server.URL)
	configFilePath := makeConfigFile(configFile, tmpDir)

	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]}, "--log.level", "debug", fmt.Sprintf("--config.file=%s", configFilePath))
	a := CEEMSLoadBalancer{
		appName: mockCEEMSLBAppName,
		App:     mockCEEMSLBApp,
	}

	// Start Main
	go func() {
		a.Main()
	}()

	// Query LB
	for i := 0; i < 10; i++ {
		if err := queryLB("localhost:9030/default"); err == nil {
			break
		} else {
			fmt.Println(err)
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start load balancer after %d attempts", i)
		}
	}
}

func TestCEEMSLBMainFail(t *testing.T) {
	// Remove test related args and add a dummy arg
	os.Args = []string{os.Args[0]}
	a, err := NewCEEMSLoadBalancer()
	if err != nil {
		t.Fatal(err)
	}

	// Run Main
	if err := a.Main(); err == nil {
		t.Errorf("expected error: %s", err)
	}
}

func TestCEEMSLBMainFailMismatchIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: "default"
      tsdb_urls:
        - http://localhost:9090

clusters:
  - id: "default-1"`

	configFilePath := makeConfigFile(configFile, tmpDir)

	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]}, "--log.level", "debug", fmt.Sprintf("--config.file=%s", configFilePath))
	a := CEEMSLoadBalancer{
		appName: mockCEEMSLBAppName,
		App:     mockCEEMSLBApp,
	}

	// Run Main
	if err := a.Main(); err == nil {
		t.Errorf("expected error: %s", err)
	}
}
