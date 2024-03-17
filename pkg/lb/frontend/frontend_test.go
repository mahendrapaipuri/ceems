package frontend

import (
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
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

func dummyTSDBServer(retention string) *httptest.Server {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"storageRetention": retention,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			w.Write([]byte("dummy-response"))
		}
	}))
	return server
}

func setupTestDB(d string) (*sql.DB, string) {
	dbPath := filepath.Join(d, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("failed to create DB")
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE units (
	"id" integer not null primary key,
	"uuid" text,
	"project" text,
	"usr" text
);
INSERT INTO units VALUES(1,'1479763', 'prj1', 'usr1');
INSERT INTO units VALUES(2,'1481508', 'prj1', 'usr2');
INSERT INTO units VALUES(3,'1479765', 'prj2', 'usr2');
INSERT INTO units VALUES(4,'1481510', 'prj3', 'usr3');
CREATE TABLE usage (
	"id" integer not null primary key,
	"project" text,
	"usr" text
);
INSERT INTO usage VALUES(1, 'prj1', 'usr1');
INSERT INTO usage VALUES(2, 'prj1', 'usr2');
INSERT INTO usage VALUES(3, 'prj2', 'usr2');
INSERT INTO usage VALUES(4, 'prj3', 'usr3');
COMMIT;
	`
	_, err = db.Exec(stmts)
	if err != nil {
		fmt.Printf("failed to insert mock data into DB: %s", err)
	}
	return db, dbPath
}

func TestNewFrontend(t *testing.T) {
	// Backends
	dummyServer1 := dummyTSDBServer("30d")
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	if err != nil {
		t.Fatal(err)
	}

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.NewTSDBServer(backend1URL, rp1, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.NewManager("resource-based", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	manager.Add(backend1)

	// make minimal config
	config := &Config{
		Logger:  log.NewNopLogger(),
		Manager: manager,
	}

	// New load balancer
	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Errorf("failed to create load balancer: %s", err)
	}

	tests := []struct {
		name     string
		req      string
		header   bool
		code     int
		response bool
	}{
		{
			name:     "pass with query",
			req:      "/test?query=foo{uuid=\"1479765|1481510\"}",
			header:   true,
			code:     200,
			response: true,
		},
		{
			name: "pass with start and query params",
			req: fmt.Sprintf(
				"/test?query=foo{uuid=\"123|345\"}&start=%d",
				time.Now().UTC().Add(-time.Duration(29*24*time.Hour)).Unix(),
			),
			header:   false,
			code:     200,
			response: true,
		},
		{
			name: "no target with start more than retention period",
			req: fmt.Sprintf(
				"/test?query=foo{uuid=\"123|345\"}&start=%d",
				time.Now().UTC().Add(-time.Duration(31*24*time.Hour)).Unix(),
			),
			header:   false,
			code:     503,
			response: false,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		if test.header {
			request.Header.Set("X-Grafana-User", "usr1")
		}
		responseRecorder := httptest.NewRecorder()

		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != test.code {
			t.Errorf("%s: expected status %d, got %d", test.name, test.code, responseRecorder.Code)
		}
		if test.response {
			if strings.TrimSpace(responseRecorder.Body.String()) != "dummy-response" {
				t.Errorf("%s: expected dummy-response, got %s", test.name, responseRecorder.Body)
			}
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)
	request := httptest.NewRequest("GET", "/test", nil)
	responseRecorder := httptest.NewRecorder()

	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != 503 {
		t.Errorf("expected status 503, got %d", responseRecorder.Code)
	}
}

func TestNewFrontendUUIDCheck(t *testing.T) {
	// Setup test DB
	_, dbPath := setupTestDB(t.TempDir())

	// Backends
	dummyServer1 := dummyTSDBServer("30d")
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	if err != nil {
		t.Fatal(err)
	}

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.NewTSDBServer(backend1URL, rp1, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.NewManager("resource-based", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	manager.Add(backend1)

	// make minimal config
	config := &Config{
		Logger:  log.NewNopLogger(),
		DBPath:  dbPath,
		Manager: manager,
	}

	// New load balancer
	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Errorf("failed to create load balancer: %s", err)
	}

	tests := []struct {
		name   string
		req    string
		user   string
		header bool
		code   int
	}{
		{
			name:   "forbid due to mismatch uuid",
			req:    "/test?query=foo{uuid=~\"1479765|1481510\"}",
			user:   "usr1",
			header: true,
			code:   400,
		},
		{
			name:   "pass due to missing project",
			req:    "/test?query=foo{uuid=~\"123|345\"}",
			header: false,
			code:   200,
		},
		{
			name:   "pass due to missing header",
			req:    "/test?query=foo{uuid=~\"123|345\"}",
			header: false,
			code:   200,
		},
		{
			name:   "pass due to correct uuid",
			req:    "/test?query=foo{uuid=\"1479763\"}",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "pass due to correct uuid with gpuuuid in query",
			req:    "/test?query=foo{uuid=\"1479763\",gpuuuid=\"GPU-01234\"}",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "pass due to uuid from same project",
			req:    "/test?query=foo{uuid=\"1481508\"}",
			user:   "usr1",
			header: true,
			code:   200,
		},
		{
			name:   "pass due to no uuid",
			req:    "/test?query=foo{uuid=\"\"}",
			header: true,
			user:   "usr3",
			code:   200,
		},
		{
			name:   "pass due to no uuid and non-empty gpuuuid",
			req:    "/test?query=foo{uuid=\"\",gpuuuid=\"GPU-01234\"}",
			header: true,
			user:   "usr2",
			code:   200,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", test.req, nil)
		if test.header {
			request.Header.Set("X-Grafana-User", test.user)
		}
		responseRecorder := httptest.NewRecorder()

		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != test.code {
			t.Errorf("%s: expected status %d, got %d", test.name, test.code, responseRecorder.Code)
		}
	}
}
