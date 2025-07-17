package backend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/tsdb"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTSDBServer(storageRetention string, emptyResponse bool, basicAuth bool) *httptest.Server {
	// Start test server
	expectedRuntime := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": storageRetention,
		},
	}
	expectedRange := tsdb.Response[any]{
		Status: "success",
		Data: map[string]any{
			"resultType": "matrix",
			"result": []any{
				map[string]any{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []any{
						[]any{time.Now().Add(-15 * 24 * time.Hour).Unix(), "1"},
						[]any{time.Now().Add(-15 * 23 * time.Hour).Unix(), "1"},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if basicAuth {
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusForbidden)

				return
			}
		}

		if emptyResponse {
			expected := "dummy"
			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}

			return
		}

		if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntime); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			if err := json.NewEncoder(w).Encode(&expectedRange); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))

	return server
}

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer("30d", false, false)
	// defer server.Close()

	b, err := NewTSDB(&ServerConfig{Web: &models.WebConfig{URL: server.URL}}, noOpLogger)
	require.NoError(t, err)

	assert.Equal(t, server.URL, b.URL().String())
	assert.Equal(t, 354*time.Hour, b.RetentionPeriod())
	assert.True(t, b.IsAlive())
	assert.Equal(t, 0, b.ActiveConnections())
	assert.NotEmpty(t, b.ReverseProxy())

	// Stop dummy server and query for retention period, we should get last updated value
	server.Close()
	assert.Equal(t, 354*time.Hour, b.RetentionPeriod())
}

func TestTSDBConfigSuccessWithTwoRetentions(t *testing.T) {
	// Start test server
	server := testTSDBServer("30d or 10GiB", false, false)
	defer server.Close()

	b, err := NewTSDB(&ServerConfig{Web: &models.WebConfig{URL: server.URL}}, noOpLogger)
	require.NoError(t, err)

	assert.Equal(t, server.URL, b.URL().String())
	assert.Equal(t, 354*time.Hour, b.RetentionPeriod())
	assert.True(t, b.IsAlive())
}

func TestTSDBConfigSuccessWithRetentionSize(t *testing.T) {
	// Start test server
	server := testTSDBServer("10GiB", false, false)
	defer server.Close()

	b, err := NewTSDB(&ServerConfig{Web: &models.WebConfig{URL: server.URL}}, noOpLogger)
	require.NoError(t, err)

	assert.Equal(t, server.URL, b.URL().String())
	assert.Equal(t, 354*time.Hour, b.RetentionPeriod())
	assert.True(t, b.IsAlive())
}

func TestTSDBConfigFail(t *testing.T) {
	// Start test server
	server := testTSDBServer("10GiB", true, false)
	defer server.Close()

	b, err := NewTSDB(&ServerConfig{Web: &models.WebConfig{URL: server.URL}}, noOpLogger)
	require.NoError(t, err)

	assert.Equal(t, server.URL, b.URL().String())
	assert.Equal(t, 0*time.Hour, b.RetentionPeriod())
	assert.True(t, b.IsAlive())
}

func TestTSDBBackendWithBasicAuth(t *testing.T) {
	// Start test server
	server := testTSDBServer("30d", false, true)
	defer server.Close()

	c := &ServerConfig{
		Web: &models.WebConfig{
			URL: server.URL,
			HTTPClientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username: "prometheus",
					Password: "secret",
				},
			},
		},
	}
	b, err := NewTSDB(c, noOpLogger)
	require.NoError(t, err)

	assert.Equal(t, 354*time.Hour, b.RetentionPeriod())
	assert.True(t, b.IsAlive())
}

func TestTSDBQueryWithLabelFilter(t *testing.T) {
	// Start test server
	server := testTSDBServer("30d", false, false)
	defer server.Close()

	b, err := NewTSDB(
		&ServerConfig{Web: &models.WebConfig{URL: server.URL}, FilterLabels: []string{"instance"}},
		noOpLogger,
	)
	require.NoError(t, err)

	// Make a request to query resource
	req := httptest.NewRequest(http.MethodGet, "/query", nil)

	responseRecorder := httptest.NewRecorder()
	http.HandlerFunc(b.Serve).ServeHTTP(responseRecorder, req)

	// Ensure that filtered labels do not exist on response
	body, err := io.ReadAll(responseRecorder.Body)
	require.NoError(t, err)

	var tsdbResp tsdb.Response[tsdb.Data]
	err = json.Unmarshal(body, &tsdbResp)
	require.NoError(t, err)

	assert.Empty(t, tsdbResp.Data.Result[0].Metric["instance"])
}

func TestTSDBBackendAlive(t *testing.T) {
	c := &ServerConfig{Web: &models.WebConfig{URL: "http://localhost:9090"}}
	b, err := NewTSDB(c, noOpLogger)
	require.NoError(t, err)

	b.SetAlive(b.IsAlive())

	assert.True(t, b.IsAlive())
}
