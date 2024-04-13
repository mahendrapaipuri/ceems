package frontend

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
)

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

func setupMiddleware(tmpDir string) http.Handler {
	// Setup test DB
	db, _ := setupTestDB(tmpDir)

	// Create an instance of middleware
	amw := authenticationMiddleware{
		logger:     log.NewNopLogger(),
		adminUsers: []string{"adm1"},
		db:         db,
		grafana:    &grafana.Grafana{},
	}

	// create a handler to use as "next" which will verify the request
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// create the handler to test, using our custom "next" handler
	return amw.Middleware(nextHandler)
}

func TestMiddleware(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware(t.TempDir())

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
			code:   401,
		},
		{
			name:   "allow query for admins",
			req:    "/test?query=foo{uuid=~\"1479765|1481510\"}",
			user:   "adm1",
			header: true,
			code:   200,
		},
		{
			name:   "forbid due to missing project",
			req:    "/test?query=foo{uuid=~\"123|345\"}",
			user:   "usr1",
			header: true,
			code:   401,
		},
		{
			name:   "forbid due to missing header",
			req:    "/test?query=foo{uuid=~\"123|345\"}",
			header: false,
			code:   401,
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

		handlerToTest.ServeHTTP(responseRecorder, request)

		if responseRecorder.Result().StatusCode != test.code {
			t.Errorf("%s: expected status %d, got %d", test.name, test.code, responseRecorder.Result().StatusCode)
		}
	}

}
