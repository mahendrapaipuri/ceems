package security

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipUnprivileged(t *testing.T) {
	t.Helper()

	// Get current user
	currentUser, err := user.Current()
	require.NoError(t, err)

	if currentUser.Uid != "0" {
		t.Skip("Skipping testing due to lack of privileges")
	}
}

// func TestDropPrivileges(t *testing.T) {
// 	skipUnprivileged(t)

// 	// Target a cap
// 	value, err := cap.FromName("cap_sys_admin")
// 	require.NoError(t, err)

// 	// Make test config
// 	// We are running as root as using any other user
// 	// will make the process owner running that test
// 	// as that user and go wont be able to create
// 	// build and coverage related files anymore after
// 	// test finishes.
// 	cfg := Config{
// 		RunAsUser: "root",
// 		Caps:      []cap.Value{value},
// 	}

// 	// Drop all privileges
// 	err = DropPrivileges(&cfg)
// 	require.NoError(t, err)

// 	// Check process do not have any privileges
// 	capName := cap.GetProc().String()

// 	require.NoError(t, err)
// 	assert.EqualValues(t, "cap_sys_admin=p", capName)

// 	// Get current caps
// 	current := cap.GetProc()

// 	// Setback current caps
// 	err = current.SetFlag(cap.Effective, true, value)
// 	require.NoError(t, err)

// 	err = current.SetProc()
// 	require.NoError(t, err)
// }

func TestChangeOwnership(t *testing.T) {
	skipUnprivileged(t)

	// Ensure parent of tmpDir has x permissions for others.
	// Seems like tmpDirs are created by default with 0700
	tmpDir := t.TempDir()
	tmpDirParent := filepath.Dir(tmpDir)
	err := os.Chmod(tmpDirParent, 0o755)
	require.NoError(t, err)

	// Ensure tmpDirParent has 0755
	info, err := os.Stat(tmpDirParent)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), info.Mode().Perm())

	// Make dummy read and readwrite paths
	readPath := filepath.Join(tmpDir, "ro")
	err = os.WriteFile(readPath, []byte("dummy"), 0o600)
	require.NoError(t, err)

	readWritePath := filepath.Join(tmpDir, "rw")
	err = os.WriteFile(readWritePath, []byte("dummy"), 0o600)
	require.NoError(t, err)

	// Make test config
	cfg := Config{
		RunAsUser:      "root",
		ReadPaths:      []string{readPath},
		ReadWritePaths: []string{readWritePath},
	}

	// change ownership
	err = changeOwnership(&cfg)
	require.NoError(t, err)

	// Ensure paths are reachable
	err = pathsReachable(&cfg)
	require.NoError(t, err)

	// Ensure readPath has root as user
	info, err = os.Stat(readPath)
	require.NoError(t, err)

	stat, ok := info.Sys().(*syscall.Stat_t)
	assert.True(t, ok)
	assert.EqualValues(t, 0, stat.Uid)

	// Ensure readWritePath has root as user
	info, err = os.Stat(readWritePath)
	require.NoError(t, err)

	stat, ok = info.Sys().(*syscall.Stat_t)
	assert.True(t, ok)
	assert.EqualValues(t, 0, stat.Uid)

	// Ensure user has write permissions
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}
