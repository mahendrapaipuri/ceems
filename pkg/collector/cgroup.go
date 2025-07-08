package collector

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
)

const (
	// Max cgroup subsystems count that is used from BPF side
	// to define a max index for the default controllers on tasks.
	// For further documentation check BPF part.
	cgroupSubSysCount = 15
	genericSubsystem  = "compute"
)

// Shares.
const (
	milliCPUtoCPU = 1000
	sharesPerCPU  = 1024
	minShares     = 2
	maxShares     = 262144
)

// Custom errors.
var (
	errUnknownManager = errors.New("unknown resource manager")
)

type manager int

// Resource Managers.
const (
	_ manager = iota
	slurm
	libvirt
	k8s
)

// Resource manager names.
var rmNames = map[manager]string{
	slurm:   "slurm",
	libvirt: "libvirt",
	k8s:     "k8s",
}

// Block IO Op names.
const (
	readOp  = "Read"
	writeOp = "Write"
)

const (
	cpuSubsystem = "cpu,cpuacct"
	netSubsystem = "net_cls,net_prio"
)

const (
	systemdSlicesName    = "machine.slice"
	nonSystemdSlicesName = "machine"
)

// Regular expressions of cgroup paths for different resource managers.
// ^.*/(?:(.*?)_)?slurm(?:_(.*?)/)?(?:.*?)/job_([0-9]+)(?:.*$)
// ^.*/slurm(?:_(.*?))?/(?:.*?)/job_([0-9]+)(?:.*$)
/*
	For v1 possibilities are /cpuacct/slurm/uid_1000/job_211
							 /memory/slurm/uid_1000/job_211

	For v2 possibilities are /system.slice/slurmstepd.scope/job_211
							/system.slice/slurmstepd.scope/job_211/step_interactive
							/system.slice/slurmstepd.scope/job_211/step_extern/user/task_0
*/
var (
	slurmCgroupV1PathRegex = regexp.MustCompile("^.*/slurm(?:_(?P<host>.*?)/)?(?:.*?)job_(?P<id>[0-9]+)(?:.*$)")
	slurmCgroupV2PathRegex = regexp.MustCompile("^.*/(?:(?P<host>.*?)_)?slurm(?:.*?)/job_(?P<id>[0-9]+)(?:.*$)")
	slurmIgnoreProcsRegex  = regexp.MustCompile("slurmstepd:(.*)|sleep ([0-9]+)|/bin/bash (.*)/slurm_script")
)

// Ref: https://libvirt.org/cgroups.html#legacy-cgroups-layout
// Take escaped unicode characters in regex
/*
	For v1 possibilities are /cpuacct/machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope
							 /memory/machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope

	For v2 possibilities are /machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope
							 /machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope/libvirt

	Non systemd: machine/qemu-1-instance1.libvirt-qemu
*/
var (
	libvirtCgroupPathRegex          = regexp.MustCompile("^.*/(?:.+?)-qemu-(?:[0-9]+)-(?P<id>instance-[0-9a-f]+)(?:.*$)")
	libvirtCgroupNoSystemdPathRegex = regexp.MustCompile("^.*/(?:.+?)qemu-(?:[0-9]+)-(?P<id>instance-[0-9a-f]+)(?:.*$)")
)

// Ref: https://linuxera.org/cpu-memory-management-kubernetes-cgroupsv2/
// https://regex101.com/r/2RksNL/1
/*
	For v1 possibilities are /cpuacct/kubepods/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /cpuacct/kubepods/besteffort/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /cpuacct/kubepods/burstable/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /memory/kubepods/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /memory/kubepods/besteffort/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /memory/kubepods/burstable/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12

	For v2 possibilities are /kubepods/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /kubepods/besteffort/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12
							 /kubepods/burstable/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12

	For systemd driver
	For v1 possibilities are /cpuacct/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod2fc932ce_fdcc_454b_97bd_aadfdeb4c340.slice/cri-containerd-aaefb9d8feed2d453b543f6d928cede7a4dbefa6a0ae7c9b990dd234c56e93b9.scope
	For v2 possibilities are /kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod2fc932ce_fdcc_454b_97bd_aadfdeb4c340.slice/cri-containerd-aaefb9d8feed2d453b543f6d928cede7a4dbefa6a0ae7c9b990dd234c56e93b9.scope
*/
var (
	k8sCgroupPathRegex = regexp.MustCompile("^.*/kubepods(?:.*)pod(?P<id>[0-9a-z-_]+)(?:.*$)")
)

// CLI options.
var (
	activeController = CEEMSExporterApp.Flag(
		"collector.cgroups.active-subsystem",
		"Active subsystem for cgroups v1.",
	).Default("cpuacct").String()

	// Hidden opts for e2e and unit tests.
	forceCgroupsVersion = CEEMSExporterApp.Flag(
		"collector.cgroups.force-version",
		"Set cgroups version manually. Used only for testing.",
	).Hidden().Enum("v1", "v2")

	noSystemdMode = CEEMSExporterApp.Flag(
		"collector.cgroups.no-systemd-mode",
		"Set if running on a non-systemd host",
	).Default("false").Bool()
)

func resolveSlices(nonSystemdMode bool) string {
	if nonSystemdMode {
		return nonSystemdSlicesName
	} else {
		return systemdSlicesName
	}
}

func resolveLibvirtRegex(nonSystemdMode bool) *regexp.Regexp {
	if nonSystemdMode {
		return libvirtCgroupNoSystemdPathRegex
	} else {
		return libvirtCgroupPathRegex
	}
}

// resolveSubsystem returns the resolved cgroups v1 subsystem.
func resolveSubsystem(subsystem string) string {
	switch subsystem {
	case "cpuacct":
		return cpuSubsystem
	case "cpu":
		return cpuSubsystem
	case "net_cls":
		return netSubsystem
	case "net_prio":
		return netSubsystem
	default:
		return subsystem
	}
}

type cgroupPath struct {
	abs, rel string
}

// String implements stringer interface of the struct.
func (c *cgroupPath) String() string {
	return c.abs
}

type cgroup struct {
	id       string
	uuid     string // uuid is the identifier known to user whereas id is identifier used by resource manager internally
	hostname string
	procs    []procfs.Proc
	path     cgroupPath
	children []cgroupPath // All the children under this root cgroup
}

// String implements stringer interface of the struct.
func (c *cgroup) String() string {
	return fmt.Sprintf(
		"id: %s path: %s num_procs: %d num_children: %d",
		c.id,
		c.path,
		len(c.procs),
		len(c.children),
	)
}

// cgroupManager is the container that have cgroup information of resource manager.
type cgroupManager struct {
	logger           *slog.Logger
	fs               procfs.FS
	id               manager           // cgroup manager ID
	mode             cgroups.CGMode    // cgroups mode: unified, legacy, hybrid
	root             string            // cgroups root
	slices           []string          // Slice(s) under which cgroups are managed eg system.slice, machine.slice
	scopes           []string          // Scope(s) under which cgroups are managed eg slurmstepd.scope, machine-qemu\x2d1\x2dvm1.scope
	activeController string            // Active controller for cgroups v1
	mountPoints      []string          // Path(s) under which resource manager creates cgroups
	name             string            // cgroup manager name
	idRegex          *regexp.Regexp    // Regular expression to capture cgroup ID set by resource manager
	isChild          func(string) bool // Function to identify child cgroup paths. Function must return true if cgroup is a child to root cgroup
	ignoreProc       func(string) bool // Function to filter processes in cgroup based on cmdline. Function must return true if process must be ignored
}

// NewCgroupManager returns an instance of cgroupManager based on resource manager.
func NewCgroupManager(name manager, logger *slog.Logger) (*cgroupManager, error) {
	// Instantiate a new Proc FS
	fs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		logger.Error("Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	var manager *cgroupManager

	switch name {
	/*
		When SLURM is compiled with --enable-multiple-slurmd flag, SLURM
		will create cgroup directories with nodename in it. This nodename
		is the nodename declared in SLURM conf. Effectively this is to run
		multiple "virtual" hosts within a single physical host.

		In this case there will be multiple cgroup directories on each host
		and we need to account for them. The name of cgroup directories is
		created differently for v1 and v2.

		In the case of v1, it will /slurm_{nodename}/cpuacct/
		In the case of v2, it will be /system.slice/{nodename}_slurmstepd.scope/

		We discover all the cgroup directories and add them to either slice or scope
		depending on the version of cgroup. This way we discover cgroups from different
		cgroup directories and we add a "special" label `cgrouphostname` in the metrics.

		References:
		https://github.com/SchedMD/slurm/blob/a5b7de91e64fb0206890361fcb0aed5bce08d41e/src/interfaces/cgroup.c#L173-L177
		https://github.com/SchedMD/slurm/blob/a5b7de91e64fb0206890361fcb0aed5bce08d41e/src/plugins/cgroup/v2/cgroup_v2.c#L407-L416
		https://github.com/SchedMD/slurm/blob/a5b7de91e64fb0206890361fcb0aed5bce08d41e/src/plugins/cgroup/v1/xcgroup.c#L365-L373
		https://github.com/SchedMD/slurm/blob/a5b7de91e64fb0206890361fcb0aed5bce08d41e/src/common/read_config.c#L5574-L5581
		https://slurm.schedmd.com/cgroup_v2.html#slurmd_startup
	*/
	case slurm:
		if (*forceCgroupsVersion == "" && cgroups.Mode() == cgroups.Unified) || *forceCgroupsVersion == "v2" {
			// Discover all cgroup roots
			scopes, err := lookupCgroupRoots(filepath.Join(*cgroupfsPath, "system.slice"), "slurmstepd.scope")
			if err != nil {
				logger.Error("Failed to discover cgroup roots", "err", err)

				return nil, err
			}

			manager = &cgroupManager{
				logger:  logger,
				fs:      fs,
				mode:    cgroups.Unified,
				root:    *cgroupfsPath,
				idRegex: slurmCgroupV2PathRegex,
				slices:  []string{"system.slice"},
				scopes:  scopes,
			}
		} else {
			var mode cgroups.CGMode
			if *forceCgroupsVersion == "v1" {
				mode = cgroups.Legacy
			} else {
				mode = cgroups.Mode()
			}

			// Resolve subsystem
			activeSubsystem := resolveSubsystem(*activeController)

			// Discover all cgroup roots
			slices, err := lookupCgroupRoots(filepath.Join(*cgroupfsPath, activeSubsystem), "slurm")
			if err != nil {
				logger.Error("Failed to discover cgroup roots", "err", err)

				return nil, err
			}

			manager = &cgroupManager{
				logger:           logger,
				fs:               fs,
				mode:             mode,
				root:             *cgroupfsPath,
				idRegex:          slurmCgroupV1PathRegex,
				activeController: activeSubsystem,
				slices:           slices,
			}
		}

		// Add manager field
		manager.id = name
		manager.name = rmNames[name]

		// Identify child cgroup
		manager.isChild = func(p string) bool {
			return strings.Contains(p, "/step_")
		}
		manager.ignoreProc = func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		}

		// Set mountpoints
		manager.setMountPoints()

		return manager, nil

	case libvirt:
		if (*forceCgroupsVersion == "" && cgroups.Mode() == cgroups.Unified) || *forceCgroupsVersion == "v2" {
			manager = &cgroupManager{
				logger: logger,
				fs:     fs,
				mode:   cgroups.Unified,
				root:   *cgroupfsPath,
				slices: []string{resolveSlices(*noSystemdMode)},
			}
		} else {
			var mode cgroups.CGMode
			if *forceCgroupsVersion == "v1" {
				mode = cgroups.Legacy
			} else {
				mode = cgroups.Mode()
			}

			// Resolve subsystem
			activeSubsystem := resolveSubsystem(*activeController)

			manager = &cgroupManager{
				logger:           logger,
				fs:               fs,
				mode:             mode,
				root:             *cgroupfsPath,
				activeController: activeSubsystem,
				slices:           []string{resolveSlices(*noSystemdMode)},
			}
		}

		// Add manager field
		manager.id = name
		manager.name = rmNames[name]

		// Add path regex
		manager.idRegex = resolveLibvirtRegex(*noSystemdMode)

		// Identify child cgroup
		// In cgroups v1 or on a non-systemd host, all the child cgroups like emulator, vcpu* are flat whereas
		// in v2 they are all inside libvirt child
		manager.isChild = func(p string) bool {
			return strings.Contains(p, "/libvirt") || strings.Contains(p, "/emulator") || strings.Contains(p, "/vcpu")
		}
		manager.ignoreProc = func(p string) bool {
			return false
		}

		// Set mountpoint
		manager.setMountPoints()

		return manager, nil

	/*
		For k8s, there are two different cgroup drivers possible: systemd and cgroupfs

		Depending on the driver used, the cgroup paths will be different. For
		cgroupfs drivers, cgroups will be created at

		/kubepods/pod5ba385ca-b6e6-4246-bd66-d48d429dbc12

		where as for systemd driver, cgroups will be created at

		/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod2fc932ce_fdcc_454b_97bd_aadfdeb4c340.slice

		As of 2025, k8s advise to use systemd driver for the distros supporting
		cgroups v2. However, the default for kubelet and containerd is still cgroupfs

		If we configure kubelet to use systemd driver and containerd with cgroupfs driver,
		containerd starts to create container cgroups in `/system.slice` outside of `/kubelet.slice`
		and this can have all sorts of issues. If we find the current host is in this situation,
		we should warn the users. Moreover, if we end up in this situtation, we will not be able
		to get cgroup metrics of pods as pod cgroups will be empty!!

		References:
		https://github.com/kubernetes/kubernetes/blob/f007012f5fe49e40ae0596cf463a8e7b247b3357/pkg/kubelet/stats/cri_stats_provider.go#L952-L967
		https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/configure-cgroup-driver/
		https://gjhenrique.com/cgroups-k8s/
	*/
	case k8s:
		var activeSubsystem string

		if (*forceCgroupsVersion == "" && cgroups.Mode() == cgroups.Unified) || *forceCgroupsVersion == "v2" {
			manager = &cgroupManager{
				logger: logger,
				fs:     fs,
				mode:   cgroups.Unified,
				root:   *cgroupfsPath,
			}
		} else {
			var mode cgroups.CGMode
			if *forceCgroupsVersion == "v1" {
				mode = cgroups.Legacy
			} else {
				mode = cgroups.Mode()
			}

			// Resolve subsystem
			activeSubsystem = resolveSubsystem(*activeController)

			manager = &cgroupManager{
				logger:           logger,
				fs:               fs,
				mode:             mode,
				root:             *cgroupfsPath,
				activeController: activeSubsystem,
			}
		}

		// Discover all cgroup slices depending on the driver used
		for _, slice := range []string{"kubepods", "kubepods.slice"} {
			if _, err := os.Stat(filepath.Join(*cgroupfsPath, activeSubsystem, slice)); err == nil {
				manager.slices = append(manager.slices, slice)
			}
		}

		// If both kubepods and kubepods.slice are found, it means the node
		// has changed from one cgroup driver to another and there has not been
		// a reboot after the modification. This isnt a good idea and warn about it
		if len(manager.slices) == 2 {
			logger.Warn(
				"Both kubepods and kubepods.slice found in cgroup FS. This happens when cgroup driver of " +
					"kubelet has been modified. To remove obselete cgroup folder, consider rebooting the node",
			)

			// In this scenario, verify if there are cgroups formed under `/system.slice`. This
			// happens when cgroup driver on containerd does not match with the one of kubelet.
			if matches, err := filepath.Glob(filepath.Join(*cgroupfsPath, activeSubsystem, "/system.slice/kubepods*")); err == nil && len(matches) > 0 {
				logger.Warn(
					"Containerd creating container cgroups in /system.slice instead of /kubepods.slice. " +
						"This happens when cgroup driver of containerd does not match with that of kubelet and this can have " +
						"undesirable effects. Consider using same cgroup driver for both kubelet and containerd",
				)
			}
		}

		// Add manager field
		manager.id = name
		manager.name = rmNames[name]

		// Add path regex
		manager.idRegex = k8sCgroupPathRegex

		// Identify child cgroup
		manager.isChild = func(p string) bool {
			// With systemd driver, every path with cri-containerd is a child
			if strings.Contains(p, "/cri-containerd") {
				return true
			}

			// With cgroupfs driver, we check the base name of path and if it does not
			// contain pod, it is a child
			return !strings.Contains(filepath.Base(p), "pod")
		}
		manager.ignoreProc = func(p string) bool {
			return false
		}

		// Set mountpoint
		manager.setMountPoints()

		return manager, nil

	default:
		return nil, errUnknownManager
	}
}

// String implements stringer interface of the struct.
func (c *cgroupManager) String() string {
	var mode string
	switch c.mode { //nolint:exhaustive
	case cgroups.Legacy:
		mode = "v1/legacy"
	case cgroups.Hybrid:
		mode = "hybrid"
	case cgroups.Unified:
		mode = "v2/unified"
	default:
		mode = "unknown"
	}

	return fmt.Sprintf(
		"mode: %s manager: %s  root: %s slice(s): %s scope(s): %s mount(s): %s",
		mode,
		c.name,
		c.root,
		strings.Join(c.slices, ","),
		strings.Join(c.scopes, ","),
		strings.Join(c.mountPoints, ","),
	)
}

// setMountPoints discover mount points for the current manager.
func (c *cgroupManager) setMountPoints() {
	switch c.id {
	case slurm:
		switch c.mode { //nolint:exhaustive
		case cgroups.Unified:
			// /sys/fs/cgroup/system.slice/slurmstepd.scope
			// /sys/fs/cgroup/system.slice/node0_slurmstepd.scope
			for _, slice := range c.slices {
				for _, scope := range c.scopes {
					c.mountPoints = append(c.mountPoints, filepath.Join(c.root, slice, scope))
				}
			}
		default:
			// /sys/fs/cgroup/cpuacct/slurm
			for _, slice := range c.slices {
				c.mountPoints = append(c.mountPoints, filepath.Join(c.root, c.activeController, slice))
			}

			// For cgroups v1 we need to shift root to /sys/fs/cgroup/cpuacct
			c.root = filepath.Join(c.root, c.activeController)
		}
	case libvirt:
		switch c.mode { //nolint:exhaustive
		case cgroups.Unified:
			// /sys/fs/cgroup/machine.slice
			for _, slice := range c.slices {
				c.mountPoints = append(c.mountPoints, filepath.Join(c.root, slice))
			}
		default:
			// /sys/fs/cgroup/cpuacct/machine.slice
			for _, slice := range c.slices {
				c.mountPoints = append(c.mountPoints, filepath.Join(c.root, c.activeController, slice))
			}

			// For cgroups v1 we need to shift root to /sys/fs/cgroup/cpuacct
			c.root = filepath.Join(c.root, c.activeController)
		}
	case k8s:
		switch c.mode { //nolint:exhaustive
		case cgroups.Unified:
			// /sys/fs/cgroup/kubepods
			for _, slice := range c.slices {
				c.mountPoints = append(c.mountPoints, filepath.Join(c.root, slice))
			}
		default:
			// /sys/fs/cgroup/cpuacct/kubepods
			for _, slice := range c.slices {
				c.mountPoints = append(c.mountPoints, filepath.Join(c.root, c.activeController, slice))
			}

			// For cgroups v1 we need to shift root to /sys/fs/cgroup/cpuacct
			c.root = filepath.Join(c.root, c.activeController)
		}
	default:
		c.mountPoints = []string{c.root}
	}
}

// discover finds all the active cgroups in the given mountpoint.
func (c *cgroupManager) discover() ([]cgroup, error) {
	var cgroups []cgroup

	cgroupProcs := make(map[string][]procfs.Proc)

	cgroupChildren := make(map[string][]cgroupPath)

	// Walk through all cgroups and get cgroup paths
	// https://goplay.tools/snippet/coVDkIozuhg
	for _, mountPoint := range c.mountPoints {
		if err := filepath.WalkDir(mountPoint, func(p string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Ignore paths that are not directories
			if !info.IsDir() {
				return nil
			}

			// Get relative path of cgroup
			rel, err := filepath.Rel(c.root, p)
			if err != nil {
				c.logger.Error("Failed to resolve relative path for cgroup", "path", p, "err", err)

				return nil
			}

			// Add leading slash to relative path
			rel = "/" + rel

			// Unescape UTF-8 characters in cgroup path
			sanitizedPath, err := unescapeString(p)
			if err != nil {
				c.logger.Error("Failed to sanitize cgroup path", "path", p, "err", err)

				return nil
			}

			// Find all matches of regex
			matches := c.idRegex.FindStringSubmatch(sanitizedPath)
			if len(matches) < 2 {
				return nil
			}

			// Get capture group maps and map values to names
			captureGrps := make(map[string]string)
			for i, name := range c.idRegex.SubexpNames() {
				if i != 0 && name != "" {
					captureGrps[name] = matches[i]
				}
			}

			// Get cgroup ID which is instance ID
			id := strings.TrimSpace(captureGrps["id"])
			if id == "" {
				c.logger.Error("Empty cgroup ID", "path", p)

				return nil
			}

			// For k8s when systemd is used, there will be "_" in the
			// id. We need to replace them with "-"
			// Ref: https://github.com/kubernetes/kubernetes/blob/f007012f5fe49e40ae0596cf463a8e7b247b3357/pkg/kubelet/stats/cri_stats_provider.go#L952-L967
			id = strings.ReplaceAll(id, "_", "-")

			// Optionally we get "virtual" hostname as well if it is in
			// cgroup path (for SLURM only)
			vhost := strings.TrimSpace(captureGrps["host"])

			// Find procs in this cgroup
			if data, err := os.ReadFile(filepath.Join(p, "cgroup.procs")); err == nil {
				scanner := bufio.NewScanner(bytes.NewReader(data))
				for scanner.Scan() {
					if pid, err := strconv.ParseInt(scanner.Text(), 10, 0); err == nil {
						if proc, err := c.fs.Proc(int(pid)); err == nil {
							cgroupProcs[id] = append(cgroupProcs[id], proc)
						}
					}
				}
			}

			// Ignore child cgroups. We are only interested in root cgroup
			if c.isChild(p) {
				cgroupChildren[id] = append(cgroupChildren[id], cgroupPath{abs: sanitizedPath, rel: rel})

				return nil
			}

			// By default set id and uuid to same cgroup ID and if the resource
			// manager has two representations, override it in corresponding
			// collector. For instance, it applies only to libvirt
			cgrp := cgroup{
				id:       id,
				uuid:     id,
				path:     cgroupPath{abs: sanitizedPath, rel: rel},
				hostname: vhost,
			}

			cgroups = append(cgroups, cgrp)
			cgroupChildren[id] = append(cgroupChildren[id], cgroupPath{abs: sanitizedPath, rel: rel})

			return nil
		}); err != nil {
			c.logger.Error("Error walking cgroup subsystem", "path", mountPoint, "err", err)

			return nil, err
		}
	}

	// Merge cgroupProcs and cgroupChildren with cgroups slice
	for icgrp := range cgroups {
		if procs, ok := cgroupProcs[cgroups[icgrp].id]; ok {
			cgroups[icgrp].procs = procs
		}

		if children, ok := cgroupChildren[cgroups[icgrp].id]; ok {
			cgroups[icgrp].children = children
		}
	}

	return cgroups, nil
}

// cgMetric contains metrics returned by cgroup.
type cgMetric struct {
	cgroup          cgroup
	cpuUser         float64
	cpuSystem       float64
	cpuTotal        float64
	cpus            int
	cpuPressure     float64
	memoryRSS       float64
	memoryCache     float64
	memoryUsed      float64
	memoryTotal     float64
	memoryFailCount float64
	memswUsed       float64
	memswTotal      float64
	memswFailCount  float64
	memoryPressure  float64
	blkioReadBytes  map[string]float64
	blkioWriteBytes map[string]float64
	blkioReadReqs   map[string]float64
	blkioWriteReqs  map[string]float64
	blkioPressure   float64
	rdmaHCAHandles  map[string]float64
	rdmaHCAObjects  map[string]float64
	err             bool
}

// cgroupCollector collects cgroup metrics for different resource managers.
type cgroupCollector struct {
	logger            *slog.Logger
	cgroupManager     *cgroupManager
	opts              cgroupOpts
	hostname          string
	hostMemInfo       map[string]float64
	blockDevices      map[string]string
	numCgs            *prometheus.Desc
	cgCPUUser         *prometheus.Desc
	cgCPUSystem       *prometheus.Desc
	cgCPUs            *prometheus.Desc
	cgCPUPressure     *prometheus.Desc
	cgMemoryRSS       *prometheus.Desc
	cgMemoryCache     *prometheus.Desc
	cgMemoryUsed      *prometheus.Desc
	cgMemoryTotal     *prometheus.Desc
	cgMemoryFailCount *prometheus.Desc
	cgMemswUsed       *prometheus.Desc
	cgMemswTotal      *prometheus.Desc
	cgMemswFailCount  *prometheus.Desc
	cgMemoryPressure  *prometheus.Desc
	cgBlkioReadBytes  *prometheus.Desc
	cgBlkioWriteBytes *prometheus.Desc
	cgBlkioReadReqs   *prometheus.Desc
	cgBlkioWriteReqs  *prometheus.Desc
	cgBlkioPressure   *prometheus.Desc
	cgRDMAHCAHandles  *prometheus.Desc
	cgRDMAHCAObjects  *prometheus.Desc
	collectError      *prometheus.Desc
}

type cgroupOpts struct {
	collectSwapMemStats bool
	collectBlockIOStats bool
	collectPSIStats     bool
}

// NewCgroupCollector returns a new cgroupCollector exposing a summary of cgroups.
func NewCgroupCollector(logger *slog.Logger, cgManager *cgroupManager, opts cgroupOpts) (*cgroupCollector, error) {
	// Get total memory of host
	hostMemInfo := make(map[string]float64)

	file, err := os.Open(procFilePath("meminfo"))
	if err == nil {
		if memInfo, err := parseMemInfo(file); err == nil {
			hostMemInfo = memInfo
		}
	} else {
		logger.Error("Failed to get total memory of the host", "err", err)
	}

	defer file.Close()

	// Read block IO stats just to get block devices info.
	// We construct a map from major:minor to device name using this info
	blockDevices := make(map[string]string)

	if blockdevice, err := blockdevice.NewFS(*procfsPath, *sysPath); err == nil {
		if stats, err := blockdevice.ProcDiskstats(); err == nil {
			for _, s := range stats {
				blockDevices[fmt.Sprintf("%d:%d", s.MajorNumber, s.MinorNumber)] = s.DeviceName
			}
		} else {
			logger.Error("Failed to get stats of block devices on the host", "err", err)
		}
	} else {
		logger.Error("Failed to get list of block devices on the host", "err", err)
	}

	return &cgroupCollector{
		logger:        logger,
		cgroupManager: cgManager,
		opts:          opts,
		hostMemInfo:   hostMemInfo,
		hostname:      hostname,
		blockDevices:  blockDevices,
		numCgs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "units"),
			"Total number of jobs",
			[]string{"manager", "hostname"},
			nil,
		),
		cgCPUUser: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_user_seconds_total"),
			"Total job CPU user seconds",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgCPUSystem: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_system_seconds_total"),
			"Total job CPU system seconds",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgCPUs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpus"),
			"Total number of job CPUs",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgCPUPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_psi_seconds"),
			"Total CPU PSI in seconds",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_cache_bytes"),
			"Memory cache used in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_used_bytes"),
			"Memory used in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_total_bytes"),
			"Memory total in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_fail_count"),
			"Memory fail count",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_used_bytes"),
			"Swap used in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_total_bytes"),
			"Swap total in bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_fail_count"),
			"Swap fail count",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgMemoryPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_psi_seconds"),
			"Total memory PSI in seconds",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
		cgBlkioReadBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_blkio_read_total_bytes"),
			"Total block IO read bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgBlkioWriteBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_blkio_write_total_bytes"),
			"Total block IO write bytes",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgBlkioReadReqs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_blkio_read_total_requests"),
			"Total block IO read requests",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgBlkioWriteReqs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_blkio_write_total_requests"),
			"Total block IO write requests",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgBlkioPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_blkio_psi_seconds"),
			"Total block IO PSI in seconds",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgRDMAHCAHandles: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_handles"),
			"Current number of RDMA HCA handles",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		cgRDMAHCAObjects: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_objects"),
			"Current number of RDMA HCA objects",
			[]string{"manager", "hostname", "cgrouphostname", "uuid", "device"},
			nil,
		),
		collectError: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "collect_error"),
			"Indicates collection error, 0=no error, 1=error",
			[]string{"manager", "hostname", "cgrouphostname", "uuid"},
			nil,
		),
	}, nil
}

// Update updates cgroup metrics on given channel.
func (c *cgroupCollector) Update(ch chan<- prometheus.Metric, cgroups []cgroup) error {
	// Fetch metrics
	metrics := c.update(cgroups)

	// First send num jobs on the current host
	ch <- prometheus.MustNewConstMetric(c.numCgs, prometheus.GaugeValue, float64(len(metrics)), c.cgroupManager.name, c.hostname)

	// Send metrics of each cgroup
	for _, m := range metrics {
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, float64(1), c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		}

		// CPU stats
		ch <- prometheus.MustNewConstMetric(c.cgCPUUser, prometheus.CounterValue, m.cpuUser, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgCPUSystem, prometheus.CounterValue, m.cpuSystem, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgCPUs, prometheus.GaugeValue, float64(m.cpus)/milliCPUtoCPU, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)

		// Memory stats
		ch <- prometheus.MustNewConstMetric(c.cgMemoryRSS, prometheus.GaugeValue, m.memoryRSS, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryCache, prometheus.GaugeValue, m.memoryCache, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryUsed, prometheus.GaugeValue, m.memoryUsed, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryTotal, prometheus.GaugeValue, m.memoryTotal, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryFailCount, prometheus.GaugeValue, m.memoryFailCount, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)

		// Memory swap stats
		if c.opts.collectSwapMemStats {
			ch <- prometheus.MustNewConstMetric(c.cgMemswUsed, prometheus.GaugeValue, m.memswUsed, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemswTotal, prometheus.GaugeValue, m.memswTotal, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemswFailCount, prometheus.GaugeValue, m.memswFailCount, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		}

		// Block IO stats
		if c.opts.collectBlockIOStats {
			for device := range m.blkioReadBytes {
				if v, ok := m.blkioReadBytes[device]; ok && v > 0 {
					ch <- prometheus.MustNewConstMetric(c.cgBlkioReadBytes, prometheus.GaugeValue, v, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
				}

				if v, ok := m.blkioWriteBytes[device]; ok && v > 0 {
					ch <- prometheus.MustNewConstMetric(c.cgBlkioWriteBytes, prometheus.GaugeValue, v, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
				}

				if v, ok := m.blkioReadReqs[device]; ok && v > 0 {
					ch <- prometheus.MustNewConstMetric(c.cgBlkioReadReqs, prometheus.GaugeValue, v, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
				}

				if v, ok := m.blkioWriteReqs[device]; ok && v > 0 {
					ch <- prometheus.MustNewConstMetric(c.cgBlkioWriteReqs, prometheus.GaugeValue, v, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
				}
			}
		}

		// PSI stats
		if c.opts.collectPSIStats {
			ch <- prometheus.MustNewConstMetric(c.cgCPUPressure, prometheus.GaugeValue, m.cpuPressure, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemoryPressure, prometheus.GaugeValue, m.memoryPressure, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid)
		}

		// RDMA stats
		for device, handles := range m.rdmaHCAHandles {
			if handles > 0 {
				ch <- prometheus.MustNewConstMetric(c.cgRDMAHCAHandles, prometheus.GaugeValue, handles, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
			}
		}

		for device, objects := range m.rdmaHCAHandles {
			if objects > 0 {
				ch <- prometheus.MustNewConstMetric(c.cgRDMAHCAObjects, prometheus.GaugeValue, objects, c.cgroupManager.name, c.hostname, m.cgroup.hostname, m.cgroup.uuid, device)
			}
		}
	}

	return nil
}

// Stop releases any system resources held by collector.
func (c *cgroupCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "sub_collector", "cgroup")

	return nil
}

// update gets metrics of current active cgroups.
func (c *cgroupCollector) update(cgroups []cgroup) []cgMetric {
	// Start wait group for go routines
	wg := &sync.WaitGroup{}
	wg.Add(len(cgroups))

	// Initialise metrics
	metrics := make([]cgMetric, len(cgroups))

	// No need for any lock primitives here as we read/write
	// a different element of slice in each go routine
	for i, cgroup := range cgroups {
		go func(idx int) {
			defer wg.Done()

			metrics[idx] = c.stats(cgroup)
		}(i)
	}

	// Wait for all go routines
	wg.Wait()

	return metrics
}

// stats get metrics of a given cgroup path.
func (c *cgroupCollector) stats(cgrp cgroup) cgMetric {
	if c.cgroupManager.mode == cgroups.Unified {
		return c.statsV2(cgrp)
	} else {
		return c.statsV1(cgrp)
	}
}

// cpusFromCPUSet returns number of milli CPUs from cpuset cgroup.
func (c *cgroupCollector) cpusFromCPUSet(path string) (int, error) {
	var cpusPath string
	if c.cgroupManager.mode == cgroups.Unified {
		cpusPath = fmt.Sprintf("%s%s/cpuset.cpus.effective", *cgroupfsPath, path)
	} else {
		cpusPath = fmt.Sprintf("%s/cpuset%s/cpuset.cpus", *cgroupfsPath, path)
	}

	if !fileExists(cpusPath) {
		return -1, fmt.Errorf("cpuset file %s not found", cpusPath)
	}

	cpusData, err := os.ReadFile(cpusPath)
	if err != nil {
		c.logger.Error("Error reading cpuset", "cpuset", cpusPath, "err", err)

		return -1, err
	}

	// Parse cpuset range
	cpus, err := parseRange(strings.TrimSuffix(string(cpusData), "\n"))
	if err != nil {
		c.logger.Error("Error parsing cpuset", "cpuset", cpusPath, "err", err)

		return -1, err
	}

	return len(cpus) * milliCPUtoCPU, nil
}

// cpusFromChildren returns number of milli CPUs from child cgroups.
func (c *cgroupCollector) cpusFromChildren(path string) (int, error) {
	// Escape \x2d to get correct path
	path = strings.Trim(strconv.Quote(path), `"`)

	// For libvirt, there will be a vcpu* child cgroup for each
	// virtual CPU
	// In cgroup v1, they are flat whereas in cgroup v2 they are inside libvirt folder
	var vcpuPath string

	if c.cgroupManager.mode == cgroups.Unified && !(*noSystemdMode) {
		vcpuPath = fmt.Sprintf("%s%s/libvirt/vcpu*", c.cgroupManager.root, path)
	} else {
		vcpuPath = fmt.Sprintf("%s%s/vcpu*", c.cgroupManager.root, path)
	}

	matches, err := filepath.Glob(vcpuPath)
	if err != nil {
		c.logger.Error("Error finding vcpu* cgroups", "path", path, "err", err)

		return -1, err
	}

	return len(matches) * milliCPUtoCPU, nil
}

// cpusFromShares returns number of milli CPUs from cpu shares/weight from cgroup.
func (c *cgroupCollector) cpusFromShares(path string) (int, error) {
	// In cgroups v1, cpushares must be read whereas in cgroups v2
	// cpuweight must be read
	var filePath string

	switch c.cgroupManager.mode { //nolint:exhaustive
	case cgroups.Unified:
		filePath = fmt.Sprintf("%s%s/cpu.weight", c.cgroupManager.root, path)
	default:
		filePath = fmt.Sprintf("%s%s/cpu.shares", c.cgroupManager.root, path)
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		c.logger.Error("Error reading shares/weight file", "path", filePath, "err", err)

		return -1, err
	}

	// Convert to int
	val, err := strconv.ParseInt(strings.Trim(string(b), "\n"), 10, 64)
	if err != nil {
		c.logger.Error("Error parsing integer string", "path", filePath, "err", err)

		return -1, err
	}

	// If cgroups is v2, convert weight to shares
	// Ref: https://github.com/kubernetes/kubernetes/blob/f5da3b717fc2177b05b9cd23a0e2711d42f74cad/pkg/kubelet/cm/cgroup_manager_linux.go#L570-L574
	// Ref: https://victoriametrics.com/blog/kubernetes-cpu-go-gomaxprocs/
	if c.cgroupManager.mode == cgroups.Unified {
		val = (((val - 1) * 262142) / 9999) + 2
	}

	// Check bounds on shares
	if val < minShares {
		val = minShares
	}

	if val > maxShares {
		val = maxShares
	}

	// Convert to int
	var shares int
	if val > math.MaxInt {
		shares = math.MaxInt
	} else {
		shares = int(val)
	}

	// Convert shares to milli CPUs
	// Ref: https://github.com/kubernetes/kubernetes/blob/f5da3b717fc2177b05b9cd23a0e2711d42f74cad/pkg/kubelet/cm/helpers_linux.go#L85-L101
	milliCPUs := shares * milliCPUtoCPU / sharesPerCPU

	return milliCPUs, nil
}

// getCPUs returns number of milli CPUs in the cgroup.
func (c *cgroupCollector) getCPUs(path string) (int, error) {
	switch c.cgroupManager.id {
	case slurm:
		return c.cpusFromCPUSet(path)
	case libvirt:
		return c.cpusFromChildren(path)
	case k8s:
		return c.cpusFromShares(path)
	default:
		return -1, errUnknownManager
	}
}

// statsV1 fetches metrics from cgroups v1.
func (c *cgroupCollector) statsV1(cgrp cgroup) cgMetric {
	path := cgrp.path.rel
	metric := cgMetric{
		cgroup: cgrp,
	}

	c.logger.Debug("Loading cgroup v1", "path", path)

	ctrl, err := cgroup1.Load(cgroup1.StaticPath(path), cgroup1.WithHierarchy(subsystem))
	if err != nil {
		metric.err = true

		c.logger.Error("Failed to load cgroups", "path", path, "err", err)

		return metric
	}

	// Load cgroup stats
	stats, err := ctrl.Stat(cgroup1.IgnoreNotExist)
	if err != nil {
		metric.err = true

		c.logger.Error("Failed to stat cgroups", "path", path, "err", err)

		return metric
	}

	if stats == nil {
		metric.err = true

		c.logger.Error("Cgroup stats are nil", "path", path)

		return metric
	}

	// Get CPU stats
	if stats.GetCPU() != nil {
		if stats.GetCPU().GetUsage() != nil {
			metric.cpuUser = float64(stats.GetCPU().GetUsage().GetUser()) / 1000000000.0
			metric.cpuSystem = float64(stats.GetCPU().GetUsage().GetKernel()) / 1000000000.0
			metric.cpuTotal = float64(stats.GetCPU().GetUsage().GetTotal()) / 1000000000.0
		}
	}

	if ncpus, err := c.getCPUs(path); err == nil {
		metric.cpus = ncpus
	}

	// Get memory stats
	if stats.GetMemory() != nil {
		metric.memoryRSS = float64(stats.GetMemory().GetTotalRSS())
		metric.memoryCache = float64(stats.GetMemory().GetTotalCache())

		if stats.GetMemory().GetUsage() != nil {
			metric.memoryUsed = float64(stats.GetMemory().GetUsage().GetUsage())
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.GetMemory().GetUsage().GetLimit() == math.MaxUint64 && c.hostMemInfo["MemTotal_bytes"] > 0 {
				metric.memoryTotal = c.hostMemInfo["MemTotal_bytes"]
			} else {
				metric.memoryTotal = float64(stats.GetMemory().GetUsage().GetLimit())
			}

			metric.memoryFailCount = float64(stats.GetMemory().GetUsage().GetFailcnt())
		}

		if stats.GetMemory().GetSwap() != nil {
			metric.memswUsed = float64(stats.GetMemory().GetSwap().GetUsage())
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.GetMemory().GetSwap().GetLimit() == math.MaxUint64 {
				switch {
				case c.hostMemInfo["SwapTotal_bytes"] > 0:
					metric.memswTotal = c.hostMemInfo["SwapTotal_bytes"]
				case c.hostMemInfo["MemTotal_bytes"] > 0:
					metric.memswTotal = c.hostMemInfo["MemTotal_bytes"]
				default:
					metric.memswTotal = float64(stats.GetMemory().GetSwap().GetLimit())
				}
			} else {
				metric.memswTotal = float64(stats.GetMemory().GetSwap().GetLimit())
			}

			metric.memswFailCount = float64(stats.GetMemory().GetSwap().GetFailcnt())
		}
	}

	// Get block IO stats
	if stats.GetBlkio() != nil {
		metric.blkioReadBytes = make(map[string]float64)
		metric.blkioReadReqs = make(map[string]float64)
		metric.blkioWriteBytes = make(map[string]float64)
		metric.blkioWriteReqs = make(map[string]float64)

		for _, stat := range stats.GetBlkio().GetIoServiceBytesRecursive() {
			devName := c.blockDevices[fmt.Sprintf("%d:%d", stat.GetMajor(), stat.GetMinor())]

			if stat.GetOp() == readOp {
				metric.blkioReadBytes[devName] = float64(stat.GetValue())
			} else if stat.GetOp() == writeOp {
				metric.blkioWriteBytes[devName] = float64(stat.GetValue())
			}
		}

		for _, stat := range stats.GetBlkio().GetIoServicedRecursive() {
			devName := c.blockDevices[fmt.Sprintf("%d:%d", stat.GetMajor(), stat.GetMinor())]

			if stat.GetOp() == readOp {
				metric.blkioReadReqs[devName] = float64(stat.GetValue())
			} else if stat.GetOp() == writeOp {
				metric.blkioWriteReqs[devName] = float64(stat.GetValue())
			}
		}
	}

	// Get RDMA metrics if available
	if stats.GetRdma() != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)

		for _, device := range stats.GetRdma().GetCurrent() {
			metric.rdmaHCAHandles[device.GetDevice()] = float64(device.GetHcaHandles())
			metric.rdmaHCAObjects[device.GetDevice()] = float64(device.GetHcaObjects())
		}
	}

	return metric
}

// statsV2 fetches metrics from cgroups v2.
func (c *cgroupCollector) statsV2(cgrp cgroup) cgMetric {
	path := cgrp.path.rel
	metric := cgMetric{
		cgroup: cgrp,
	}

	c.logger.Debug("Loading cgroup v2", "path", path)

	// Load cgroups
	ctrl, err := cgroup2.Load(path, cgroup2.WithMountpoint(*cgroupfsPath))
	if err != nil {
		metric.err = true

		c.logger.Error("Failed to load cgroups", "path", path, "err", err)

		return metric
	}

	// Get stats from cgroup
	stats, err := ctrl.Stat()
	if err != nil {
		metric.err = true

		c.logger.Error("Failed to stat cgroups", "path", path, "err", err)

		return metric
	}

	if stats == nil {
		metric.err = true

		c.logger.Error("Cgroup stats are nil", "path", path)

		return metric
	}

	// Get CPU stats
	if stats.GetCPU() != nil {
		metric.cpuUser = float64(stats.GetCPU().GetUserUsec()) / 1000000.0
		metric.cpuSystem = float64(stats.GetCPU().GetSystemUsec()) / 1000000.0
		metric.cpuTotal = float64(stats.GetCPU().GetUsageUsec()) / 1000000.0

		if stats.GetCPU().GetPSI() != nil {
			metric.cpuPressure = float64(stats.GetCPU().GetPSI().GetFull().GetTotal()) / 1000000.0
		}
	}

	if ncpus, err := c.getCPUs(path); err == nil {
		metric.cpus = ncpus
	}

	// Get memory stats
	// cgroups2 does not expose swap memory events. So we dont set memswFailCount
	if stats.GetMemory() != nil {
		metric.memoryUsed = float64(stats.GetMemory().GetUsage())
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with total system memory
		if stats.GetMemory().GetUsageLimit() == math.MaxUint64 && c.hostMemInfo["MemTotal_bytes"] > 0 {
			metric.memoryTotal = c.hostMemInfo["MemTotal_bytes"]
		} else {
			metric.memoryTotal = float64(stats.GetMemory().GetUsageLimit())
		}

		metric.memoryCache = float64(stats.GetMemory().GetFile()) // This is page cache
		metric.memoryRSS = float64(stats.GetMemory().GetAnon())
		metric.memswUsed = float64(stats.GetMemory().GetSwapUsage())
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with either total swap/system memory
		if stats.GetMemory().GetSwapLimit() == math.MaxUint64 {
			switch {
			case c.hostMemInfo["SwapTotal_bytes"] > 0:
				metric.memswTotal = c.hostMemInfo["SwapTotal_bytes"]
			case c.hostMemInfo["MemTotal_bytes"] > 0:
				metric.memswTotal = c.hostMemInfo["MemTotal_bytes"]
			default:
				metric.memswTotal = float64(stats.GetMemory().GetSwapLimit())
			}
		} else {
			metric.memswTotal = float64(stats.GetMemory().GetSwapLimit())
		}

		if stats.GetMemory().GetPSI() != nil {
			metric.memoryPressure = float64(stats.GetMemory().GetPSI().GetFull().GetTotal()) / 1000000.0
		}
	}
	// Get memory events
	if stats.GetMemoryEvents() != nil {
		metric.memoryFailCount = float64(stats.GetMemoryEvents().GetOom())
	}

	// Get block IO stats
	if stats.GetIo() != nil {
		metric.blkioReadBytes = make(map[string]float64)
		metric.blkioReadReqs = make(map[string]float64)
		metric.blkioWriteBytes = make(map[string]float64)
		metric.blkioWriteReqs = make(map[string]float64)

		for _, stat := range stats.GetIo().GetUsage() {
			devName := c.blockDevices[fmt.Sprintf("%d:%d", stat.GetMajor(), stat.GetMinor())]
			metric.blkioReadBytes[devName] = float64(stat.GetRbytes())
			metric.blkioReadReqs[devName] = float64(stat.GetRios())
			metric.blkioWriteBytes[devName] = float64(stat.GetWbytes())
			metric.blkioWriteReqs[devName] = float64(stat.GetWios())
		}

		if stats.GetIo().GetPSI() != nil {
			metric.blkioPressure = float64(stats.GetIo().GetPSI().GetFull().GetTotal()) / 1000000.0
		}
	}

	// Get RDMA stats
	if stats.GetRdma() != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)

		for _, device := range stats.GetRdma().GetCurrent() {
			metric.rdmaHCAHandles[device.GetDevice()] = float64(device.GetHcaHandles())
			metric.rdmaHCAObjects[device.GetDevice()] = float64(device.GetHcaObjects())
		}
	}

	return metric
}

// subsystem returns cgroups v1 subsystems.
func subsystem() ([]cgroup1.Subsystem, error) {
	s := []cgroup1.Subsystem{
		cgroup1.NewCpuacct(*cgroupfsPath),
		cgroup1.NewMemory(*cgroupfsPath),
		cgroup1.NewRdma(*cgroupfsPath),
		cgroup1.NewPids(*cgroupfsPath),
		cgroup1.NewBlkio(*cgroupfsPath),
		cgroup1.NewCpuset(*cgroupfsPath),
	}

	return s, nil
}

// cgroupController is a container for cgroup controllers in v1.
type cgroupController struct {
	id     uint64 // Hierarchy unique ID
	idx    uint64 // Cgroup SubSys index
	name   string // Controller name
	active bool   // Will be set to true if controller is set and active
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
			return cgroupControllers, fmt.Errorf(
				"cgroup default subsystem '%s' is indexed at idx=%d higher than CGROUP_SUBSYS_COUNT=%d",
				fields[0],
				idx,
				cgroupSubSysCount,
			)
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
