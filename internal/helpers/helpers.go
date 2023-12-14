package helpers

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/zeebo/xxh3"
)

// Get a UUID5 for given slice of strings
func GetUuidFromString(stringSlice []string) (string, error) {
	s := strings.Join(stringSlice[:], ",")
	h := xxh3.HashString128(s).Bytes()
	uuid, err := uuid.FromBytes(h[:])
	return uuid.String(), err
}

// Execute command and return stdout/stderr
func Execute(cmd string, args []string, logger log.Logger) ([]byte, error) {
	level.Debug(logger).Log("msg", "Executing", "command", cmd, "args", fmt.Sprintf("%+v", args))
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		level.Error(logger).Log("msg", "Error executing command", "command", cmd, "args", fmt.Sprintf("%+v", args), "err", err)
	}
	return out, err
}

// Execute command as a given UID and GID and return stdout/stderr
func ExecuteAs(cmd string, args []string, uid int, gid int, logger log.Logger) ([]byte, error) {
	level.Debug(logger).Log("msg", "Executing as user", "command", cmd, "args", fmt.Sprintf("%+v", args), "uid", uid, "gid", gid)
	execCmd := exec.Command(cmd, args...)
	execCmd.SysProcAttr = &syscall.SysProcAttr{}
	execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	out, err := execCmd.CombinedOutput()
	if err != nil {
		level.Error(logger).Log("msg", "Error executing command as user", "command", cmd, "args", fmt.Sprintf("%+v", args), "uid", uid, "gid", gid, "err", err)
	}
	return out, err
}
