// Package security implements privilege management and execution of
// privileged actions in security contexts.
package security

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"kernel.org/pub/linux/libs/security/libcap/cap"
)

type Config struct {
	RunAsUser      string      // Change to this user if app is started as root
	Caps           []cap.Value // Capabilities necessary for the app
	ReadPaths      []string    // Paths that "RunAsUser" user able to read
	ReadWritePaths []string    // Paths that "RunAsUser" user able to read/write
}

// DropPrivileges will change `root` user to `nobody` and drop any
// unnecessary privileges only keeping the ones passed in `caps` argument.
// If current user is not root, this function is no-op and we expect
// either process or file to have necessary capabilities in the production
// environments.
func DropPrivileges(config *Config) error {
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
		return setCapabilities(config.Caps)
	}

	// Here we set a bunch of linux specific security stuff.
	// Change ownership on any files that need read/write access to runAsUser
	if err := changeOwnership(config); err != nil {
		return err
	}

	// Now change the user from root to runAsUser
	if err := changeUser(config.RunAsUser); err != nil {
		return err
	}

	// Ensure ReadPaths and ReadWritePaths are accessible for runAsUser.
	// This can happen when any of the parent directories do not have rx
	// on others which might prevent runAsUser to access these paths.
	if err := pathsReachable(config); err != nil {
		return err
	}

	// Attempt to set capabilities before we setup seccomp rules
	// Note that we discard any errors because they are not actionable.
	// The beat should use `getcap` at a later point to examine available capabilities
	// rather than relying on errors from `setcap`
	return setCapabilities(config.Caps)
}

// DropCapabilities drops any existing capabilities on the process.
func DropCapabilities() error {
	return setCapabilities(nil)
}

// changeUser switches the current user to specific localUserName.
func changeUser(localUserName string) error {
	localUser, err := user.Lookup(localUserName)
	if err != nil {
		return fmt.Errorf("could not lookup %s: %w", localUser, err)
	}

	localUserUID, err := strconv.Atoi(localUser.Uid)
	if err != nil {
		return fmt.Errorf("could not parse UID %s as int: %w", localUser.Uid, err)
	}

	localUserGID, err := strconv.Atoi(localUser.Gid)
	if err != nil {
		return fmt.Errorf("could not parse GID %s as int: %w", localUser.Uid, err)
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

	// This may not be necessary, but is good hygiene
	return os.Setenv("HOME", localUser.HomeDir)
}

// pathsReachable tests if all the relevant paths are reachable for runAsUser.
func pathsReachable(config *Config) error {
	// Read paths
	for _, path := range config.ReadPaths {
		if path == "" {
			continue
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("could not reach path %s after changing user to %s", path, config.RunAsUser)
		}
	}

	// ReadWrite paths
	for _, path := range config.ReadWritePaths {
		if path == "" {
			continue
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("could not reach path %s after changing user to %s", path, config.RunAsUser)
		}
	}

	return nil
}

// changeOwnership changes the ownership on all relevant files to runAsUser.
func changeOwnership(config *Config) error {
	// Read paths
	for _, path := range config.ReadPaths {
		if path == "" {
			continue
		}

		if err := changePathOwnership(path, config.RunAsUser, false); err != nil {
			return err
		}
	}

	// ReadWrite paths
	for _, path := range config.ReadWritePaths {
		if path == "" {
			continue
		}

		if err := changePathOwnership(path, config.RunAsUser, true); err != nil {
			return err
		}
	}

	return nil
}

// changePathOwnership changes the user ownership on a given path to the user.
func changePathOwnership(path string, runAsUserName string, readWrite bool) error {
	runAsUser, err := user.Lookup(runAsUserName)
	if err != nil {
		return fmt.Errorf("could not lookup %s: %w", runAsUserName, err)
	}

	// Get runAdUser's UID
	runAsUserUID, err := strconv.Atoi(runAsUser.Uid)
	if err != nil {
		return fmt.Errorf("could not parse UID %s as int: %w", runAsUser.Uid, err)
	}

	// Stat path
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not stat path %s: %w", path, err)
	}

	// Get GID that we need to conserve of the path
	var gid int
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		gid = int(stat.Gid)
	} else {
		return fmt.Errorf("could not get UID and GID of path %s", path)
	}

	// Now change ownership to userName's uid
	if err := os.Chown(path, runAsUserUID, gid); err != nil {
		return fmt.Errorf("could not change ownership on path %s: %w", path, err)
	}

	// If readWrite is true, ensure that the path has write permissions for user
	if readWrite {
		if err := os.Chmod(path, info.Mode()|(os.FileMode(syscall.S_IWUSR))); err != nil {
			return fmt.Errorf("could not change permissions on path %s: %w", path, err)
		}
	}

	return nil
}

// setCapabilities sets the specific list of Linux capabilities on current process.
// It only add the capabilities to `permitted` set and it is responsible of the
// functions that need privileges to enable `effective` set before perfoming
// privileged action and then dropping them off straight after.
func setCapabilities(caps []cap.Value) error {
	// Start with an empty capability set
	newcaps := cap.NewSet()

	// Permitted makes the permission possible to get, effective makes it 'active'
	for _, c := range caps {
		if err := newcaps.SetFlag(cap.Permitted, true, c); err != nil {
			return fmt.Errorf("error setting permitted setcap: %w", err)
		}

		// Only enable effective set before performing a privileged operation
		if err := newcaps.SetFlag(cap.Effective, false, c); err != nil {
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
