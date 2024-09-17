//go:build !noebpf
// +build !noebpf

package collector

// Many of these utility functions have been nicked from https://github.com/cilium/tetragon

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ksym is a structure for a kernel symbol.
type ksym struct {
	// addr uint64
	name string
	ty   string
	kmod string
}

// isFunction returns true if the given kernel symbol is a function.
func (ksym *ksym) isFunction() bool {
	tyLow := strings.ToLower(ksym.ty)

	return tyLow == "w" || tyLow == "t"
}

// Ksyms is a structure for kernel symbols.
type Ksyms struct {
	table []ksym
}

// NewKsyms creates a new Ksyms structure (by reading procfs/kallsyms).
func NewKsyms() (*Ksyms, error) {
	file, err := os.Open(procFilePath("kallsyms"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// err = nil
	var ksyms Ksyms

	s := bufio.NewScanner(file)
	// needsSort := false

	for s.Scan() {
		txt := s.Text()
		fields := strings.Fields(txt)

		var sym ksym

		if len(fields) < 3 {
			// fmt.Fprintf(os.Stderr, "Failed to parse: '%s'\n", txt)
			continue
		}

		// Reading symbol addresses need privileges and we are currently not
		// using addresses. So, ignore reading addresses and populate table
		// only with names
		// if sym.addr, err = strconv.ParseUint(fields[0], 16, 64); err != nil {
		// 	err = fmt.Errorf("failed to parse address: %v", err)
		// 	break
		// }
		sym.ty = fields[1]
		sym.name = fields[2]

		// fmt.Printf("%s => %d %s\n", txt, sym.addr, sym.name)
		// if sym.isFunction() && sym.addr == 0 {
		// 	err = fmt.Errorf("function %s reported at address 0. Insuffcient permissions?", sym.name)
		// 	break
		// }

		// check if this symbol is part of a kmod
		if sym.isFunction() && len(fields) >= 4 {
			sym.kmod = strings.Trim(fields[3], "[]")
		}

		// if !needsSort && len(ksyms.table) > 0 {
		// 	lastSym := ksyms.table[len(ksyms.table)-1]
		// 	if lastSym.addr > sym.addr {
		// 		needsSort = true
		// 	}
		// }

		ksyms.table = append(ksyms.table, sym)
	}

	// if err == nil {
	// 	err = s.Err()
	// }

	// if err != nil && len(ksyms.table) == 0 {
	// 	err = errors.New("no symbols found")
	// }

	// if err != nil {
	// 	return nil, err
	// }

	// if needsSort {
	// 	sort.Slice(ksyms.table[:], func(i1, i2 int) bool { return ksyms.table[i1].addr < ksyms.table[i2].addr })
	// }

	return &ksyms, nil
}

// IsAvailable returns true if the given name is available on current kernel.
func (k *Ksyms) IsAvailable(name string) bool {
	for _, sym := range k.table {
		if sym.name == name {
			return true
		}
	}

	return false
}

// GetArchSpecificName returns architecture specific symbol (if exists) of a given
// kernel symbol.
func (k *Ksyms) GetArchSpecificName(name string) (string, error) {
	// This linear search is slow. But this only happens during the validation
	// of kprobe-based tracing polies. TODO: optimise if needed
	reg := regexp.MustCompile(fmt.Sprintf("(.*)%s$", name))
	for _, s := range k.table {
		// Compiler optimizations will add suffixes like .constprops, .isra
		// Split them first and then check for prefixes
		// https://people.redhat.com/~jolawren/klp-compiler-notes/livepatch/compiler-considerations.html
		// https://lore.kernel.org/lkml/20170104172509.27350-13-acme@kernel.org/
		if reg.MatchString(strings.Split(s.name, ".")[0]) {
			// We should not return symbols with __pfx_ and __cfi_ prefixes
			// https://lore.kernel.org/lkml/20230207135402.38f73bb6@gandalf.local.home/t/
			// https://www.spinics.net/lists/kernel/msg4573413.html
			if !strings.HasPrefix(s.name, "__pfx_") && !strings.HasPrefix(s.name, "__cfi_") {
				return s.name, nil
			}
		}
	}

	return "", fmt.Errorf("symbol %s not found in kallsyms or is not part of a module", name)
}

// KernelStringToNumeric converts the kernel version string into a numerical value
// that can be used to make comparison.
func KernelStringToNumeric(ver string) int64 {
	// vendors like to define kernel 4.14.128-foo but
	// everything after '-' is meaningless from BPF
	// side so toss it out.
	release := strings.Split(ver, "-")
	verStr := release[0]
	numeric := strings.TrimRight(verStr, "+")
	vers := strings.Split(numeric, ".")

	// Split out major, minor, and patch versions
	majorS := vers[0]

	minorS := ""
	if len(vers) >= 2 {
		minorS = vers[1]
	}

	patchS := ""
	if len(vers) >= 3 {
		patchS = vers[2]
	}

	// If we have no major version number, all is lost
	major, err := strconv.ParseInt(majorS, 10, 32)
	if err != nil {
		return 0
	}
	// Fall back to minor = 0 if we can't parse the minor version
	minor, err := strconv.ParseInt(minorS, 10, 32)
	if err != nil {
		minor = 0
	}
	// Fall back to patch = 0 if we can't parse the patch version
	patch, err := strconv.ParseInt(patchS, 10, 32)
	if err != nil {
		patch = 0
	}
	// Similar to https://elixir.bootlin.com/linux/v6.2.16/source/tools/lib/bpf/bpf_helpers.h#L74
	// we have to check that patch is <= 255. Otherwise make that 255.
	if patch > 255 {
		patch = 255
	}

	return ((major << 16) + (minor << 8) + patch)
}

// KernelVersion returns kernel version of current host.
func KernelVersion() (int64, error) {
	var versionStrings []string

	if versionSig, err := os.ReadFile(procFilePath("version_signature")); err == nil {
		versionStrings = strings.Fields(string(versionSig))
	}

	if len(versionStrings) > 0 {
		return KernelStringToNumeric(versionStrings[len(versionStrings)-1]), nil
	}

	var uname unix.Utsname

	err := unix.Uname(&uname)
	if err != nil {
		return 0, err
	}

	release := unix.ByteSliceToString(uname.Release[:])

	return KernelStringToNumeric(release), nil
}
