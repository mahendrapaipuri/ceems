package jobstats

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-kit/log"
)

func setupServer() *JobstatsServer {
	logger := log.NewNopLogger()
	server, _, _ := NewJobstatsServer(&Config{Logger: logger})
	server.AccountsGetter = getMockAccounts
	server.JobsGetter = getMockJobs
	return server
}

func getMockAccounts(user string, logger log.Logger) ([]Account, error) {
	return []Account{{ID: "foo"}, {ID: "bar"}}, nil
}

func getMockJobs(user string, accounts []string, from string, to string, logger log.Logger) ([]BatchJob, error) {
	return []BatchJob{{Jobid: "1000"}, {Jobid: "10001"}}, nil
}

// Test /api/accounts when no user header found
func TestAccountsHandlerNoUserHeader(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)

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

	// Unmarshal byte into structs.
	var response AccountsResponse
	json.Unmarshal(data, &response)

	if response.Status != "error" {
		t.Errorf("expected error status got %v", response.Status)
	}
	if response.ErrorType != "User Error" {
		t.Errorf("expected User Error type got %v", response.ErrorType)
	}
	if !reflect.DeepEqual(response.Data, []Account{}) {
		t.Errorf("expected empty data got %v", response.Data)
	}
}

// Test /api/accounts when header found
func TestAccountsHandlerWithUserHeader(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")

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
	expectedAccounts, _ := getMockAccounts("foo", logger)

	// Unmarshal byte into structs.
	var response AccountsResponse
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(response.Data, expectedAccounts) {
		t.Errorf("expected %v got %v", expectedAccounts, response.Data)
	}
}

// Test /api/jobs when no user header found
func TestJobsHandlerNoUserHeader(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)

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
	var response JobsResponse
	json.Unmarshal(data, &response)

	if response.Status != "error" {
		t.Errorf("expected error status got %v", response.Status)
	}
	if response.ErrorType != "User Error" {
		t.Errorf("expected User Error type got %v", response.ErrorType)
	}
	if !reflect.DeepEqual(response.Data, []BatchJob{}) {
		t.Errorf("expected empty data got %v", response.Data)
	}
}

// Test /api/jobs when user header found
func TestJobsHandlerWithUserHeader(t *testing.T) {
	server := setupServer()
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	// Add user header
	req.Header.Set("X-Grafana-User", "foo")

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
	expectedJobs, _ := getMockJobs("foo", []string{"foo", "bar"}, "", "", logger)

	// Unmarshal byte into structs.
	var response JobsResponse
	json.Unmarshal(data, &response)

	if response.Status != "success" {
		t.Errorf("expected success status got %v", response.Status)
	}
	if !reflect.DeepEqual(response.Data, expectedJobs) {
		t.Errorf("expected %v got %v", expectedJobs, response.Data)
	}
}
