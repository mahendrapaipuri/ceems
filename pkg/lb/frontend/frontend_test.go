//go:build cgo
// +build cgo

package frontend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/cli"
	ceems_db "github.com/ceems-dev/ceems/pkg/api/db"
	ceems_api_http "github.com/ceems-dev/ceems/pkg/api/http"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/lb/backend"
	"github.com/ceems-dev/ceems/pkg/lb/base"
	"github.com/ceems-dev/ceems/pkg/lb/serverpool"
	"github.com/ceems-dev/ceems/pkg/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var noOpLogger = slog.New(slog.DiscardHandler)

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
	expectedConfig := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s",
		},
	}

	expectedFlags := tsdb.Response[any]{
		Status: "success",
		Data: map[string]any{
			"query.lookback-delta": "5m",
			"query.max-samples":    "50000000",
			"query.timeout":        "2m",
		},
	}

	expectedRuntimeInfo := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"storageRetention": "30d",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "config") {
			if err := json.NewEncoder(w).Encode(&expectedConfig); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "flags") {
			if err := json.NewEncoder(w).Encode(&expectedFlags); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntimeInfo); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			w.Write([]byte(clusterID))
		}
	}))

	return server
}

func TestNewFrontend(t *testing.T) {
	tmpDir := t.TempDir()
	err := setupClusterIDsDB(tmpDir)
	require.NoError(t, err, "failed to setup test DB")

	clusterID := "slurm-0"

	// Backends
	dummyServer1 := dummyTSDBServer(clusterID)
	defer dummyServer1.Close()

	backend1, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer1.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add(clusterID, backend1)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
		APIServer: cli.CEEMSAPIServerConfig{
			Data: ceems_db.DataConfig{Path: tmpDir},
		},
	}

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

	// // Use a channel to hand off the error
	// // Ref: https://github.com/ipfs/kubo/issues/2043#issuecomment-164136026
	// errs := make(chan error, 1)
	// Seems like in CI we are still getting error
	// listen tcp 127.0.0.1:9030: bind: address already in use
	// // Wait for it
	// // err = <-errs
	// require.NoError(t, err)

	go func() {
		// err := lb.Start(t.Context())
		// errs <- err
		lb.Start(t.Context())
	}()

	err = lb.Shutdown(t.Context())
	require.NoError(t, err)
}

func TestNewFrontendSingleGroup(t *testing.T) {
	clusterID := "default"

	// Backends
	dummyServer1 := dummyTSDBServer(clusterID)
	defer dummyServer1.Close()

	backend1, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer1.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add(clusterID, backend1)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
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
		// {
		// 	name:     "query with params in ctx and start more than retention period",
		// 	start:    time.Now().UTC().Add(-32 * 24 * time.Hour).Unix(),
		// 	code:     503,
		// 	response: false,
		// },
	}

	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/test", nil)

		// Add uuids and start to context
		var newReq *http.Request

		if test.start > 0 {
			newReq = request.WithContext(
				context.WithValue(
					request.Context(), ReqParamsContextKey{},
					&ReqParams{clusterID: clusterID},
				),
			)
		} else {
			newReq = request
		}

		responseRecorder := httptest.NewRecorder()
		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

		assert.Equal(t, responseRecorder.Code, test.code, test.name)

		if test.response {
			assert.Equal(t, responseRecorder.Body.String(), clusterID, test.name)
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	newReq := request.WithContext(
		context.WithValue(
			request.Context(), ReqParamsContextKey{},
			&ReqParams{clusterID: "default"},
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

	backend1, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer1.URL}}, noOpLogger)
	require.NoError(t, err)

	// Backends for group 2
	dummyServer2 := dummyTSDBServer("rm-1")
	defer dummyServer2.Close()

	backend2, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer2.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add("rm-0", backend1)
	manager.Add("rm-1", backend2)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
	}

	// New load balancer
	lb, err := New(config)
	require.NoError(t, err)

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
		// {
		// 	name:      "query with params in ctx and start more than retention period",
		// 	start:     time.Now().UTC().Add(-31 * 24 * time.Hour).Unix(),
		// 	clusterID: "rm-0",
		// 	code:      503,
		// 	response:  false,
		// },
	}

	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "/test", nil)

		// Add uuids and start to context
		var newReq *http.Request

		if test.start > 0 {
			newReq = request.WithContext(
				context.WithValue(
					request.Context(), ReqParamsContextKey{},
					&ReqParams{clusterID: test.clusterID},
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
			request.Context(), ReqParamsContextKey{},
			&ReqParams{clusterID: "rm-0"},
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

	backend, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
	}
	config.APIServer.Data.Path = tmpDir

	// New load balancer
	_, err = New(config)
	require.NoError(t, err)
}

func TestValidateClusterIDsWithDBFail(t *testing.T) {
	tmpDir := t.TempDir()
	err := setupClusterIDsDB(tmpDir)
	require.NoError(t, err, "failed to setup test DB")

	// Backends for group 1
	dummyServer := dummyTSDBServer("slurm-0")
	defer dummyServer.Close()

	backend, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add("unknown", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
	}
	config.APIServer.Data.Path = tmpDir

	// New load balancer
	_, err = New(config)
	require.Error(t, err)
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

	backend, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
	}
	config.APIServer.Web.URL = ceemsServer.URL

	// New load balancer
	_, err = New(config)
	require.NoError(t, err)
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

	backend, err := backend.NewTSDB(&backend.ServerConfig{Web: &models.WebConfig{URL: dummyServer.URL}}, noOpLogger)
	require.NoError(t, err)

	// Start manager
	manager, err := serverpool.New(base.RoundRobin, noOpLogger)
	require.NoError(t, err)

	manager.Add("slurm-0", backend)
	manager.Add("os-1", backend)

	// make minimal config
	config := &Config{
		Logger:  noOpLogger,
		Manager: manager,
		Address: "localhost:9030", // dummy address
	}
	config.APIServer.Web.URL = ceemsServer.URL

	// New load balancer
	_, err = New(config)
	require.Error(t, err)
}
