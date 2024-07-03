package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

type testCase struct {
	name    string
	req     string
	user    string
	admin   bool
	handler func(http.ResponseWriter, *http.Request)
	code    int
}

var (
	mockServerUnits = []models.Unit{
		{UUID: "1000", ClusterID: "slurm-0", ResourceManager: "slurm", User: "foousr"},
		{UUID: "10001", ClusterID: "os-0", ResourceManager: "openstack", User: "barusr"},
	}
	mockServerUsage = []models.Usage{
		{Project: "foo", ClusterID: "slurm-0", ResourceManager: "slurm"},
		{Project: "bar", ClusterID: "os-0", ResourceManager: "openstack"},
	}
	mockServerProjects = []models.Project{
		{Name: "foo", ClusterID: "slurm-0", ResourceManager: "slurm", Users: models.List{"foousr"}},
		{Name: "bar", ClusterID: "os-0", ResourceManager: "openstack", Users: models.List{"barusr"}},
	}
	mockServerUsers = []models.User{
		{Name: "foousr", ClusterID: "slurm-0", ResourceManager: "slurm", Projects: models.List{"foo"}},
		{Name: "bar", ClusterID: "os-0", ResourceManager: "openstack", Projects: models.List{"bar"}},
	}
	mockServerClusters = []models.Cluster{
		{ID: "slurm-0", Manager: "slurm"},
		{ID: "os-0", Manager: "openstack"},
	}
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
	return mockServerUnits, nil
}

func usageQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Usage, error) {
	return mockServerUsage, nil
}

func projectQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Project, error) {
	return mockServerProjects, nil
}

func userQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.User, error) {
	return mockServerUsers, nil
}

func clusterQuerier(db *sql.DB, q Query, logger log.Logger) ([]models.Cluster, error) {
	return mockServerClusters, nil
}

func getMockUnits(
	_ Query,
	_ log.Logger,
) ([]models.Unit, error) {
	return mockServerUnits, nil
}

// Test users and users admin handlers
func TestUsersHandlers(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "users",
			req:     "/api/" + base.APIVersion + "/users?field=uuid&field=project",
			user:    "foousr",
			admin:   false,
			handler: server.users,
			code:    200,
		},
		{
			name:    "users admin",
			req:     "/api/" + base.APIVersion + "/users/admin?project=foo",
			user:    "foousr",
			admin:   true,
			handler: server.usersAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Errorf("expected error to be nil got %v", err)
		}

		// Unmarshal byte into structs.
		var response Response[models.User]
		json.Unmarshal(data, &response)
		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
		if response.Status != "success" {
			t.Errorf("%s: expected success status got %v", test.name, response.Status)
		}
		if !reflect.DeepEqual(response.Data, mockServerUsers) {
			t.Errorf("%s: expected data %#v got %#v", test.name, mockServerUsers, response.Data)
		}
	}
}

// Test projects and projects admin handlers
func TestProjectsHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "projects",
			req:     "/api/" + base.APIVersion + "/projects",
			user:    "foousr",
			admin:   false,
			handler: server.projects,
			code:    200,
		},
		{
			name:    "projects admin",
			req:     "/api/" + base.APIVersion + "/projects/admin",
			user:    "foousr",
			admin:   true,
			handler: server.projectsAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("project", "foo")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Errorf("expected error to be nil got %v", err)
		}

		// Unmarshal byte into structs.
		var response Response[models.Project]
		json.Unmarshal(data, &response)
		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
		if response.Status != "success" {
			t.Errorf("%s: expected success status got %v", test.name, response.Status)
		}
		if !reflect.DeepEqual(response.Data, mockServerProjects) {
			t.Errorf("%s: expected data %#v got %#v", test.name, mockServerProjects, response.Data)
		}
	}
}

// Test units and units admin handlers
func TestUnitsHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "units",
			req:     "/api/" + base.APIVersion + "/units",
			user:    "foousr",
			admin:   false,
			handler: server.units,
			code:    200,
		},
		{
			name:    "units admin",
			req:     "/api/" + base.APIVersion + "/units/admin",
			user:    "foousr",
			admin:   true,
			handler: server.unitsAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Errorf("expected error to be nil got %v", err)
		}

		// Unmarshal byte into structs.
		var response Response[models.Unit]
		json.Unmarshal(data, &response)
		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
		if response.Status != "success" {
			t.Errorf("%s: expected success status got %v", test.name, response.Status)
		}
		if !reflect.DeepEqual(response.Data, mockServerUnits) {
			t.Errorf("%s: expected data %#v got %#v", test.name, mockServerUnits, response.Data)
		}
	}
}

// Test usage and usage admin handlers
func TestUsageHandlers(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "current usage",
			req:     "/api/" + base.APIVersion + "/usage/current",
			user:    "foousr",
			admin:   false,
			handler: server.usage,
			code:    200,
		},
		{
			name:    "global usage",
			req:     "/api/" + base.APIVersion + "/usage/global",
			user:    "foousr",
			admin:   false,
			handler: server.usage,
			code:    200,
		},
		{
			name:    "current usage admin",
			req:     "/api/" + base.APIVersion + "/usage/current/admin",
			user:    "foousr",
			admin:   true,
			handler: server.usageAdmin,
			code:    200,
		},
		{
			name:    "global usage admin",
			req:     "/api/" + base.APIVersion + "/usage/global/admin",
			user:    "foousr",
			admin:   true,
			handler: server.usageAdmin,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if test.admin {
			q := url.Values{}
			q.Add("user", "foousr")
			request.URL.RawQuery = q.Encode()
		}
		if strings.Contains(test.name, "current") {
			request = mux.SetURLVars(request, map[string]string{"mode": "current"})
		} else {
			request = mux.SetURLVars(request, map[string]string{"mode": "global"})
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		// Get body
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Errorf("expected error to be nil got %v", err)
		}

		// Unmarshal byte into structs.
		var response Response[models.Usage]
		json.Unmarshal(data, &response)
		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
		if response.Status != "success" {
			t.Errorf("%s: expected success status got %v", test.name, response.Status)
		}
		if !reflect.DeepEqual(response.Data, mockServerUsage) {
			t.Errorf("%s: expected data %#v got %#v", test.name, mockServerUsage, response.Data)
		}
	}
}

// Test verify handler
func TestVerifyHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	tests := []testCase{
		{
			name:    "verify bad data",
			req:     "/api/" + base.APIVersion + "/units/verify",
			user:    "foousr",
			admin:   false,
			handler: server.verifyUnitsOwnership,
			code:    400,
		},
		{
			name:    "verify forbidden",
			req:     "/api/" + base.APIVersion + "/units/verify?uuid=1234",
			user:    "foousr",
			admin:   false,
			handler: server.verifyUnitsOwnership,
			code:    403,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
	}
}

// Test demo handlers
func TestDemoHandlers(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Test cases
	tests := []testCase{
		{
			name:    "units demo",
			req:     "/api/" + base.APIVersion + "/demo/units",
			user:    "foousr",
			admin:   false,
			handler: server.demo,
			code:    200,
		},
		{
			name:    "usage demo",
			req:     "/api/" + base.APIVersion + "/demo/usage",
			user:    "foousr",
			admin:   false,
			handler: server.demo,
			code:    200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		request.Header.Set("X-Grafana-User", test.user)
		if strings.Contains(test.name, "units") {
			request = mux.SetURLVars(request, map[string]string{"resource": "units"})
		} else {
			request = mux.SetURLVars(request, map[string]string{"resource": "usage"})
		}

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)
		res := w.Result()
		defer res.Body.Close()

		if w.Code != test.code {
			t.Errorf("%s: expected status code %d, got %d", test.name, test.code, w.Code)
		}
	}
}

// Test clusters handlers
func TestClustersHandler(t *testing.T) {
	server := setupServer()
	defer server.Shutdown(context.Background())

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/"+base.APIVersion+"/clusters/admin", nil)
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

// // Test /usage
// func TestUsageHandler(t *testing.T) {
// 	server := setupServer()
// 	defer server.Shutdown(context.Background())

// 	// Create request
// 	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/current", nil)
// 	// Need to set path variables here
// 	req = mux.SetURLVars(req, map[string]string{"mode": "current"})

// 	// Add user header
// 	currentUser := "foo"
// 	req.Header.Set("X-Grafana-User", currentUser)

// 	// Start recorder
// 	w := httptest.NewRecorder()
// 	server.usage(w, req)
// 	res := w.Result()
// 	defer res.Body.Close()

// 	// Get body
// 	data, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		t.Errorf("expected error to be nil got %v", err)
// 	}

// 	// Expected result
// 	expectedUsage, _ := usageQuerier(server.db, Query{}, server.logger)

// 	// Unmarshal byte into structs.
// 	var response Response[models.Usage]
// 	json.Unmarshal(data, &response)

// 	if response.Status != "success" {
// 		t.Errorf("expected success status got %#v", response)
// 	}

// 	if !reflect.DeepEqual(expectedUsage, response.Data) {
// 		t.Errorf("expected usage %#v usage, got %#v", expectedUsage, response.Data)
// 	}
// }
