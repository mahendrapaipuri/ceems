package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTargets() ([]*httptest.Server, []string) {
	servers := make([]*httptest.Server, 2)
	remoteIPs := []string{"192.168.1.1", "192.168.1.2"}
	// Test redfish server
	for i := range 2 {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(remoteIPs[i]))
		}))
	}

	return servers, remoteIPs
}

func TestNewRedfishProxyServerWithTargets(t *testing.T) {
	// Start test targets
	targets, remoteIPs := testTargets()

	// Target URLs
	var targetURLs []*url.URL

	for _, t := range targets {
		u, _ := url.Parse(t.URL)
		targetURLs = append(targetURLs, u)

		defer t.Close()
	}

	// Test config
	config := &Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Redfish: &Redfish{
			Config: struct {
				Web struct {
					Insecure bool `yaml:"insecure_skip_verify"`
				} `yaml:"web"`
				Targets []Target `yaml:"targets"`
			}{
				Targets: []Target{
					{
						HostAddrs: []string{remoteIPs[0]},
						URL:       targetURLs[0],
					},
					{
						HostAddrs: []string{remoteIPs[1]},
						URL:       targetURLs[1],
					},
				},
			},
		},
	}

	p, l, err := common.GetFreePort()
	require.NoError(t, err)
	l.Close()

	// Web addresses
	config.Web.Addresses = []string{":" + strconv.FormatInt(int64(p), 10)}

	// New instance
	server := NewRedfishProxyServer(config)

	// Start server
	go func() {
		server.Start()
	}()

	time.Sleep(500 * time.Millisecond)

	// Make requests
	client := http.Client{}

	for _, ip := range remoteIPs {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d", p), nil) //nolint:noctx
		require.NoError(t, err)

		req.Header.Add(realIPHeaderName, ip)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Check the body if it has same IP set
		assert.Equal(t, strings.Join([]string{ip}, ","), string(bodyBytes))
	}
}

func TestNewRedfishProxyServerWithWebConfig(t *testing.T) {
	// Start test targets
	targets, remoteIPs := testTargets()

	// Target URLs
	var targetURLs []*url.URL

	for _, t := range targets {
		u, _ := url.Parse(t.URL)
		targetURLs = append(targetURLs, u)

		defer t.Close()
	}

	// Port for 1st server
	port := targetURLs[0].Port()

	// Test config
	config := &Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Redfish: &Redfish{
			Config: struct {
				Web struct {
					Insecure bool `yaml:"insecure_skip_verify"`
				} `yaml:"web"`
				Targets []Target `yaml:"targets"`
			}{
				Web: struct {
					Insecure bool `yaml:"insecure_skip_verify"`
				}{
					Insecure: true,
				},
			},
		},
	}

	p, l, err := common.GetFreePort()
	require.NoError(t, err)
	l.Close()

	// Web addresses
	config.Web.Addresses = []string{":" + strconv.FormatInt(int64(p), 10)}

	// New instance
	server := NewRedfishProxyServer(config)

	// Start server
	go func() {
		server.Start()
	}()

	time.Sleep(500 * time.Millisecond)

	// Make requests
	client := http.Client{}

	// Make request to only 1st server
	for i := range 2 {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d", p), nil) //nolint:noctx
		require.NoError(t, err)

		// Make second request without Redfish URL header and it should pass as we
		// must have populated targets map with first request
		if i == 0 {
			req.Header.Add(redfishURLHeaderName, "http://localhost:"+port)
		}

		req.Header.Add(realIPHeaderName, remoteIPs[0])

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Check the body if it has same IP set
		assert.Equal(t, strings.Join([]string{remoteIPs[0]}, ","), string(bodyBytes))
	}
}
