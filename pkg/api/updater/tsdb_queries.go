package updater

const (
	avgCPUUsageQuery = `
avg_over_time(
	avg by (uuid) (
		(
			rate(ceems_slurm_job_cpu_user_seconds{uuid=~"%[1]s"}[%[2]s])
			+
			rate(ceems_slurm_job_cpu_system_seconds{uuid=~"%[1]s"}[%[2]s])
		)
		/
		ceems_slurm_job_cpus{uuid=~"%[1]s"}
	)[%[3]s:]
) * 100`

	avgCPUMemUsageQuery = `
avg_over_time(
	avg by (uuid) (
		ceems_slurm_job_memory_used_bytes{uuid=~"%[1]s"}
		/
		ceems_slurm_job_memory_total_bytes{uuid=~"%[1]s"}
	)[%[3]s:]
) * 100`

	totalCPUEnergyUsageQuery = `
sum_over_time(
	sum by (uuid) (
		instance:ceems_ipmi_dcmi_avg_watts:cpu * %[5]d / 3.6e9
		* on (instance) group_right ()
		(
			rate(ceems_slurm_job_cpu_user_seconds{uuid=~"%[1]s"}[%[2]s])
			+
			rate(ceems_slurm_job_cpu_system_seconds{uuid=~"%[1]s"}[%[2]s])
		)
	/ on (instance) group_left ()
		sum by (instance) (rate(ceems_cpu_seconds_total{mode!~"idle|iowait|steal"}[%[2]s]))
	)[%[3]s:%[4]s]
)`

	totalCPUEmissionsUsageQuery = `
sum_over_time(
	sum by (uuid) (
	label_replace(
		label_replace(
				instance:ceems_ipmi_dcmi_avg_watts:cpu * %[5]d / 3.6e9
			* on (instance) group_right ()
				(
					rate(ceems_slurm_job_cpu_user_seconds{uuid=~"%[1]s"}[%[2]s])
				+
					rate(ceems_slurm_job_cpu_system_seconds{uuid=~"%[1]s"}[%[2]s])
				)
			/ on (instance) group_left ()
			sum by (instance) (rate(ceems_cpu_seconds_total{mode!~"idle|iowait|steal"}[%[2]s])),
			"common_label",
			"mock",
			"hostname",
			"(.*)"
		)
		* on (common_label) group_left ()
		label_replace(
			ceems_emissions_gCo2_kWh{provider="rte"},
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

	avgGPUUsageQuery = `
avg_over_time(
	avg by (uuid) (
		DCGM_FI_DEV_GPU_UTIL
		* on (gpuuuid) group_right ()
		ceems_slurm_job_gpu_index_flag{uuid=~"%[1]s"}
	)[%[3]s:%[4]s]
)`

	avgGPUMemUsageQuery = `
avg_over_time(
	avg by (uuid) (
		DCGM_FI_DEV_MEM_COPY_UTIL
		* on (gpuuuid) group_right ()
		ceems_slurm_job_gpu_index_flag{uuid=~"%[1]s"}
	)[%[3]s:%[4]s]
)`

	totalGPUEnergyUsageQuery = `
sum_over_time(
	sum by (uuid) (
		instance:DCGM_FI_DEV_POWER_USAGE:gpu * %[5]d / 3.6e9
		* on (gpuuuid) group_right()
		ceems_slurm_job_gpu_index_flag{uuid=~"%[1]s"}
	)[%[3]s:%[4]s]
)`

	totalGPUEmissionsUsageQuery = `
sum_over_time(
	sum by (uuid) (
		label_replace(
			instance:DCGM_FI_DEV_POWER_USAGE:gpu * %[5]d / 3.6e+09
			* on (gpuuuid) group_right ()
			ceems_slurm_job_gpu_index_flag{uuid=~"%[1]s"},
			"common_label",
			"mock",
			"instance",
			"(.*)"
		)
		* on (common_label) group_left ()
		label_replace(
			ceems_emissions_gCo2_kWh{provider="rte"},
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
	"cpuUsage":       avgCPUUsageQuery,
	"cpuMemUsage":    avgCPUMemUsageQuery,
	"cpuEnergyUsage": totalCPUEnergyUsageQuery,
	"cpuEmissions":   totalCPUEmissionsUsageQuery,
	"gpuUsage":       avgGPUUsageQuery,
	"gpuMemUsage":    avgGPUMemUsageQuery,
	"gpuEnergyUsage": totalGPUEnergyUsageQuery,
	"gpuEmissions":   totalGPUEmissionsUsageQuery,
}
