package collector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/procfs"
)

var (
	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
	reParens        = regexp.MustCompile(`\((.*)\)`)
)

type cgroup struct {
	id    string
	procs []procfs.Proc
}

// SanitizeMetricName sanitize the given metric name by replacing invalid characters by underscores.
//
// OpenMetrics and the Prometheus exposition format require the metric name
// to consist only of alphanumericals and "_", ":" and they must not start
// with digits. Since colons in MetricFamily are reserved to signal that the
// MetricFamily is the result of a calculation or aggregation of a general
// purpose monitoring system, colons will be replaced as well.
//
// Note: If not subsequently prepending a namespace and/or subsystem (e.g.,
// with prometheus.BuildFQName), the caller must ensure that the supplied
// metricName does not begin with a digit.
func SanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}

// getCgroups returns a slice of active cgroups and processes contained in each cgroup.
func getCgroups(fs procfs.FS, idRegex *regexp.Regexp, targetEnvVars []string, procFilter func(string) bool) ([]cgroup, error) {
	// Get all active procs
	allProcs, err := fs.AllProcs()
	if err != nil {
		return nil, err
	}

	// If no idRegex provided, return empty
	if idRegex == nil {
		return nil, errors.New("cgroup IDs cannot be retrieved due to empty regex")
	}

	cgroupsMap := make(map[string][]procfs.Proc)

	var cgroupIDs []string

	for _, proc := range allProcs {
		// Get cgroup ID from regex
		var cgroupID string

		cgrps, err := proc.Cgroups()
		if err != nil || len(cgrps) == 0 {
			continue
		}

		for _, cgrp := range cgrps {
			// If cgroup path is root, skip
			if cgrp.Path == "/" {
				continue
			}

			// Unescape UTF-8 characters in cgroup path
			sanitizedPath, err := unescapeString(cgrp.Path)
			if err != nil {
				continue
			}

			cgroupIDMatches := idRegex.FindStringSubmatch(sanitizedPath)
			if len(cgroupIDMatches) <= 1 {
				continue
			}

			cgroupID = cgroupIDMatches[1]

			break
		}

		// If no cgroupID found, ignore
		if cgroupID == "" {
			continue
		}

		// If targetEnvVars is not empty check if this env vars is present for the process
		// We dont check for the value of env var. Presence of env var is enough to
		// trigger the profiling of that process
		if len(targetEnvVars) > 0 {
			environ, err := proc.Environ()
			if err != nil {
				continue
			}

			for _, env := range environ {
				for _, targetEnvVar := range targetEnvVars {
					if strings.HasPrefix(env, targetEnvVar) {
						goto check_process
					}
				}
			}

			// If target env var(s) is not found, return
			continue
		}

	check_process:
		// Ignore processes where command line matches the regex
		if procFilter != nil {
			procCmdLine, err := proc.CmdLine()
			if err != nil || len(procCmdLine) == 0 {
				continue
			}

			// Ignore process if matches found
			if procFilter(strings.Join(procCmdLine, " ")) {
				continue
			}
		}

		cgroupsMap[cgroupID] = append(cgroupsMap[cgroupID], proc)
		cgroupIDs = append(cgroupIDs, cgroupID)
	}

	// Sort cgroupIDs and make slice of cgProcs
	cgroups := make([]cgroup, len(cgroupsMap))

	slices.Sort(cgroupIDs)

	for icgroup, cgroupID := range slices.Compact(cgroupIDs) {
		cgroups[icgroup] = cgroup{
			id:    cgroupID,
			procs: cgroupsMap[cgroupID],
		}
	}

	return cgroups, nil
}

// fileExists checks if given file exists or not.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

// lookPath is like exec.LookPath but looks only in /sbin, /usr/sbin,
// /usr/local/sbin which are reserved for super user.
func lookPath(f string) (string, error) {
	locations := []string{
		"/sbin",
		"/usr/sbin",
		"/usr/local/sbin",
	}

	for _, path := range locations {
		fullPath := filepath.Join(path, f)
		if fileExists(fullPath) {
			return fullPath, nil
		}
	}

	return "", errors.New("file does not exist")
}

// inode returns the inode of a given path.
func inode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("error running stat(%s): %w", path, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("missing syscall.Stat_t in FileInfo for %s", path)
	}

	return stat.Ino, nil
}

// unescapeString sanitizes the string by unescaping UTF-8 characters.
func unescapeString(s string) (string, error) {
	sanitized, err := strconv.Unquote("\"" + s + "\"")
	if err != nil {
		return "", err
	}

	return sanitized, nil
}

// readUintFromFile reads a file and attempts to parse a uint64 from it.
func readUintFromFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}
