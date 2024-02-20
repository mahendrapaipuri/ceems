package db

import (
	"database/sql"
	"embed"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
)

// Directory containing DB migrations
const testMigrationsDir = "test_migrations"

//go:embed test_migrations/*.sql
var testMigrationsFS embed.FS

func TestMigratorError(t *testing.T) {
	// Setup Migrator
	migrator, err := NewMigrator(testMigrationsFS, testMigrationsDir, log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to create Migrator instance: %s", err)
	}

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Errorf("failed to create SQLite3 DB instance: %s", err)
	}

	// Perform DB migrations
	if err = migrator.ApplyMigrations(db); err == nil {
		t.Errorf("expected DB migrations error")
	}
}
