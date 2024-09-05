package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/containerd/cgroups/v3"
)

const (
	// Max cgroup subsystems count that is used from BPF side
	// to define a max index for the default controllers on tasks.
	// For further documentation check BPF part.
	cgroupSubSysCount = 15
)

// Regular expressions of cgroup paths for different resource managers
/*
	For v1 possibilities are /cpuacct/slurm/uid_1000/job_211
							 /memory/slurm/uid_1000/job_211

	For v2 possibilities are /system.slice/slurmstepd.scope/job_211
							/system.slice/slurmstepd.scope/job_211/step_interactive
							/system.slice/slurmstepd.scope/job_211/step_extern/user/task_0
*/
var (
	slurmCgroupPathRegex  = regexp.MustCompile("^.*/slurm(?:.*?)/job_([0-9]+)(?:.*$)")
	slurmIgnoreProcsRegex = regexp.MustCompile("slurmstepd:(.*)|sleep ([0-9]+)|/bin/bash (.*)/slurm_script")
)

// cgroupFS is a struct that contains cgroup related info for a
// given resource manager.
type cgroupFS struct {
	mode       cgroups.CGMode    // cgroups mode: unified, legacy, hybrid
	root       string            // cgroups root
	mount      string            // path at which resource manager manages cgroups
	subsystem  string            // active subsystem in cgroups v1
	manager    string            // cgroup manager
	idRegex    *regexp.Regexp    // regular expression to capture cgroup ID
	pathFilter func(string) bool // function to filter cgroup paths. Function must return true if cgroup path must be ignored
	procFilter func(string) bool // function to filter processes in cgroup based on cmdline. Function must return true if process must be ignored
}

// cgroupController is a container for cgroup controllers in v1.
type cgroupController struct {
	id     uint64 // Hierarchy unique ID
	idx    uint64 // Cgroup SubSys index
	name   string // Controller name
	active bool   // Will be set to true if controller is set and active
}

// slurmCgroupFS returns cgroupFS struct for SLURM.
func slurmCgroupFS(cgroupRootPath, subsystem, forceCgroupsVersion string) cgroupFS {
	var cgroup cgroupFS
	if cgroups.Mode() == cgroups.Unified {
		cgroup = cgroupFS{
			mode:  cgroups.Unified,
			root:  cgroupRootPath,
			mount: filepath.Join(cgroupRootPath, "system.slice/slurmstepd.scope"),
		}
	} else {
		cgroup = cgroupFS{
			mode:      cgroups.Mode(),
			root:      filepath.Join(cgroupRootPath, subsystem),
			mount:     filepath.Join(cgroupRootPath, subsystem, "slurm"),
			subsystem: subsystem,
		}
	}

	// For overriding in tests
	if forceCgroupsVersion != "" {
		if forceCgroupsVersion == "v2" {
			cgroup = cgroupFS{
				mode:  cgroups.Unified,
				root:  cgroupRootPath,
				mount: filepath.Join(cgroupRootPath, "system.slice/slurmstepd.scope"),
			}
		} else if forceCgroupsVersion == "v1" {
			cgroup = cgroupFS{
				mode:      cgroups.Legacy,
				root:      filepath.Join(cgroupRootPath, subsystem),
				mount:     filepath.Join(cgroupRootPath, subsystem, "slurm"),
				subsystem: subsystem,
			}
		}
	}

	// Add manager field
	cgroup.manager = "slurm"

	// Add path regex
	cgroup.idRegex = slurmCgroupPathRegex

	// Add filter functions
	cgroup.pathFilter = func(p string) bool {
		return strings.Contains(p, "/step_")
	}
	cgroup.procFilter = func(p string) bool {
		return slurmIgnoreProcsRegex.MatchString(p)
	}

	return cgroup
}

// parseCgroupSubSysIds returns cgroup controllers for cgroups v1.
func parseCgroupSubSysIds() ([]cgroupController, error) {
	var cgroupControllers []cgroupController

	// Read /proc/cgroups file
	file, err := os.Open(procFilePath("cgroups"))
	if err != nil {
		return nil, err
	}

	defer file.Close()

	fscanner := bufio.NewScanner(file)

	var idx uint64 = 0

	fscanner.Scan() // ignore first entry

	for fscanner.Scan() {
		line := fscanner.Text()
		fields := strings.Fields(line)

		/* We care only for the controllers that we want */
		if idx >= cgroupSubSysCount {
			/* Maybe some cgroups are not upstream? */
			return cgroupControllers, fmt.Errorf("cgroup default subsystem '%s' is indexed at idx=%d higher than CGROUP_SUBSYS_COUNT=%d",
				fields[0], idx, cgroupSubSysCount)
		}

		if id, err := strconv.ParseUint(fields[1], 10, 32); err == nil {
			cgroupControllers = append(cgroupControllers, cgroupController{
				id:     id,
				idx:    idx,
				name:   fields[0],
				active: true,
			})
		}

		idx++
	}

	return cgroupControllers, nil
}
