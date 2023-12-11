package jobstats

var (
	checksMap = map[string]interface{}{
		"slurm": slurmChecks,
	}
	statsMap = map[string]interface{}{
		"slurm": getSlurmJobs,
	}
)
