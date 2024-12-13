package ipmi

import "golang.org/x/sys/unix"

// FDSet set a fd of fdSet.
func FDSet(fd uintptr, p *unix.FdSet) {
	p.Bits[fd/NFDBits] |= (1 << (fd % NFDBits))
}

// FDClr clear a fd of fdSet.
func FDClr(fd uintptr, p *unix.FdSet) {
	p.Bits[fd/NFDBits] &^= (1 << fd % NFDBits)
}

// FDIsSet return true if fd is set.
func FDIsSet(fd uintptr, p *unix.FdSet) bool {
	return p.Bits[fd/NFDBits]&(1<<(fd%NFDBits)) != 0
}
