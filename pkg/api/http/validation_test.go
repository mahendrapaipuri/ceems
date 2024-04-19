package http

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
)

// Same as the one in lb/frontend/middleware_test.go
func setupMockDB(d string) (*sql.DB, string) {
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

func TestVerifyOwnership(t *testing.T) {
	db, _ := setupMockDB(t.TempDir())

	tests := []struct {
		name   string
		uuids  []string
		user   string
		verify bool
	}{
		{
			name:   "forbid due to mismatch uuid",
			uuids:  []string{"1479765", "1481510"},
			user:   "usr1",
			verify: false,
		},
		{
			name:   "forbid due to missing project",
			uuids:  []string{"123", "345"},
			user:   "usr1",
			verify: false,
		},
		{
			name:   "forbid due to missing header",
			uuids:  []string{"123", "345"},
			user:   "",
			verify: false,
		},
		{
			name:   "pass due to correct uuid",
			uuids:  []string{"1479763"},
			user:   "usr1",
			verify: true,
		},
		{
			name:   "pass due to uuid from same project",
			uuids:  []string{"1481508"},
			user:   "usr1",
			verify: true,
		},
		{
			name:   "pass due to no uuid",
			uuids:  []string{},
			user:   "usr3",
			verify: true,
		},
	}

	for _, test := range tests {
		result := VerifyOwnership(test.user, test.uuids, db, log.NewNopLogger())

		if result != test.verify {
			t.Errorf("%s: expected %t, got %t", test.name, test.verify, result)
		}
	}
}
