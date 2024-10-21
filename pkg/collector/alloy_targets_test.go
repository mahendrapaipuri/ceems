package collector

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedTargets = []Target{
	{
		Targets: []string{"1320003"},
		Labels: map[string]string{
			"__process_commandline":   "/gpfslocalsup/spack_soft/gromacs/2022.2/gcc-8.4.1-kblhs7pjrcqlgv675gejjjy7n3h6wz2n/bin/gmx_mpi mdrun -ntomp 10 -v -deffnm run10 -multidir 1/ 2/ 3/ 4/",
			"__process_effective_uid": "1000",
			"__process_exe":           "/usr/bin/vim",
			"__process_pid__":         "46236",
			"__process_real_uid":      "1000",
			"service_name":            "1320003",
		},
	},
	{
		Targets: []string{"1320003"},
		Labels: map[string]string{
			"__process_commandline":   "/gpfslocalsup/spack_soft/gromacs/2022.2/gcc-8.4.1-kblhs7pjrcqlgv675gejjjy7n3h6wz2n/bin/gmx_mpi mdrun -ntomp 10 -v -deffnm run10 -multidir 1/ 2/ 3/ 4/",
			"__process_effective_uid": "1000",
			"__process_exe":           "/usr/bin/vim",
			"__process_pid__":         "46235",
			"__process_real_uid":      "1000",
			"service_name":            "1320003",
		},
	},
	{
		Targets: []string{"4824887"},
		Labels: map[string]string{
			"__process_commandline":   "/gpfslocalsup/spack_soft/gromacs/2022.2/gcc-8.4.1-kblhs7pjrcqlgv675gejjjy7n3h6wz2n/bin/gmx_mpi mdrun -ntomp 10 -v -deffnm run10 -multidir 1/ 2/ 3/ 4/",
			"__process_effective_uid": "1000",
			"__process_exe":           "/usr/bin/vim",
			"__process_pid__":         "46281",
			"__process_real_uid":      "1000",
			"service_name":            "4824887",
		},
	},
	{
		Targets: []string{"4824887"},
		Labels: map[string]string{
			"__process_commandline":   "/gpfslocalsup/spack_soft/gromacs/2022.2/gcc-8.4.1-kblhs7pjrcqlgv675gejjjy7n3h6wz2n/bin/gmx_mpi mdrun -ntomp 10 -v -deffnm run10 -multidir 1/ 2/ 3/ 4/",
			"__process_effective_uid": "1000",
			"__process_exe":           "/usr/bin/vim",
			"__process_pid__":         "46231",
			"__process_real_uid":      "1000",
			"service_name":            "4824887",
		},
	},
}

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
	assert.ElementsMatch(t, expectedTargets, targets)
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
	assert.ElementsMatch(t, expectedTargets, targets)
}
