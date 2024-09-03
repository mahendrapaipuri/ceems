package frontend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	ceems_api_http "github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupClusterIDsDB(d string) error {
	dbPath := filepath.Join(d, "ceems.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to create DB: %w", err)
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE units (
	"id" integer not null primary key,
	"cluster_id" text,
	"resource_manager" text
);
INSERT INTO units VALUES(1, 'slurm-0', 'slurm');
INSERT INTO units VALUES(2, 'os-0', 'openstack');
INSERT INTO units VALUES(3, 'os-1', 'openstack');
INSERT INTO units VALUES(4, 'slurm-1', 'slurm');
COMMIT;`

	_, err = db.Exec(stmts)
	if err != nil {
		return fmt.Errorf("failed to insert mock data into DB: %w", err)
	}

	return nil
}

func dummyTSDBServer(clusterID string) *httptest.Server {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			w.Write([]byte(clusterID))
		}
	}))

	return server
}

func TestNewFrontendSingleGroup(t *testing.T) {
	clusterID := "default"

	// Backends
	dummyServer1 := dummyTSDBServer(clusterID)
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	require.NoError(t, err)

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.New(backend1URL, rp1, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add(clusterID, backend1)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	tests := []struct {
		name     string
		start    int64
		code     int
		response bool
	}{
		{
			name:     "query with params in ctx",
			start:    time.Now().UTC().Unix(),
			code:     200,
			response: true,
		},
		{
			name:     "query with no params in ctx",
			code:     400,
			response: false,
		},
		{
			name:     "query with params in ctx and start more than retention period",
			start:    time.Now().UTC().Add(-31 * 24 * time.Hour).Unix(),
			code:     503,
			response: false,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/test", nil)

		// Add uuids and start to context
		var newReq *http.Request

		if test.start > 0 {
			period := time.Duration((time.Now().UTC().Unix() - test.start)) * time.Second
			newReq = request.WithContext(
				context.WithValue(
					request.Context(), QueryParamsContextKey{},
					&QueryParams{queryPeriod: period, id: clusterID},
				),
			)
		} else {
			newReq = request
		}

		responseRecorder := httptest.NewRecorder()
		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

		assert.Equal(t, responseRecorder.Code, test.code)

		if test.response {
			assert.Equal(t, responseRecorder.Body.String(), clusterID)
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	newReq := request.WithContext(
		context.WithValue(
			request.Context(), QueryParamsContextKey{},
			&QueryParams{id: "default"},
		),
	)
	responseRecorder := httptest.NewRecorder()
	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

	assert.Equal(t, 503, responseRecorder.Code)
}

func TestNewFrontendTwoGroups(t *testing.T) {
	// Backends for group 1
	dummyServer1 := dummyTSDBServer("rm-0")
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	require.NoError(t, err)

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.New(backend1URL, rp1, log.NewNopLogger())

	// Backends for group 2
	dummyServer2 := dummyTSDBServer("rm-1")
	defer dummyServer2.Close()
	backend2URL, err := url.Parse(dummyServer2.URL)
	require.NoError(t, err)

	rp2 := httputil.NewSingleHostReverseProxy(backend2URL)
	backend2 := backend.New(backend2URL, rp2, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add("rm-0", backend1)
	manager.Add("rm-1", backend2)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// Validate cluster IDs
	require.NoError(t, lb.ValidateClusterIDs(context.Background()))

	tests := []struct {
		name      string
		start     int64
		clusterID string
		code      int
		response  bool
	}{
		{
			name:      "query for rm-0 with params in ctx",
			start:     time.Now().UTC().Unix(),
			clusterID: "rm-0",
			code:      200,
			response:  true,
		},
		{
			name:      "query for rm-1 with params in ctx",
			start:     time.Now().UTC().Unix(),
			clusterID: "rm-1",
			code:      200,
			response:  true,
		},
		{
			name:     "query with no clusterID params in ctx",
			start:    time.Now().UTC().Unix(),
			code:     503,
			response: false,
		},
		{
			name:      "query with params in ctx and start more than retention period",
			start:     time.Now().UTC().Add(-31 * 24 * time.Hour).Unix(),
			clusterID: "rm-0",
			code:      503,
			response:  false,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/test", nil)

		// Add uuids and start to context
		var newReq *http.Request

		if test.start > 0 {
			period := time.Duration((time.Now().UTC().Unix() - test.start)) * time.Second
			newReq = request.WithContext(
				context.WithValue(
					request.Context(), QueryParamsContextKey{},
					&QueryParams{queryPeriod: period, id: test.clusterID},
				),
			)
		} else {
			newReq = request
		}

		responseRecorder := httptest.NewRecorder()
		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

		assert.Equal(t, responseRecorder.Code, test.code)

		if test.response {
			assert.Equal(t, responseRecorder.Body.String(), test.clusterID)
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	newReq := request.WithContext(
		context.WithValue(
			request.Context(), QueryParamsContextKey{},
			&QueryParams{id: "rm-0"},
		),
	)
	responseRecorder := httptest.NewRecorder()
	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

	assert.Equal(t, 503, responseRecorder.Code)
}

func TestValidateClusterIDsWithDBPass(t *testing.T) {
	tmpDir := t.TempDir()
	err := setupClusterIDsDB(tmpDir)
	require.NoError(t, err, "failed to setup test DB")

	// Backends for group 1
	dummyServer := dummyTSDBServer("slurm-0")
	defer dummyServer.Close()
	backendURL, err := url.Parse(dummyServer.URL)
	require.NoError(t, err)

	rp := httputil.NewSingleHostReverseProxy(backendURL)
	backend := backend.New(backendURL, rp, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}
	config.APIServer.Data.Path = tmpDir

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// Validate cluster IDs
	require.NoError(t, lb.ValidateClusterIDs(context.Background()))
}

func TestValidateClusterIDsWithDBFail(t *testing.T) {
	tmpDir := t.TempDir()
	err := setupClusterIDsDB(tmpDir)
	require.NoError(t, err, "failed to setup test DB")

	// Backends for group 1
	dummyServer := dummyTSDBServer("slurm-0")
	defer dummyServer.Close()
	backendURL, err := url.Parse(dummyServer.URL)
	require.NoError(t, err)

	rp := httputil.NewSingleHostReverseProxy(backendURL)
	backend := backend.New(backendURL, rp, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add("unknown", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}
	config.APIServer.Data.Path = tmpDir

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// Validate cluster IDs
	require.Error(t, lb.ValidateClusterIDs(context.Background()))
}

func TestValidateClusterIDsWithAPIPass(t *testing.T) {
	// Test CEEMS API server
	expected := ceems_api_http.Response[models.Cluster]{
		Status: "success",
		Data: []models.Cluster{
			{
				ID: "slurm-0",
			},
			{
				ID: "os-1",
			},
		},
	}

	ceemsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer ceemsServer.Close()

	// Backends for group 1
	dummyServer := dummyTSDBServer("slurm-0")
	defer dummyServer.Close()
	backendURL, err := url.Parse(dummyServer.URL)
	require.NoError(t, err)

	rp := httputil.NewSingleHostReverseProxy(backendURL)
	backend := backend.New(backendURL, rp, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}
	config.APIServer.Web.URL = ceemsServer.URL

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// Validate cluster IDs
	require.NoError(t, lb.ValidateClusterIDs(context.Background()))
}

func TestValidateClusterIDsWithAPIFail(t *testing.T) {
	// Test CEEMS API server
	expected := ceems_api_http.Response[models.Cluster]{
		Status: "error",
	}

	ceemsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer ceemsServer.Close()

	// Backends for group 1
	dummyServer := dummyTSDBServer("slurm-0")
	defer dummyServer.Close()
	backendURL, err := url.Parse(dummyServer.URL)
	require.NoError(t, err)

	rp := httputil.NewSingleHostReverseProxy(backendURL)
	backend := backend.New(backendURL, rp, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.New("resource-based", log.NewNopLogger())
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:    log.NewNopLogger(),
		Manager:   manager,
		Addresses: []string{"localhost:9030"}, // dummy address
	}
	config.APIServer.Web.URL = ceemsServer.URL

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// Validate cluster IDs
	require.Error(t, lb.ValidateClusterIDs(context.Background()))
}
