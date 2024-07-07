package db

import (
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	statDBPath := filepath.Join(tmpDir, "stats.db")
	j := statsDB{
		logger: log.NewNopLogger(),
	}

	// Test setupDB function
	_, _, err := setupDB(statDBPath, j.logger)
	require.NoError(t, err)
	require.FileExists(t, statDBPath, "DB file not found")

	// Call setupDB again. This should return with db conn
	_, _, err = setupDB(statDBPath, j.logger)
	require.NoError(t, err, "failed to setup DB on already setup DB")

	// Check DB file exists
	assert.FileExists(t, statDBPath)
}
