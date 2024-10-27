package collector

import (
	"os"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedTargetsV2 = []Target{
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46231", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46281", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "1009248"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46235", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46236", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "1009249"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "26242", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "1009250"}},
	}
	expectedTargetsV2Filtered = []Target{
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46231", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46281", "service_name": "1009248"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46235", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46236", "service_name": "1009249"}},
	}
	expectedTargetsV1 = []Target{
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46231", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46281", "service_name": "1009248"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46235", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46236", "service_name": "1009249"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "26242", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "1009250"}},
	}
)

func TestAlloyDiscovererSlurmCgroupsV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets.resource-manager", "slurm",
		"--collector.cgroups.force-version", "v2",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(log.NewNopLogger())
	require.NoError(t, err)

	targets, err := discoverer.Discover()
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedTargetsV2, targets)
}

func TestAlloyDiscovererSlurmCgroupsV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets.resource-manager", "slurm",
		"--collector.cgroups.force-version", "v1",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(log.NewNopLogger())
	require.NoError(t, err)

	targets, err := discoverer.Discover()
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedTargetsV1, targets)
}

func TestAlloyDiscovererSlurmCgroupsV2WithEnviron(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets.resource-manager", "slurm",
		"--discoverer.alloy-targets.env-var", "ENABLE_PROFILING",
		"--collector.cgroups.force-version", "v2",
		"--discoverer.alloy-targets.self-profiler",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(log.NewNopLogger())
	require.NoError(t, err)

	targets, err := discoverer.Discover()
	require.NoError(t, err)

	expectedTargets := append(expectedTargetsV2Filtered, Target{
		Targets: []string{selfTargetID},
		Labels: map[string]string{
			"__process_pid__": strconv.FormatInt(int64(os.Getpid()), 10),
			"service_name":    selfTargetID,
		},
	})

	assert.ElementsMatch(t, expectedTargets, targets)
}
