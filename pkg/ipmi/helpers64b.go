//go:build amd64 || arm64 || mips64 || mips64le || ppc64le || riscv64
// +build amd64 arm64 mips64 mips64le ppc64le riscv64

package ipmi

import (
	"time"

	"golang.org/x/sys/unix"
)

const (
	// NFDBitS is the amount of bits per mask.
	NFDBits = 8 * 8
)

// FDZero set to zero the fdSet.
func FDZero(p *unix.FdSet) {
	p.Bits = [16]int64{}
}

// timeval returns a pointer to unix.Timeval based on timeout value.
func (t *timeout) timeval() *unix.Timeval {
	var timeoutS, timeoutUs time.Duration
	if t.value >= time.Second {
		timeoutS = t.value.Truncate(time.Second)
		timeoutUs = t.value - timeoutS
	} else {
		timeoutS = 0
		timeoutUs = t.value
	}

	return &unix.Timeval{Sec: int64(timeoutS.Seconds()), Usec: timeoutUs.Microseconds()}
}
