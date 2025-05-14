package migrator

import (
	"database/sql"
	"embed"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Directory containing DB migrations.
const testMigrationsDir = "test_migrations"

//go:embed test_migrations/*/*.sql
var testMigrationsFS embed.FS

var noOpLogger = slog.New(slog.DiscardHandler)

func TestMigratorSuccess(t *testing.T) {
	// Setup Migrator
	migrator, err := New(testMigrationsFS, filepath.Join(testMigrationsDir, "pass"), noOpLogger)
	require.NoError(t, err, "failed to create migrator")

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to open DB")

	// Perform DB migrations
	err = migrator.ApplyMigrations(db)
	assert.NoError(t, err)
}

func TestMigratorFail(t *testing.T) {
	// Setup Migrator
	migrator, err := New(testMigrationsFS, filepath.Join(testMigrationsDir, "fail"), noOpLogger)
	require.NoError(t, err, "failed to create migrator")

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to open DB")

	// Perform DB migrations
	err = migrator.ApplyMigrations(db)
	assert.Error(t, err)
}
