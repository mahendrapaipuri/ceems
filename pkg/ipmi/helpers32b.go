//go:build 386 || mips || mipsle
// +build 386 mips mipsle

package ipmi

import (
	"time"

	"golang.org/x/sys/unix"
)

const (
	// NFDBitS is the amount of bits per mask
	NFDBits = 4 * 8
)

// FDZero set to zero the fdSet.
func FDZero(p *unix.FdSet) {
	p.Bits = [32]int32{}
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

	return &unix.Timeval{Sec: int32(timeoutS.Seconds()), Usec: int32(timeoutUs.Microseconds())}
}
