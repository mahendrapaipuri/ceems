// Package security implements privilege management and execution of
// privileged actions in security contexts.
package security

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"slices"
	"strconv"
	"syscall"

	"github.com/steiler/acls"
	"github.com/wneessen/go-fileperm"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

const (
	deleteACLCtx = "delete_acl_entries"
)

type deleteACLEntriesCtxData struct {
	acls []acl
}

type Config struct {
	RunAsUser      string      // Change to this user if app is started as root
	Caps           []cap.Value // Capabilities necessary for the app
	ReadPaths      []string    // Paths that "RunAsUser" user able to read
	ReadWritePaths []string    // Paths that "RunAsUser" user able to read/write
}

type acl struct {
	path  string
	entry *acls.ACLEntry
}

// Manager implements security manager.
type Manager struct {
	logger           *slog.Logger
	runAsUser        *user.User
	caps             []cap.Value
	acls             []acl
	securityContexts map[string]*SecurityContext
}

// NewManager returns a new instance of security manager.
func NewManager(c *Config, logger *slog.Logger) (*Manager, error) {
	var err, errs error

	// Start a new instance of manager
	manager := &Manager{
		logger: logger,
		caps:   c.Caps,
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Get the user struct for the run as user
	// First attempt to lookup by username and then by uid
	manager.runAsUser, err = user.Lookup(c.RunAsUser)
	if err != nil {
		errs = errors.Join(errs, err)

		if manager.runAsUser, err = user.LookupId(c.RunAsUser); err != nil {
			errs = errors.Join(errs, err)

			return nil, fmt.Errorf("could not lookup %s: %w", c.RunAsUser, errs)
		}
	}

	// Convert run as user UID to uint32
	val, err := strconv.ParseUint(manager.runAsUser.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to convert user UID to uint32: %w", err)
	}

	// val will be always 64 bit but we can convert it to 32 bit without any checks as
	// we asked for 32 bits while conversion
	runAsUserUID := uint32(val)

	// Calculate ACL entries for different paths
	for _, path := range c.ReadPaths {
		if path == "" {
			continue
		}

		// First check if path details and permissions
		fperms, err := fileperm.New(path)
		if err != nil {
			return nil, fmt.Errorf("failed to path permissions: %w", err)
		}

		var perms uint16

		var hasPerms bool

		// Based on file type set permission bit
		// For directories add rx and for regular files
		// add only r
		switch mode := fperms.Stat.Mode(); {
		case mode.IsDir():
			perms = 5
			hasPerms = hasReadExecutable(fperms, currentUser, manager.runAsUser)
		case mode.IsRegular():
			perms = 4
			hasPerms = hasRead(fperms, currentUser, manager.runAsUser)
		}

		// If the path is readable/executable by runAsUser, nothing to do here. Continue
		if hasPerms {
			continue
		}

		entry := acls.NewEntry(acls.TAG_ACL_USER, runAsUserUID, perms)
		manager.acls = append(manager.acls, acl{path: path, entry: entry})
	}

	// Now handle read write paths
	for _, path := range c.ReadWritePaths {
		if path == "" {
			continue
		}

		// First check if path details and permissions
		fperms, err := fileperm.New(path)
		if err != nil {
			return nil, fmt.Errorf("failed to path permissions: %w", err)
		}

		var perms uint16

		var hasPerms bool

		// Based on file type set permission bit
		// For directories add rwx and for regular files
		// add only rw
		switch mode := fperms.Stat.Mode(); {
		case mode.IsDir():
			perms = 7
			hasPerms = hasReadWriteExecutable(fperms, currentUser, manager.runAsUser)
		case mode.IsRegular():
			perms = 6
			hasPerms = hasReadWrite(fperms, currentUser, manager.runAsUser)
		}

		// If the path is readable/executable by runAsUser, nothing to do here. Continue
		if hasPerms {
			continue
		}

		entry := acls.NewEntry(acls.TAG_ACL_USER, runAsUserUID, perms)
		manager.acls = append(manager.acls, acl{path: path, entry: entry})
	}

	// If there is at least one ACL, we need to setup a security context
	// with CAP_FOWNER to remove the ACLs before shutting down an app
	if len(manager.acls) > 0 {
		manager.securityContexts = make(map[string]*SecurityContext)

		// If there is no FOWNER cap already in caps, add it
		if !slices.Contains(manager.caps, cap.FOWNER) {
			manager.caps = append(manager.caps, cap.FOWNER)
		}

		// Create a new security context
		cfg := &SCConfig{
			Name:   deleteACLCtx,
			Caps:   []cap.Value{cap.FOWNER},
			Func:   deleteACLEntries,
			Logger: logger,
		}

		securityCtx, err := NewSecurityContext(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to setup security context: %w", err)
		}

		manager.securityContexts[deleteACLCtx] = securityCtx
	}

	return manager, nil
}

// DropPrivileges will change `root` user to run as user and drop any
// unnecessary privileges only keeping the ones passed in `caps` argument.
// If current user is not root, this function is no-op and we expect
// either process or file to have necessary capabilities in the production
// environments.
func (m *Manager) DropPrivileges(enableEffective bool) error {
	if syscall.Geteuid() != 0 {
		existing := cap.GetProc()

		// Get if the current process has any capabilities at all
		// by comparing against a new capability set
		// If no capabilities found, nothing to do, return
		if isPriv, err := existing.Cf(cap.NewSet()); err == nil && isPriv == 0 {
			return nil
		}

		// If there are capabilities, ensure we raise only permitted set
		// and clear effective set
		return setCapabilities(m.caps, enableEffective)
	}

	// Here we set a bunch of linux specific security stuff.
	// Add ACL entries to all relevant paths
	if err := m.addACLEntries(); err != nil {
		return err
	}

	// Now change the user from root to runAsUser
	if err := m.changeUser(); err != nil {
		return err
	}

	// Ensure ReadPaths and ReadWritePaths are accessible for runAsUser.
	// This can happen when any of the parent directories do not have rx
	// on others which might prevent runAsUser to access these paths.
	if err := m.pathsReachable(); err != nil {
		return err
	}

	// Attempt to set capabilities before we setup seccomp rules
	// Note that we discard any errors because they are not actionable.
	// The beat should use `getcap` at a later point to examine available capabilities
	// rather than relying on errors from `setcap`
	return setCapabilities(m.caps, enableEffective)
}

// DeleteACLEntries removes any ACL added entries.
func (m *Manager) DeleteACLEntries() error {
	// If there are no ACLs, nothing to remove here
	if len(m.acls) == 0 {
		return nil
	}

	// Setup data pointer to be passed into security context
	dataPtr := &deleteACLEntriesCtxData{
		acls: m.acls,
	}

	if securityCtx, ok := m.securityContexts[deleteACLCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			return fmt.Errorf("failed to remove ACLs in a security context: %w", err)
		}
	} else {
		return fmt.Errorf("no security context found to remove ACLs: %w", ErrNoSecurityCtx)
	}

	return nil
}

// addACLEntries adds ACL entries to paths.
func (m *Manager) addACLEntries() error {
	// Add ACL entries
	for _, acl := range m.acls {
		a := &acls.ACL{}

		// Load the existing ACL entries of the PosixACLAccess type
		if err := a.Load(acl.path, acls.PosixACLAccess); err != nil {
			return fmt.Errorf("failed to load acl entries: %w", err)
		}

		// Add entry to new ACL
		if err := a.AddEntry(acl.entry); err != nil {
			return fmt.Errorf("failed to add acl entry %s err: %w", acl.entry, err)
		}

		// Apply entry to new ACL
		if err := a.Apply(acl.path, acls.PosixACLAccess); err != nil {
			return fmt.Errorf("failed to apply acl entries %s to path %s err: %w", a, acl.path, err)
		}

		m.logger.Debug("ACL applied", "path", acl.path, "acl", acl.entry)
	}

	return nil
}

// changeUser switches the current user to run as user.
func (m *Manager) changeUser() error {
	localUserUID, err := strconv.Atoi(m.runAsUser.Uid)
	if err != nil {
		return fmt.Errorf("could not parse UID %s as int: %w", m.runAsUser.Uid, err)
	}

	localUserGID, err := strconv.Atoi(m.runAsUser.Gid)
	if err != nil {
		return fmt.Errorf("could not parse GID %s as int: %w", m.runAsUser.Uid, err)
	}

	// Set the main group as localUserUid so new files created are owned by the user's group
	err = syscall.Setgid(localUserGID)
	if err != nil {
		return fmt.Errorf("could not set gid to %d: %w", localUserGID, err)
	}

	// Note this is not the regular SetUID! Look at the 'cap' package docs for it, it preserves
	// capabilities post-SetUID, which we use to lock things down immediately
	err = cap.SetUID(localUserUID)
	if err != nil {
		return fmt.Errorf("could not setuid to %d: %w", localUserUID, err)
	}

	m.logger.Debug("Current user changed after dropping privileges", "username", m.runAsUser.Name)

	// This may not be necessary, but is good hygiene
	return os.Setenv("HOME", m.runAsUser.HomeDir)
}

// pathsReachable tests if all the relevant paths are reachable for runAsUser.
func (m *Manager) pathsReachable() error {
	// Stat path to check if they are reachable
	for _, a := range m.acls {
		if _, err := os.Stat(a.path); err != nil {
			return fmt.Errorf("could not reach path %s after changing user to %s", a.path, m.runAsUser.Username)
		}
	}

	return nil
}

// DropCapabilities drops any existing capabilities on the process.
func DropCapabilities() error {
	return setCapabilities(nil, false)
}

// setCapabilities sets the specific list of Linux capabilities on current process.
// It only add the capabilities to `permitted` set and it is responsible of the
// functions that need privileges to enable `effective` set before perfoming
// privileged action and then dropping them off straight after.
func setCapabilities(caps []cap.Value, enableEffective bool) error {
	// Start with an empty capability set
	newcaps := cap.NewSet()

	// Permitted makes the permission possible to get, effective makes it 'active'
	for _, c := range caps {
		if err := newcaps.SetFlag(cap.Permitted, true, c); err != nil {
			return fmt.Errorf("error setting permitted setcap: %w", err)
		}

		// Only enable effective set before performing a privileged operation
		if err := newcaps.SetFlag(cap.Effective, enableEffective, c); err != nil {
			return fmt.Errorf("error setting effective setcap: %w", err)
		}

		// We do not want these capabilities to be inherited by subprocesses
		if err := newcaps.SetFlag(cap.Inheritable, false, c); err != nil {
			return fmt.Errorf("error setting inheritable setcap: %w", err)
		}
	}

	// Apply the new capabilities to the current process (incl. all threads)
	if err := newcaps.SetProc(); err != nil {
		return fmt.Errorf("error setting new process capabilities via setcap: %w", err)
	}

	return nil
}

// hasRead returns true if runAsUser has r permissions on path.
func hasRead(p fileperm.PermUser, currentUser *user.User, runAsUser *user.User) bool {
	// If current user is runAsUser, check for user permissions
	if currentUser.Uid == runAsUser.Uid {
		return p.UserReadable()
	}

	// If not check, check for other permissions
	if p.Stat.Mode().Perm()&fileperm.OsOthR != 0 {
		return true
	}

	return false
}

// hasReadExecutable returns true if runAsUser has rx permissions on path.
func hasReadExecutable(p fileperm.PermUser, currentUser *user.User, runAsUser *user.User) bool {
	// If current user is runAsUser, check for user permissions
	if currentUser.Uid == runAsUser.Uid {
		return p.UserReadExecutable()
	}

	// If not check, check for other permissions
	if p.Stat.Mode().Perm()&fileperm.OsOthR != 0 && p.Stat.Mode().Perm()&fileperm.OsOthX != 0 {
		return true
	}

	return false
}

// hasReadWrite returns true if runAsUser has rw permissions on path.
func hasReadWrite(p fileperm.PermUser, currentUser *user.User, runAsUser *user.User) bool {
	// If current user is runAsUser, check for user permissions
	if currentUser.Uid == runAsUser.Uid {
		return p.UserWriteReadable()
	}

	// If not check, check for other permissions
	if p.Stat.Mode().Perm()&fileperm.OsOthR != 0 && p.Stat.Mode().Perm()&fileperm.OsOthW != 0 {
		return true
	}

	return false
}

// hasReadWriteExecutable returns true if runAsUser has rwx permissions on path.
func hasReadWriteExecutable(p fileperm.PermUser, currentUser *user.User, runAsUser *user.User) bool {
	// If current user is runAsUser, check for user permissions
	if currentUser.Uid == runAsUser.Uid {
		return p.UserWriteReadExecutable()
	}

	// If not check, check for other permissions
	if p.Stat.Mode().Perm()&fileperm.OsOthR != 0 && p.Stat.Mode().Perm()&fileperm.OsOthW != 0 &&
		p.Stat.Mode().Perm()&fileperm.OsOthX != 0 {
		return true
	}

	return false
}

// deleteACLEntries deletes ACL entries inside a security context.
func deleteACLEntries(data interface{}) error {
	// Assert data is of slurmSecurityCtxData
	var d *deleteACLEntriesCtxData

	var ok bool
	if d, ok = data.(*deleteACLEntriesCtxData); !ok {
		return ErrSecurityCtxDataAssertion
	}

	// Get and delete ACL entries
	for _, acl := range d.acls {
		a := &acls.ACL{}

		// Load ACL entries from a given path object
		if err := a.Load(acl.path, acls.PosixACLAccess); err != nil {
			return err
		}

		// Delete entry from entries
		a.DeleteEntry(acl.entry)

		// Apply entry to new ACL
		if err := a.Apply(acl.path, acls.PosixACLAccess); err != nil {
			return err
		}
	}

	return nil
}
