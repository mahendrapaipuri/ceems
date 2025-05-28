package collector

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type req struct {
	path     string
	respCode int
}

func TestCEEMSExporterServer(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		reqs   []req
	}{
		{
			name: "separate metrics and landing page",
			config: &Config{
				Logger:    noOpLogger,
				Collector: &CEEMSCollector{logger: noOpLogger},
				Web: WebConfig{
					MetricsPath: "/metrics",
					MaxRequests: 5,
					LandingConfig: &web.LandingConfig{
						Name: "CEEMS Exporter",
					},
				},
			},
			reqs: []req{
				{
					path:     "/metrics",
					respCode: 200,
				},
				{
					path:     "/",
					respCode: 200,
				},
				{
					path:     "/debug/pprof/",
					respCode: 404,
				},
			},
		},
		{
			name: "only metrics without landing page",
			config: &Config{
				Logger:    noOpLogger,
				Collector: &CEEMSCollector{logger: noOpLogger},
				Web: WebConfig{
					MetricsPath: "/",
					MaxRequests: 5,
					LandingConfig: &web.LandingConfig{
						Name: "CEEMS Exporter",
					},
				},
			},
			reqs: []req{
				{
					path:     "/",
					respCode: 200,
				},
				{
					path:     "/metrics",
					respCode: 404,
				},
			},
		},
		{
			name: "separate metrics and landing page and with debug server",
			config: &Config{
				Logger:    noOpLogger,
				Collector: &CEEMSCollector{logger: noOpLogger},
				Web: WebConfig{
					MetricsPath:       "/metrics",
					MaxRequests:       5,
					EnableDebugServer: true,
					LandingConfig: &web.LandingConfig{
						Name: "CEEMS Exporter",
					},
				},
			},
			reqs: []req{
				{
					path:     "/metrics",
					respCode: 200,
				},
				{
					path:     "/",
					respCode: 200,
				},
				{
					path:     "/debug/pprof/",
					respCode: 200,
				},
				{
					path:     "/targets",
					respCode: 404,
				},
			},
		},
	}

	for _, test := range tests {
		p, l, err := common.GetFreePort()
		require.NoError(t, err)
		l.Close()

		// Web addresses
		test.config.Web.Addresses = []string{":" + strconv.FormatInt(int64(p), 10)}

		// New instance
		server, err := NewCEEMSExporterServer(test.config)
		require.NoError(t, err)

		// Start server
		go func() {
			server.Start()
		}()

		time.Sleep(500 * time.Millisecond)

		for _, req := range test.reqs {
			// Make request
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", p, req.path)) //nolint:noctx
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, req.respCode, resp.StatusCode, "name: %s path: %s", test.name, req.path)
		}
	}
}
