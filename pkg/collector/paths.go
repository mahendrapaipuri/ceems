package collector

import (
	"path/filepath"

	"github.com/alecthomas/kingpin/v2"
)

var (
	// The path of the proc filesystem.
	sysPath      = kingpin.Flag("path.sysfs", "sysfs mountpoint.").Default("/sys").String()
	procfsPath   = kingpin.Flag("path.procfs", "procfs mountpoint.").Default("/proc").String()
	cgroupfsPath = kingpin.Flag("path.cgroupfs", "cgroupfs mountpoint.").
			Default("/sys/fs/cgroup").
			String()
)

func sysFilePath(name string) string {
	return filepath.Join(*sysPath, name)
}

func procFilePath(name string) string {
	return filepath.Join(*procfsPath, name)
}

func cgroupFilePath(name string) string {
	return filepath.Join(*cgroupfsPath, name)
}
