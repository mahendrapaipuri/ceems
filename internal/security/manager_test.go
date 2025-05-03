package security

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/steiler/acls"
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

func testConfig(tmpDir string) (*Config, error) {
	// Make test directories
	testDir := filepath.Join(tmpDir, "l1", "l2", "l3")
	if err := os.MkdirAll(testDir, 0o700); err != nil {
		return nil, err
	}

	// Add rx on tmpDir/l1/l2
	if err := os.Chmod(filepath.Join(tmpDir, "l1", "l2"), 0o705); err != nil {
		return nil, err
	}

	// Create a file in testDir
	testReadFile := filepath.Join(testDir, "testRead")
	if err := os.WriteFile(testReadFile, []byte("hello"), 0o700); err != nil { //nolint:gosec
		return nil, err
	}

	testWriteFile := filepath.Join(testDir, "testWrite")
	if err := os.WriteFile(testWriteFile, []byte("hello"), 0o700); err != nil { //nolint:gosec
		return nil, err
	}

	return &Config{
		RunAsUser: "nobody",
		ReadPaths: []string{
			testReadFile,
			filepath.Join(tmpDir, "l1", "l2", "l3"),
			filepath.Join(tmpDir, "l1", "l2"),
			filepath.Join(tmpDir, "l1"),
			filepath.Dir(tmpDir),
			tmpDir,
		},
		ReadWritePaths: []string{
			testWriteFile,
		},
	}, nil
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()

	c, err := testConfig(tmpDir)
	require.NoError(t, err)

	m, err := NewManager(c)
	require.NoError(t, err)

	expectedEntries := []acl{
		{path: filepath.Join(tmpDir, "l1", "l2", "l3"), entry: acls.NewEntry(acls.TAG_ACL_USER, 65534, 5)},
		{path: filepath.Join(tmpDir, "l1"), entry: acls.NewEntry(acls.TAG_ACL_USER, 65534, 5)},
		{path: filepath.Dir(tmpDir), entry: acls.NewEntry(acls.TAG_ACL_USER, 65534, 5)},
		{path: filepath.Join(tmpDir, "l1", "l2", "l3", "testRead"), entry: acls.NewEntry(acls.TAG_ACL_USER, 65534, 4)},
		{path: filepath.Join(tmpDir, "l1", "l2", "l3", "testWrite"), entry: acls.NewEntry(acls.TAG_ACL_USER, 65534, 6)},
	}

	assert.ElementsMatch(t, expectedEntries, m.acls)

	// Test illegal runAsUser
	c.RunAsUser = "illegal"

	_, err = NewManager(c)
	require.Error(t, err)
}

func TestACLs(t *testing.T) {
	skipUnprivileged(t)

	tmpDir := t.TempDir()

	c, err := testConfig(tmpDir)
	require.NoError(t, err)

	readFile := filepath.Join(tmpDir, "l1", "l2", "l3", "testRead")
	writeFile := filepath.Join(tmpDir, "l1", "l2", "l3", "testWrite")

	m, err := NewManager(c)
	require.NoError(t, err)

	// Add ACL entries
	err = m.addACLEntries()
	require.NoError(t, err)

	// Now all paths should be reachable
	err = m.pathsReachable()
	require.NoError(t, err)

	// Attempt to read and write file
	_, err = osexec.ExecuteAs("cat", []string{readFile}, 65534, 65534, nil)
	require.NoError(t, err)

	_, err = osexec.ExecuteAs("touch", []string{writeFile}, 65534, 65534, nil)
	require.NoError(t, err)

	// Drop all ACLs
	err = m.DeleteACLEntries()
	require.NoError(t, err)

	// Now attempt to read and write files. Should be errors
	_, err = osexec.ExecuteAs("cat", []string{readFile}, 65534, 65534, nil)
	require.Error(t, err)

	_, err = osexec.ExecuteAs("touch", []string{writeFile}, 65534, 65534, nil)
	require.Error(t, err)
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
