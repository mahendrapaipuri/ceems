package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryServer(address, port string) error {
	client := &http.Client{}
	req, _ := http.NewRequest( //nolint:noctx
		http.MethodGet,
		fmt.Sprintf("http://%s/redfish/v1/", address),
		nil,
	)
	req.Header.Set(redfishURLHeaderName, "http://localhost:"+port)

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

func TestConfigValidation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		err     bool
	}{
		{
			name: "valid config with web section",
			content: `
---
redfish_proxy:
  web:
    insecure_skip_verify: true`,
		},
		{
			name: "valid config with web and targets section",
			content: `
---
redfish_proxy:
  web:
    insecure_skip_verify: true
  targets:
    - host_ip_addrs:
        - 192.168.1.1
        - 192.168.1.2
      url: http://172.134.1.1:80`,
		},
		{
			name: "valid deprecated config with web and targets section",
			content: `
---
redfish_config:
  web:
    insecure_skip_verify: true
  targets:
    - host_ip_addrs:
        - 192.168.1.1
        - 192.168.1.2
      url: http://172.134.1.1:80`,
		},
		{
			name: "invalid config due to malformed web url",
			err:  true,
			content: `
---
redfish_proxy:
  web:
    insecure_skip_verify: true
  targets:
    - host_ip_addrs:
        - 192.168.1.1
        - 192.168.1.2
      url: http:172.134.1.1:80`,
		},
	}

	for i, test := range tests {
		configFilePath := filepath.Join(tmpDir, fmt.Sprintf("config-%d.yml", i))
		os.WriteFile(configFilePath, []byte(test.content), 0o600)

		// Read config file
		cfg, err := common.MakeConfig[Redfish](configFilePath)
		if test.err {
			require.Error(t, err, test.name)
		} else {
			require.NoError(t, err, test.name)

			if len(cfg.Config.Targets) > 0 {
				assert.Equal(t, "http://172.134.1.1:80", cfg.Config.Targets[0].URL.String())
			}
		}
	}
}

func TestRedfishProxyServerMain(t *testing.T) {
	// Start test targets
	targets, _ := testTargets()

	// Target URLs
	var targetURLs []*url.URL

	for _, t := range targets {
		u, _ := url.Parse(t.URL)
		targetURLs = append(targetURLs, u)

		defer t.Close()
	}

	// Port for 1st server
	port := targetURLs[0].Port()

	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
redfish_config:
  web:
    insecure_skip_verify: true`

	configFilePath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configFilePath, []byte(configFile), 0o600)

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, "--config.file="+configFilePath)
	os.Args = append(os.Args, "--log.level=debug")

	// Start Main
	go func() {
		main()
	}()

	// Query exporter
	for i := range 10 {
		if err := queryServer("localhost:5000", port); err == nil {
			break
		}

		time.Sleep(500 * time.Millisecond)

		if i == 9 {
			require.Errorf(t, fmt.Errorf("Could not start stats server after %d attempts", i), "failed to start server")
		}
	}

	// Send INT signal and wait a second to clean up server and DB
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	time.Sleep(1 * time.Second)
}
