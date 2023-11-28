// Taken from node_exporter/collectors/paths_test.go and modified

package collector

import (
	"testing"

	"github.com/alecthomas/kingpin/v2"
)

func TestDefaultSysPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.sysfs", "/sys"}); err != nil {
		t.Fatal(err)
	}

	if got, want := sysFilePath("somefile"), "/sys/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := sysFilePath("some/file"), "/sys/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestCustomSysPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.sysfs", "./../some/./place/"}); err != nil {
		t.Fatal(err)
	}

	if got, want := sysFilePath("somefile"), "../some/place/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := sysFilePath("some/file"), "../some/place/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestDefaultCgroupPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.cgroupfs", "/sys/fs/cgroup"}); err != nil {
		t.Fatal(err)
	}

	if got, want := cgroupFilePath("somefile"), "/sys/fs/cgroup/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := cgroupFilePath("some/file"), "/sys/fs/cgroup/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestCustomCgroupPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.cgroupfs", "./../some/./place/"}); err != nil {
		t.Fatal(err)
	}

	if got, want := cgroupFilePath("somefile"), "../some/place/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := cgroupFilePath("some/file"), "../some/place/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}
