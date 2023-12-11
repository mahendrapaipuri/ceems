package helpers

import (
	"fmt"
	"os/exec"
	"strings"

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
		err = fmt.Errorf("error running %s: %s", cmd, err)
	}
	return out, err
}
