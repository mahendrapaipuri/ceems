package db

import (
	"database/sql"
	"embed"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Directory containing DB migrations
const testMigrationsDir = "test_migrations"

//go:embed test_migrations/*.sql
var testMigrationsFS embed.FS

func TestMigratorError(t *testing.T) {
	// Setup Migrator
	migrator, err := NewMigrator(testMigrationsFS, testMigrationsDir, log.NewNopLogger())
	require.NoError(t, err, "failed to create migrator")

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to open DB")

	// Perform DB migrations
	err = migrator.ApplyMigrations(db)
	assert.Error(t, err, "expected DB migrations error")
}
