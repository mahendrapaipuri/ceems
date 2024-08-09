// Package runtime implements the utility functions to fetch runtime info of current host
// Nicked from https://github.com/prometheus/prometheus/blob/main/util/runtime
package runtime

import (
	"fmt"
	"math"
	"syscall"

	"golang.org/x/sys/unix"
)

// syscall.RLIM_INFINITY is a constant.
// Its type is int on most architectures but there are exceptions such as loong64.
// Uniform it to uint accorind to the standard.
// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/sys_resource.h.html
var unlimited uint64 = syscall.RLIM_INFINITY & math.MaxUint64

// Uname returns the uname of the host machine.
func Uname() string {
	buf := unix.Utsname{}

	err := unix.Uname(&buf)
	if err != nil {
		panic("unix.Uname failed: " + err.Error())
	}

	str := "(" + unix.ByteSliceToString(buf.Sysname[:])
	str += " " + unix.ByteSliceToString(buf.Release[:])
	str += " " + unix.ByteSliceToString(buf.Version[:])
	str += " " + unix.ByteSliceToString(buf.Machine[:])
	str += " " + unix.ByteSliceToString(buf.Nodename[:])
	str += " " + unix.ByteSliceToString(buf.Domainname[:]) + ")"

	return str
}

func limitToString(v uint64, unit string) string {
	if v == unlimited {
		return "unlimited"
	}

	return fmt.Sprintf("%d%s", v, unit)
}

func getLimits(resource int, unit string) string {
	rlimit := syscall.Rlimit{}

	err := syscall.Getrlimit(resource, &rlimit)
	if err != nil {
		panic("syscall.Getrlimit failed: " + err.Error())
	}
	// rlimit.Cur and rlimit.Max are int64 on some platforms, such as dragonfly.
	// We need to cast them explicitly to uint64.
	return fmt.Sprintf(
		"(soft=%s, hard=%s)",
		limitToString(rlimit.Cur, unit),
		limitToString(rlimit.Max, unit),
	)
}

// FdLimits returns the soft and hard limits for file descriptors.
func FdLimits() string {
	return getLimits(syscall.RLIMIT_NOFILE, "")
}
