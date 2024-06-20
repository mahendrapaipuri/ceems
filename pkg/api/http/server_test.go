package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

func setupServer() *CEEMSServer {
	logger := log.NewNopLogger()
	server, _, _ := NewCEEMSServer(&Config{Logger: logger})
	server.maxQueryPeriod = time.Duration(time.Hour * 168)
	server.queriers = queriers{
		unit:    unitQuerier,
		usage:   usageQuerier,
		project: projectQuerier,
		user:    userQuerier,
		cluster: clusterQuerier,
	}
	return server
}

func unitQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Unit, error) {
	return []models.Unit{{UUID: "1000", Usr: "user"}, {UUID: "10001", Usr: "user"}}, nil
}

func usageQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Usage, error) {
	return []models.Usage{{Project: "foo"}, {Project: "bar"}}, nil
}

func projectQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Project, error) {
	return []models.Project{{Name: "foo"}, {Name: "bar"}}, nil
}

func userQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.User, error) {
	return []models.User{{Name: "foo"}, {Name: "bar"}}, nil
}

func clusterQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Cluster, error) {
	return []models.Cluster{{ID: "slurm-0", Manager: "slurm"}, {ID: "os-0", Manager: "openstack"}}, nil
}

func getMockUnits(
	_ Query,
	_ log.Logger,
) ([]models.Unit, error) {
	return []models.Unit{{UUID: "1000", Usr: "user"}, {UUID: "10001", Usr: "user"}}, nil
}

// func getMockAdminUsers(url string, client *http.Client, logger log.Logger) ([]string, error) {
// 	return []string{"adm1", "adm2"}, nil
// }

// // Test /api/projects when no user header found
// func TestAccountsHandlerNoUserHeader(t *testing.T) {
// 	server := setupServer()
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.projects(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Unmarshal byte into structs.
// 	var response Response
// 	json.Unmarshal(data, &response)

// 	if response.Status != "error" {
// 		t.Errorf("expected error status got %v", response.Status)
// 	}
// 	if response.ErrorType != "user_error" {
// 		t.Errorf("expected user_error type got %v", response.ErrorType)
// 	}
// 	if response.Data != nil {
// 		t.Errorf("expected nil data got %v", response.Data)
// 	}
// }

// Test /projects
func TestProjectsHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	// Add user header
	// req.Header.Set("X-Grafana-User", "foo")

	// Start recorder
	w := httptest.NewRecorder()
	server.projects(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedAccounts, _ := projectQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Project]
	json.Unmarshal(data, &response)
	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(response.Data, expectedAccounts) {
		t.Errorf("expected projects %#v got %#v", expectedAccounts, response.Data)
	}
}

// Test /users
func TestUsersHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")

	// Start recorder
	w := httptest.NewRecorder()
	server.users(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedUsers, _ := userQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.User]
	json.Unmarshal(data, &response)
	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(response.Data, expectedUsers) {
		t.Errorf("expected users %#v got %#v", expectedUsers, response.Data)
	}
}

// // Test /api/units when no user header found
// func TestUnitsHandlerNoUserHeader(t *testing.T) {
// 	server := setupServer()
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.units(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Unmarshal byte into structs.
// 	var response Response
// 	json.Unmarshal(data, &response)

// 	if response.Status != "error" {
// 		t.Errorf("expected error status got %v", response.Status)
// 	}
// 	if response.ErrorType != "user_error" {
// 		t.Errorf("expected user_error type got %v", response.ErrorType)
// 	}
// 	if response.Data != nil {
// 		t.Errorf("expected nil data got %v", response.Data)
// 	}
// }

// Test /units
func TestUnitsHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	currentUser := "foo"
	req.Header.Set("X-Grafana-User", currentUser)

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedUnits, _ := unitQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Unit]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	if !reflect.DeepEqual(expectedUnits, response.Data) {
		t.Errorf("expected units %d units, got %d", len(expectedUnits), len(response.Data))
	}
}

// // Test /api/units when user header and impersonated user header found
// func TestUnitsHandlerWithUserHeaderAndAdmin(t *testing.T) {
// 	server := setupServer()
// 	// server.adminUsers = []string{"admin"}
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)
// 	// Add user header
// 	// req.Header.Set("X-Grafana-User", server.adminUsers[0])
// 	req.Header.Set("X-Dashboard-User", "foo")

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.units(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Expected result
// 	expectedUnits, _ := getMockUnits(Query{}, server.logger)

// 	// Unmarshal byte into structs.
// 	var response Response
// 	json.Unmarshal(data, &response)

// 	if response.Status != "success" {
// 		t.Errorf("expected success status got %v", response.Status)
// 	}
// 	if !reflect.DeepEqual(response.Data, expectedUnits) {
// 		t.Errorf("expected %v got %v", expectedUnits, response.Data)
// 	}
// }

// Test /units when from/to query parameters are malformed
func TestUnitsHandlerWithMalformedQueryParams(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "10-12-2023")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Unmarshal byte into structs.
	var response Response[any]
	json.Unmarshal(data, &response)

	if response.Status != "error" {
		t.Errorf("expected error status got %v", response.Status)
	}
	if response.ErrorType != "bad_data" {
		t.Errorf("expected data_error type got %v", response.ErrorType)
	}
	if response.Data != nil {
		t.Errorf("expected nil data got %v", response.Data)
	}
}

// Test /units when from/to query parameters exceed max time window
func TestUnitsHandlerWithQueryWindowExceeded(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "1672527600")
	q.Add("to", "1685570400")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Unmarshal byte into structs.
	var response Response[any]
	json.Unmarshal(data, &response)

	if response.Status != "error" {
		t.Errorf("expected error status got %v", response.Status)
	}
	if response.Error != "maximum query window exceeded" {
		t.Errorf("expected Maximum time window exceeded got %v", response.Error)
	}
	if response.Data != nil {
		t.Errorf("expected nil data got %v", response.Data)
	}
}

// Test /units when from/to query parameters exceed max time window but when unit uuids
// are present
func TestUnitsHandlerWithUnituuidsQueryParams(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "1672527600")
	q.Add("to", "1685570400")
	q.Add("uuid", "foo-bar")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.units(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedUnits, _ := getMockUnits(Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Unit]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	if !reflect.DeepEqual(expectedUnits, response.Data) {
		t.Errorf("expected %#v units, got %#v", expectedUnits, response.Data)
	}
}

// Test /usage
func TestUsageHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/current", nil)
	// Need to set path variables here
	req = mux.SetURLVars(req, map[string]string{"mode": "current"})

	// Add user header
	currentUser := "foo"
	req.Header.Set("X-Grafana-User", currentUser)

	// Start recorder
	w := httptest.NewRecorder()
	server.usage(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedUsage, _ := usageQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Usage]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %#v", response)
	}

	if !reflect.DeepEqual(expectedUsage, response.Data) {
		t.Errorf("expected usage %#v usage, got %#v", expectedUsage, response.Data)
	}
}

// Test /clusters
func TestClustersHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/admin", nil)
	// Add user header
	currentUser := "foo"
	req.Header.Set("X-Grafana-User", currentUser)

	// Start recorder
	w := httptest.NewRecorder()
	server.clustersAdmin(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedClusters, _ := clusterQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs
	var response Response[models.Cluster]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	if !reflect.DeepEqual(expectedClusters, response.Data) {
		t.Errorf("expected clusters %#v clusters, got %#v", expectedClusters, response.Data)
	}
}
