package http

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Same as the one in lb/frontend/middleware_test.go.
func setupMockDB(d string) (*sql.DB, error) {
	dbPath := filepath.Join(d, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB: %w", err)
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE units (
	"id" integer not null primary key,
	"cluster_id" text,
	"uuid" text,
	"project" text,
	"usr" text,
	"started_at_ts" int
);
INSERT INTO units VALUES(1, 'rm-0', '1479763', 'prj1', 'usr1', 1735045414000);
INSERT INTO units VALUES(2, 'rm-0', '1481508', 'prj1', 'usr2', 1735045414000);
INSERT INTO units VALUES(3, 'rm-0', '1479765', 'prj2', 'usr2', 1735045414000);
INSERT INTO units VALUES(4, 'rm-0', '1481510', 'prj3', 'usr3', 1735045414000);
INSERT INTO units VALUES(5, 'rm-0', '1481508', 'prj3', 'usr3', 1703419414000);
INSERT INTO units VALUES(6, 'rm-1', '1479763', 'prj1', 'usr1', 1735045414000);
INSERT INTO units VALUES(7, 'rm-1', '1481508', 'prj1', 'usr2', 1735045414000);
INSERT INTO units VALUES(8, 'rm-1', '1479765', 'prj4', 'usr4', 1735045414000);
INSERT INTO units VALUES(9, 'rm-1', '1481510', 'prj5', 'usr5', 1735045414000);
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
CREATE TABLE projects (
	"id" integer not null primary key,
	"cluster_id" text,
	"name" text,
	"users" text
);
INSERT INTO projects VALUES(1, 'rm-0', 'prj1', '["usr1","usr2"]');
INSERT INTO projects VALUES(2, 'rm-0', 'prj2', '["usr2"]');
INSERT INTO projects VALUES(3, 'rm-0', 'prj3', '["usr3"]');
INSERT INTO projects VALUES(4, 'rm-1', 'prj1', '["usr1","usr2"]');
INSERT INTO projects VALUES(5, 'rm-1', 'prj4', '["usr4"]');
INSERT INTO projects VALUES(6, 'rm-1', 'prj5', '["usr5"]');
CREATE TABLE users (
	"id" integer not null primary key,
	"cluster_id" text,
	"name" text,
	"projects" text
);
INSERT INTO users VALUES(1, 'rm-0', 'usr1', '["prj1"]');
INSERT INTO users VALUES(2, 'rm-0', 'usr2', '["prj1","prj2"]');
INSERT INTO users VALUES(3, 'rm-0', 'usr3', '["prj3"]');
INSERT INTO users VALUES(4, 'rm-1', 'usr1', '["prj1"]');
INSERT INTO users VALUES(5, 'rm-1', 'usr2', '["prj1"]');
INSERT INTO users VALUES(6, 'rm-1', 'usr4', '["prj4"]');
INSERT INTO users VALUES(7, 'rm-1', 'usr5', '["prj5"]');
CREATE TABLE admin_users (
	"id" integer not null primary key,
	"source" text,
	"users" text
);
INSERT INTO admin_users VALUES(1, 'ceems', '["adm1","adm2","adm3"]');
INSERT INTO admin_users VALUES(2, 'grafana', '["adm4","adm5","adm6"]');
COMMIT;`

	_, err = db.Exec(stmts)
	if err != nil {
		return nil, fmt.Errorf("failed to insert mock data into DB: %w", err)
	}

	return db, nil
}

func TestVerifyOwnership(t *testing.T) {
	db, err := setupMockDB(t.TempDir())
	require.NoError(t, err, "failed to setup test DB")

	tests := []struct {
		name   string
		rmID   string
		uuids  []string
		starts []int64
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
			name:   "forbid due to incorrect start",
			uuids:  []string{"1481508"},
			user:   "usr2",
			starts: []int64{1703419414000},
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
			name:   "pass with correct uuid and start",
			uuids:  []string{"1481508"},
			rmID:   "rm-0",
			user:   "usr2",
			starts: []int64{1735045414000},
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
			verify: false,
		},
	}

	for _, test := range tests {
		result := VerifyOwnership(
			context.Background(),
			test.user,
			[]string{test.rmID},
			test.uuids,
			test.starts,
			db,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)
		assert.Equal(t, test.verify, result, test.name)
	}
}

func TestAdminUsers(t *testing.T) {
	db, err := setupMockDB(t.TempDir())
	require.NoError(t, err, "failed to setup test DB")

	// Expected users
	expectedUsers := []string{"adm1", "adm2", "adm3", "adm4", "adm5", "adm6"}

	users := adminUsers(context.Background(), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Equal(t, expectedUsers, users)
}
