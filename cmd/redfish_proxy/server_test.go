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
	clientIPs := []string{"192.168.1.1", "192.168.1.2"}
	// Test redfish server
	for i := range 2 {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(clientIPs[i]))
		}))
	}

	return servers, clientIPs
}

func TestNewRedfishProxyServer(t *testing.T) {
	// Start test targets
	targets, clientIPs := testTargets()

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
					SkipTLSVerify bool `yaml:"insecure_skip_verify"`
				} `yaml:"web"`
				Targets []Target `yaml:"targets"`
			}{
				Targets: []Target{
					{
						HostAddrs: []string{clientIPs[0]},
						URL:       targetURLs[0],
					},
					{
						HostAddrs: []string{clientIPs[1]},
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
	server, err := NewRedfishProxyServer(config)
	require.NoError(t, err)

	// Start server
	go func() {
		server.Start()
	}()

	time.Sleep(500 * time.Millisecond)

	// Make requests
	client := http.Client{}

	for ip := range server.targets {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d", p), nil) //nolint:noctx
		require.NoError(t, err)

		req.Header.Add(realIPHeaderName, ip)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Check the body if it has same IP set
		assert.EqualValues(t, strings.Join([]string{ip}, ","), string(bodyBytes))
	}
}
