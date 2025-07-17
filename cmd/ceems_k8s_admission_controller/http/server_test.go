package http

import (
	"net/http"
	"testing"

	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdmissionControllerServer(t *testing.T) {
	// Test config
	c := &base.Config{
		Logger: noOpLogger,
		Web: base.WebConfig{
			Addresses: []string{":9000"},
		},
	}

	// Create new server
	server, err := NewAdmissionControllerServer(c)
	require.NoError(t, err)

	// Start server in go routine
	go server.Start()

	// Make invalid requests
	tests := []struct {
		name   string
		method string
		url    string
		code   int
	}{
		{
			name:   "GET request to validate",
			method: http.MethodGet,
			url:    validatePath,
			code:   http.StatusMethodNotAllowed,
		},
		{
			name:   "GET request to mutate",
			method: http.MethodGet,
			url:    mutatePath,
			code:   http.StatusMethodNotAllowed,
		},
		{
			name:   "POST request to health",
			method: http.MethodPost,
			url:    "/health",
			code:   http.StatusMethodNotAllowed,
		},
		{
			name:   "GET request to health",
			method: http.MethodGet,
			url:    "/health",
			code:   http.StatusOK,
		},
	}

	for _, test := range tests {
		var resp *http.Response

		var err error
		if test.method == http.MethodGet {
			resp, err = http.Get("http://localhost:9000" + test.url) //nolint:noctx
		} else {
			resp, err = http.Post("http://localhost:9000"+test.url, "application/json", nil) //nolint:noctx
		}

		require.NoError(t, err, test.name)

		defer resp.Body.Close()
		assert.Equal(t, test.code, resp.StatusCode, test.name)
	}

	// Stop server
	err = server.Shutdown(t.Context())
	require.NoError(t, err)
}
