package server

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
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
)

func setupServer() *JobstatsServer {
	logger := log.NewNopLogger()
	server, _, _ := NewJobstatsServer(&Config{Logger: logger})
	server.maxQueryPeriod = time.Duration(time.Hour * 168)
	server.Querier = mockQuerier
	return server
}

func mockQuerier(db *sql.DB, q Query, model string, logger log.Logger) (interface{}, error) {
	if model == "jobs" {
		return []base.Job{{Jobid: 1000, Usr: "user"}, {Jobid: 10001, Usr: "user"}}, nil
	} else if model == "usage" {
		return []base.Usage{{Account: "foo"}, {Account: "bar"}}, nil
	} else if model == "accounts" {
		return []base.Account{{Name: "foo"}, {Name: "bar"}}, nil
	}
	return nil, fmt.Errorf("unknown model")
}

func getMockJobs(
	query Query,
	logger log.Logger,
) ([]base.Job, error) {
	return []base.Job{{Jobid: 1000, Usr: "user"}, {Jobid: 10001, Usr: "user"}}, nil
}

func getMockAdminUsers(url string, client *http.Client, logger log.Logger) ([]string, error) {
	return []string{"adm1", "adm2"}, nil
}

// // Test /api/accounts when no user header found
// func TestAccountsHandlerNoUserHeader(t *testing.T) {
// 	server := setupServer()
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.accounts(w, req)
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

// Test /api/accounts
func TestAccountsHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	// Add user header
	// req.Header.Set("X-Grafana-User", "foo")

	// Start recorder
	w := httptest.NewRecorder()
	server.accounts(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedAccounts, _ := mockQuerier(server.db, Query{}, "accounts", server.logger)

	// Unmarshal byte into structs.
	var response Response
	json.Unmarshal(data, &response)
	var accountData []base.Account
	for _, name := range response.Data.([]interface{}) {
		accountData = append(accountData, base.Account{Name: name.(map[string]interface{})["name"].(string)})
	}

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(accountData, expectedAccounts) {
		t.Errorf("expected %#v got %#v", expectedAccounts, accountData)
	}
}

// // Test /api/jobs when no user header found
// func TestJobsHandlerNoUserHeader(t *testing.T) {
// 	server := setupServer()
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.jobs(w, req)
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

// Test /api/jobs
func TestJobsHandler(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	// Add user header
	currentUser := "foo"
	req.Header.Set("X-Grafana-User", currentUser)

	// Start recorder
	w := httptest.NewRecorder()
	server.jobs(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedJobs, _ := getMockJobs(Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	var jobData = response.Data.([]interface{})
	if len(jobData) != len(expectedJobs) {
		t.Errorf("expected %d jobs, got %d", len(jobData), len(expectedJobs))
	}
}

// // Test /api/jobs when user header and impersonated user header found
// func TestJobsHandlerWithUserHeaderAndAdmin(t *testing.T) {
// 	server := setupServer()
// 	// server.adminUsers = []string{"admin"}
// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
// 	// Add user header
// 	// req.Header.Set("X-Grafana-User", server.adminUsers[0])
// 	req.Header.Set("X-Dashboard-User", "foo")

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.jobs(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Expected result
// 	expectedJobs, _ := getMockJobs(Query{}, server.logger)

// 	// Unmarshal byte into structs.
// 	var response Response
// 	json.Unmarshal(data, &response)

// 	if response.Status != "success" {
// 		t.Errorf("expected success status got %v", response.Status)
// 	}
// 	if !reflect.DeepEqual(response.Data, expectedJobs) {
// 		t.Errorf("expected %v got %v", expectedJobs, response.Data)
// 	}
// }

// Test /api/jobs when from/to query parameters are malformed
func TestJobsHandlerWithMalformedQueryParams(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "10-12-2023")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.jobs(w, req)
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

// Test /api/jobs when from/to query parameters exceed max time window
func TestJobsHandlerWithQueryWindowExceeded(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("from", "1672527600")
	q.Add("to", "1685570400")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.jobs(w, req)
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
	if response.Error != "Maximum query window exceeded" {
		t.Errorf("expected Maximum time window exceeded got %v", response.Error)
	}
	if response.Data != nil {
		t.Errorf("expected nil data got %v", response.Data)
	}
}

// Test /api/jobs when from/to query parameters exceed max time window but when jobuuids
// are present
func TestJobsHandlerWithJobuuidsQueryParams(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")
	// Add from query parameter
	q := req.URL.Query()
	q.Add("jobuuid", "foo-bar")
	req.URL.RawQuery = q.Encode()

	// Start recorder
	w := httptest.NewRecorder()
	server.jobs(w, req)
	res := w.Result()
	defer res.Body.Close()

	// Get body
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}

	// Expected result
	expectedJobs, _ := getMockJobs(Query{}, server.logger)

	// Unmarshal byte into structs.
	var response Response
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}

	var jobData = response.Data.([]interface{})
	if len(jobData) != len(expectedJobs) {
		t.Errorf("expected %d jobs, got %d", len(jobData), len(expectedJobs))
	}
}
