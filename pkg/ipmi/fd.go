package ipmi

import "golang.org/x/sys/unix"

// FDZero set to zero the fdSet.
func FDZero(p *unix.FdSet) {
	p.Bits = [16]int64{}
}

// FDSet set a fd of fdSet.
func FDSet(fd int, p *unix.FdSet) {
	p.Bits[fd/32] |= (1 << (uint(fd) % 32))
}

// FDClr clear a fd of fdSet.
func FDClr(fd int, p *unix.FdSet) {
	p.Bits[fd/32] &^= (1 << (uint(fd) % 32))
}

// FDIsSet return true if fd is set.
func FDIsSet(fd int, p *unix.FdSet) bool {
	return p.Bits[fd/32]&(1<<(uint(fd)%32)) != 0
}
