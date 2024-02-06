package tsdb

const (
	avgCpuUsageQuery = `
avg_over_time(
	avg by (jobid) (
		(
			rate(batchjob_slurm_job_cpu_user_seconds{jobid=~"%[1]s"}[%[2]s])
			+
			rate(batchjob_slurm_job_cpu_system_seconds{jobid=~"%[1]s"}[%[2]s])
		)
		/
		batchjob_slurm_job_cpus{jobid=~"%[1]s"}
	)[%[3]s:]
) * 100`

	avgCpuMemUsageQuery = `
avg_over_time(
	avg by (jobid) (
		batchjob_slurm_job_memory_used_bytes{jobid=~"%[1]s"}
		/
		batchjob_slurm_job_memory_total_bytes{jobid=~"%[1]s"}
	)[%[3]s:]
) * 100`

	totalCpuEnergyUsageQuery = `
sum_over_time(
	sum by (jobid) (
		batchjob_ipmi_dcmi_current_watts_total * %[5]d / 3.6e9
		* on (instance) group_right ()
		(
			rate(batchjob_slurm_job_cpu_user_seconds{jobid=~"%[1]s"}[%[2]s])
			+
			rate(batchjob_slurm_job_cpu_system_seconds{jobid=~"%[1]s"}[%[2]s])
		)
	/ on (instance) group_left ()
		sum by (instance) (rate(batchjob_cpu_seconds_total{mode!~"idle|iowait|steal"}[%[2]s]))
	)[%[3]s:%[4]s]
)`

	totalCpuEmissionsUsageQuery = `
sum_over_time(
	sum by (jobid) (
	label_replace(
		label_replace(
				batchjob_ipmi_dcmi_current_watts_total * %[5]d / 3.6e9
			* on (instance) group_right ()
				(
					rate(batchjob_slurm_job_cpu_user_seconds{jobid=~"%[1]s"}[%[2]s])
				+
					rate(batchjob_slurm_job_cpu_system_seconds{jobid=~"%[1]s"}[%[2]s])
				)
			/ on (instance) group_left ()
			sum by (instance) (rate(batchjob_cpu_seconds_total{mode!~"idle|iowait|steal"}[%[2]s])),
			"common_label",
			"mock",
			"hostname",
			"(.*)"
		)
		* on (common_label) group_left ()
		label_replace(
			batchjob_emissions_gCo2_kWh{provider="rte"},
			"common_label",
			"mock",
			"hostname",
			"(.*)"
		),
		"provider",
		"${provider:raw}",
		"instance",
		"(.*)"
	)
	)[%[3]s:%[4]s]
)`

	avgGpuUsageQuery = `
avg_over_time(
	avg by (jobid) (
		DCGM_FI_DEV_GPU_UTIL
		* on (UUID) group_right ()
		label_replace(
			batchjob_slurm_job_gpu_index_flag{jobid=~"%[1]s"},
			"UUID",
			"$1",
			"uuid",
			"(.*)"
		)
	)[%[3]s:%[4]s]
)`

	avgGpuMemUsageQuery = `
avg_over_time(
	avg by (jobid) (
		DCGM_FI_DEV_MEM_COPY_UTIL
		* on (UUID) group_right ()
		label_replace(
			batchjob_slurm_job_gpu_index_flag{jobid=~"%[1]s"},
			"UUID",
			"$1",
			"uuid",
			"(.*)"
		)
	)[%[3]s:%[4]s]
)`

	totalGpuEnergyUsageQuery = `
sum_over_time(
	sum by (jobid) (
		DCGM_FI_DEV_POWER_USAGE * %[5]d / 3.6e9
		* on (UUID) group_right()
		batchjob_slurm_job_gpu_index_flag{jobid=~"%[1]s"}
	)[%[3]s:%[4]s]
)`

	totalGpuEmissionsUsageQuery = `
sum_over_time(
	sum by (jobid) (
		label_replace(
			DCGM_FI_DEV_POWER_USAGE * %[5]d / 3.6e+09
			* on (UUID) group_right ()
			batchjob_slurm_job_gpu_index_flag{jobid=~"%[1]s"},
			"common_label",
			"mock",
			"instance",
			"(.*)"
		)
		* on (common_label) group_left ()
		label_replace(
			batchjob_emissions_gCo2_kWh{provider="rte"},
			"common_label",
			"mock",
			"instance",
			"(.*)"
		)
	)[%[3]s:%[4]s]
)`
)

// TSDB queries to get aggregate metrics of jobs
var aggMetricQueries = map[string]string{
	"cpuUsage":       avgCpuUsageQuery,
	"cpuMemUsage":    avgCpuMemUsageQuery,
	"cpuEnergyUsage": totalCpuEnergyUsageQuery,
	"cpuEmissions":   totalCpuEmissionsUsageQuery,
	"gpuUsage":       avgGpuUsageQuery,
	"gpuMemUsage":    avgGpuMemUsageQuery,
	"gpuEnergyUsage": totalGpuEnergyUsageQuery,
	"gpuEmissions":   totalGpuEmissionsUsageQuery,
}
