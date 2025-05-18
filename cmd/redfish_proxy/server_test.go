package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/prometheus/common/config"
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
		Logger: slog.New(slog.DiscardHandler),
		Redfish: &Redfish{
			Config: struct {
				Web struct {
					HTTPClientConfig          config.HTTPClientConfig `yaml:",inline"`
					AllowedAPIResources       []string                `yaml:"allowed_api_resources"`
					Insecure                  bool                    `yaml:"insecure_skip_verify"`
					allowedAPIResourcesRegexp *regexp.Regexp
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
	server, err := NewRedfishProxyServer(config)
	require.NoError(t, err)

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
		Logger: slog.New(slog.DiscardHandler),
		Redfish: &Redfish{
			Config: struct {
				Web struct {
					HTTPClientConfig          config.HTTPClientConfig `yaml:",inline"`
					AllowedAPIResources       []string                `yaml:"allowed_api_resources"`
					Insecure                  bool                    `yaml:"insecure_skip_verify"`
					allowedAPIResourcesRegexp *regexp.Regexp
				} `yaml:"web"`
				Targets []Target `yaml:"targets"`
			}{
				Web: struct {
					HTTPClientConfig          config.HTTPClientConfig `yaml:",inline"`
					AllowedAPIResources       []string                `yaml:"allowed_api_resources"`
					Insecure                  bool                    `yaml:"insecure_skip_verify"`
					allowedAPIResourcesRegexp *regexp.Regexp
				}{
					Insecure:                  true,
					allowedAPIResourcesRegexp: regexp.MustCompile(strings.Join(defaultAllowedAPIResources, "|")),
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

	// Make request to only 1st server
	for i := range 2 {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/redfish/v1/", p), nil) //nolint:noctx
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

	// Make a request without redfishURL header and invalid remoteIP
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/redfish/v1/", p), nil) //nolint:noctx
	require.NoError(t, err)

	req.Header.Add(realIPHeaderName, "1.1.1.1")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the body if it has same IP set
	assert.Equal(t, 400, resp.StatusCode)

	// Start a wait group
	wg := sync.WaitGroup{}

	// Use a channel to hand off the error
	// Ref: https://github.com/ipfs/kubo/issues/2043#issuecomment-164136026
	errs := make(chan error, 20*3)

	// Make concurrent requests to detect data races
	for i := range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/redfish/v1/", p), nil) //nolint:noctx
			errs <- err

			// Target ID
			tid := i % len(targetURLs)

			req.Header.Add(redfishURLHeaderName, "http://localhost:"+targetURLs[tid].Port())
			req.Header.Add(realIPHeaderName, remoteIPs[tid])

			resp, err := client.Do(req)
			errs <- err
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			errs <- err

			// Check the body if it has same IP set
			assert.Equal(t, strings.Join([]string{remoteIPs[tid]}, ","), string(bodyBytes))
		}()
	}

	// Wait for all requests to finish
	wg.Wait()

	// Check for any errs
	// wait for all N to finish
	for range 20 * 3 {
		err := <-errs
		require.NoError(t, err)
	}
}
