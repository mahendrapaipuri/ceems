//go:build cgo
// +build cgo

package db

import (
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	statDBPath := filepath.Join(tmpDir, "stats.db")

	// Test setupDB function
	_, _, err := setupDB(statDBPath)
	require.NoError(t, err)
	require.FileExists(t, statDBPath, "DB file not found")

	// Call setupDB again. This should return with db conn
	_, _, err = setupDB(statDBPath)
	require.NoError(t, err, "failed to setup DB on already setup DB")

	// Check DB file exists
	assert.FileExists(t, statDBPath)
}
