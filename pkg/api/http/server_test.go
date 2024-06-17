package http

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
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

// Test /api/projects
func TestAccountsHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
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
		t.Errorf("expected %#v got %#v", expectedAccounts, response.Data)
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

// Test /api/units
func TestUnitsHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)
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

	if len(response.Data) != len(expectedUnits) {
		t.Errorf("expected %d units, got %d", len(response.Data), len(expectedUnits))
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

// Test /api/units when from/to query parameters are malformed
func TestUnitsHandlerWithMalformedQueryParams(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)
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

// Test /api/units when from/to query parameters exceed max time window
func TestUnitsHandlerWithQueryWindowExceeded(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)
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

// Test /api/units when from/to query parameters exceed max time window but when unituuids
// are present
func TestUnitsHandlerWithUnituuidsQueryParams(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/units", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("unituuid", "foo-bar")
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

	var unitData = response.Data
	if len(unitData) != len(expectedUnits) {
		t.Errorf("expected %d units, got %d", len(unitData), len(expectedUnits))
	}
}

// Test /api/usage
func TestUsageHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
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
	expectedUsage, _ := usageQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Unit]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	if len(response.Data) != len(expectedUsage) {
		t.Errorf("expected %d usage, got %d", len(response.Data), len(expectedUsage))
	}
}

// Test /api/clusters
func TestClustersHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/clusters/admin", nil)
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
	expectedClusters, _ := clusterQuerier(server.db, Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response[models.Unit]
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	if len(response.Data) != len(expectedClusters) {
		t.Errorf("expected %d clusters, got %d", len(response.Data), len(expectedClusters))
	}
}
