package collector

import (
	"path/filepath"
)

var (
	// The path of the proc filesystem.
	sysPath    = CEEMSExporterApp.Flag("path.sysfs", "sysfs mountpoint.").Hidden().Default("/sys").String()
	procfsPath = CEEMSExporterApp.Flag("path.procfs", "procfs mountpoint.").
			Hidden().
			Default("/proc").
			String()
	cgroupfsPath = CEEMSExporterApp.Flag("path.cgroupfs", "cgroupfs mountpoint.").Hidden().
			Default("/sys/fs/cgroup").
			String()
)

// sysFilePath returns the sub directory of sys fs.
func sysFilePath(name string) string {
	return filepath.Join(*sysPath, name)
}

// procFilePath returns the sub directory of path fs.
func procFilePath(name string) string {
	return filepath.Join(*procfsPath, name)
}

// cgroupFilePath returns the sub directory of crgoup fs.
func cgroupFilePath(name string) string {
	return filepath.Join(*cgroupfsPath, name)
}
