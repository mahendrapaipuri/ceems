// Package osexec implements subprocess execution functions
package osexec

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	sudoCmd = "sudo"
)

// Custom errors.
var (
	ErrInvalidUID = errors.New("invalid UID")
	ErrInvalidGID = errors.New("invalid GID")
)

// Execute command and return stdout/stderr.
func Execute(cmd string, args []string, env []string) ([]byte, error) {
	execCmd := exec.Command(cmd, args...)

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Execute command
	return execCmd.CombinedOutput()
}

// ExecuteAs executes a command as a given UID and GID and return stdout/stderr.
func ExecuteAs(cmd string, args []string, uid int, gid int, env []string) ([]byte, error) {
	execCmd := exec.Command(cmd, args...)

	// Check bounds on uid and gid before converting into int32
	uidInt32, err := convertToUint(uid)
	if err != nil {
		return nil, err
	}

	gidInt32, err := convertToUint(gid)
	if err != nil {
		return nil, err
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Set uid and gid for process
	execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uidInt32, Gid: gidInt32}

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	// Execute command
	return execCmd.CombinedOutput()
}

// ExecuteContext executes a command with context and return stdout/stderr.
func ExecuteContext(ctx context.Context, cmd string, args []string, env []string) ([]byte, error) {
	execCmd := exec.CommandContext(ctx, cmd, args...)

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Execute command
	return execCmd.CombinedOutput()
}

// ExecuteAsContext executes a command as a given UID and GID with context and return stdout/stderr.
func ExecuteAsContext(
	ctx context.Context,
	cmd string,
	args []string,
	uid int,
	gid int,
	env []string,
) ([]byte, error) {
	execCmd := exec.CommandContext(ctx, cmd, args...)

	// Check bounds on uid and gid before converting into int32
	uidInt32, err := convertToUint(uid)
	if err != nil {
		return nil, err
	}

	gidInt32, err := convertToUint(gid)
	if err != nil {
		return nil, err
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Set uid and gid for process
	execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uidInt32, Gid: gidInt32}

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	return execCmd.CombinedOutput()
}

// ExecuteWithTimeout exwecutes a command with timeout and return stdout/stderr.
func ExecuteWithTimeout(cmd string, args []string, timeout int, env []string) ([]byte, error) {
	ctx := context.Background()

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
	}

	execCmd := exec.CommandContext(ctx, cmd, args...)

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// The signal to send to the children when parent receives a kill signal
	// execCmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}

	// Execute command
	return execCmd.CombinedOutput()
}

// ExecuteAsWithTimeout executes a command with timeout as a given UID and GID and return stdout/stderr.
func ExecuteAsWithTimeout(
	cmd string,
	args []string,
	uid int,
	gid int,
	timeout int,
	env []string,
) ([]byte, error) {
	ctx := context.Background()

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
	}

	execCmd := exec.CommandContext(ctx, cmd, args...)

	// If env is not nil pointer, add env vars into subprocess cmd
	if env != nil {
		execCmd.Env = append(os.Environ(), env...)
	}

	// Check bounds on uid and gid before converting into int32
	uidInt32, err := convertToUint(uid)
	if err != nil {
		return nil, err
	}

	gidInt32, err := convertToUint(gid)
	if err != nil {
		return nil, err
	}

	// According to setpgid docs (https://man7.org/linux/man-pages/man2/setpgid.2.html)
	// we cannot use setpgid and setsid at the same time
	if cmd == sudoCmd {
		// Attach a separate terminal less session to the subprocess
		// This is to avoid prompting for password when we run command with sudo
		// Ref: https://stackoverflow.com/questions/13432947/exec-external-program-script-and-detect-if-it-requests-user-input
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		// Start child process in its own process group so that interrupt signal will
		// not stop the command
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Set uid and gid for process
	execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uidInt32, Gid: gidInt32}

	// Execute command
	return execCmd.CombinedOutput()
}

// convertToUint converts int to uint32 after checking bounds.
func convertToUint(i int) (uint32, error) {
	if i >= 0 && i <= math.MaxInt32 {
		return uint32(i), nil
	}

	return 0, ErrInvalidUID
}
