package collector

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedTargetsV2 = []Target{
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56231", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56281", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "2009248"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56235", "service_name": "2009249"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56236", "service_name": "2009249"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "2009249"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "2009249"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "36242", "service_name": "2009250"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "56233", "service_name": "2009250"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "2009250"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "2009250"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "2009250"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66231", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66281", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "3009248"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66235", "service_name": "3009249"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66236", "service_name": "3009249"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "3009249"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "3009249"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "46242", "service_name": "3009250"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "66233", "service_name": "3009250"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "3346567", "service_name": "3009250"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "3346596", "service_name": "3009250"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "3346674", "service_name": "3009250"}},
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
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56231", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56281", "service_name": "2009248"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56235", "service_name": "2009249"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56236", "service_name": "2009249"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66231", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66281", "service_name": "3009248"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66235", "service_name": "3009249"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66236", "service_name": "3009249"}},
	}
	expectedTargetsV1 = []Target{
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46231", "service_name": "1009248"}},
		{Targets: []string{"1009248"}, Labels: map[string]string{"__process_pid__": "46281", "service_name": "1009248"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46235", "service_name": "1009249"}},
		{Targets: []string{"1009249"}, Labels: map[string]string{"__process_pid__": "46236", "service_name": "1009249"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "26242", "service_name": "1009250"}},
		{Targets: []string{"1009250"}, Labels: map[string]string{"__process_pid__": "46233", "service_name": "1009250"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56231", "service_name": "2009248"}},
		{Targets: []string{"2009248"}, Labels: map[string]string{"__process_pid__": "56281", "service_name": "2009248"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56235", "service_name": "2009249"}},
		{Targets: []string{"2009249"}, Labels: map[string]string{"__process_pid__": "56236", "service_name": "2009249"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "36242", "service_name": "2009250"}},
		{Targets: []string{"2009250"}, Labels: map[string]string{"__process_pid__": "56233", "service_name": "2009250"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66231", "service_name": "3009248"}},
		{Targets: []string{"3009248"}, Labels: map[string]string{"__process_pid__": "66281", "service_name": "3009248"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66235", "service_name": "3009249"}},
		{Targets: []string{"3009249"}, Labels: map[string]string{"__process_pid__": "66236", "service_name": "3009249"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "46242", "service_name": "3009250"}},
		{Targets: []string{"3009250"}, Labels: map[string]string{"__process_pid__": "66233", "service_name": "3009250"}},
	}
)

func TestAlloyDiscovererSlurmCgroupsV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets",
		"--collector.slurm",
		"--collector.cgroups.force-version", "v2",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(noOpLogger)
	require.NoError(t, err)

	targets, err := discoverer.Discover()
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedTargetsV2, targets)
}

func TestAlloyDiscovererSlurmCgroupsV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets",
		"--collector.slurm",
		"--collector.cgroups.force-version", "v1",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(noOpLogger)
	require.NoError(t, err)

	targets, err := discoverer.Discover()
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedTargetsV1, targets)
}

func TestAlloyDiscovererSlurmCgroupsV2WithEnviron(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--discoverer.alloy-targets",
		"--collector.slurm",
		"--discoverer.alloy-targets.env-var", "ENABLE_PROFILING",
		"--collector.cgroups.force-version", "v2",
		"--discoverer.alloy-targets.self-profiler",
	})
	require.NoError(t, err)

	discoverer, err := NewAlloyTargetDiscoverer(noOpLogger)
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
