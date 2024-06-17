package http

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
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
	"cluster_id" text,
	"uuid" text,
	"project" text,
	"usr" text
);
INSERT INTO units VALUES(1, 'rm-0', '1479763', 'prj1', 'usr1');
INSERT INTO units VALUES(2, 'rm-0', '1481508', 'prj1', 'usr2');
INSERT INTO units VALUES(3, 'rm-0', '1479765', 'prj2', 'usr2');
INSERT INTO units VALUES(4, 'rm-0', '1481510', 'prj3', 'usr3');
INSERT INTO units VALUES(5, 'rm-1', '1479763', 'prj1', 'usr1');
INSERT INTO units VALUES(6, 'rm-1', '1481508', 'prj1', 'usr2');
INSERT INTO units VALUES(7, 'rm-1', '1479765', 'prj4', 'usr4');
INSERT INTO units VALUES(8, 'rm-1', '1481510', 'prj5', 'usr5');
CREATE TABLE usage (
	"id" integer not null primary key,
	"cluster_id" text,
	"project" text,
	"usr" text
);
INSERT INTO usage VALUES(1, 'rm-0', 'prj1', 'usr1');
INSERT INTO usage VALUES(2, 'rm-0', 'prj1', 'usr2');
INSERT INTO usage VALUES(3, 'rm-0', 'prj2', 'usr2');
INSERT INTO usage VALUES(4, 'rm-0', 'prj3', 'usr3');
INSERT INTO usage VALUES(5, 'rm-1', 'prj1', 'usr1');
INSERT INTO usage VALUES(6, 'rm-1', 'prj1', 'usr2');
INSERT INTO usage VALUES(7, 'rm-1', 'prj4', 'usr4');
INSERT INTO usage VALUES(8, 'rm-1', 'prj5', 'usr5');
CREATE TABLE admin_users (
	"id" integer not null primary key,
	"source" text,
	"users" text
);
INSERT INTO admin_users VALUES(1, 'ceems', 'adm1|adm2|adm3');
INSERT INTO admin_users VALUES(2, 'grafana', 'adm4|adm5|adm6');
COMMIT;`

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
		rmID   string
		uuids  []string
		user   string
		verify bool
	}{
		{
			name:   "forbid due to mismatch uuid",
			uuids:  []string{"1479765", "1481510"},
			rmID:   "rm-0",
			user:   "usr1",
			verify: false,
		},
		{
			name:   "forbid due to missing cluster_id",
			uuids:  []string{"1481508", "1479765"},
			user:   "usr2",
			verify: false,
		},
		{
			name:   "forbid due to missing project",
			uuids:  []string{"123", "345"},
			rmID:   "rm-1",
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
			rmID:   "rm-0",
			user:   "usr1",
			verify: true,
		},
		{
			name:   "pass due to uuid from same project",
			uuids:  []string{"1481508"},
			rmID:   "rm-0",
			user:   "usr1",
			verify: true,
		},
		{
			name:   "pass due to admin query",
			uuids:  []string{"1481508"},
			rmID:   "rm-0",
			user:   "adm1",
			verify: true,
		},
		{
			name:   "pass due to no uuid",
			uuids:  []string{},
			rmID:   "rm-0",
			user:   "usr3",
			verify: true,
		},
	}

	for _, test := range tests {
		result := VerifyOwnership(test.user, []string{test.rmID}, test.uuids, db, log.NewNopLogger())

		if result != test.verify {
			t.Errorf("%s: expected %t, got %t", test.name, test.verify, result)
		}
	}
}

func TestAdminUsers(t *testing.T) {
	db, _ := setupMockDB(t.TempDir())

	// Expected users
	expectedUsers := []string{"adm1", "adm2", "adm3", "adm4", "adm5", "adm6"}

	users := adminUsers(db, log.NewNopLogger())
	if !reflect.DeepEqual(users, expectedUsers) {
		t.Errorf("expected users %v got %v", expectedUsers, users)
	}
}
