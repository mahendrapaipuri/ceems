package http

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/stats/models"
)

func setupServer() *CEEMSServer {
	logger := log.NewNopLogger()
	server, _, _ := NewCEEMSServer(&Config{Logger: logger})
	server.maxQueryPeriod = time.Duration(time.Hour * 168)
	server.Querier = mockQuerier
	return server
}

func mockQuerier(db *sql.DB, q Query, model string, logger log.Logger) (interface{}, error) {
	if model == "units" {
		return []models.Unit{{UUID: "1000", Usr: "user"}, {UUID: "10001", Usr: "user"}}, nil
	} else if model == "usage" {
		return []models.Usage{{Project: "foo"}, {Project: "bar"}}, nil
	} else if model == "projects" {
		return []models.Project{{Name: "foo"}, {Name: "bar"}}, nil
	}
	return nil, fmt.Errorf("unknown model")
}

func getMockUnits(
	query Query,
	logger log.Logger,
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
	expectedAccounts, _ := mockQuerier(server.db, Query{}, "projects", server.logger)

	// Unmarshal byte into structs.
	var response Response
	json.Unmarshal(data, &response)
	var projectData []models.Project
	for _, name := range response.Data.([]interface{}) {
		projectData = append(projectData, models.Project{Name: name.(map[string]interface{})["name"].(string)})
	}

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(projectData, expectedAccounts) {
		t.Errorf("expected %#v got %#v", expectedAccounts, projectData)
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
	expectedUnits, _ := getMockUnits(Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	var unitData = response.Data.([]interface{})
	if len(unitData) != len(expectedUnits) {
		t.Errorf("expected %d units, got %d", len(unitData), len(expectedUnits))
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
	var response Response
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
	var response Response
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
	var response Response
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	var unitData = response.Data.([]interface{})
	if len(unitData) != len(expectedUnits) {
		t.Errorf("expected %d units, got %d", len(unitData), len(expectedUnits))
	}
}
