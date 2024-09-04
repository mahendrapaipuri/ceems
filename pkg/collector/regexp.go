package collector

import "regexp"

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
