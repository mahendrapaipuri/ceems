package types

// Models
// Unit is an abstract compute unit that can mean Job (batchjobs), VM (cloud) or Pod (k8s)
type Unit struct {
	ID                  int64   `json:"-"                          sql:"id"                         sqlitetype:"integer not null primary key"`
	UUID                string  `json:"uuid"                       sql:"uuid"                       sqlitetype:"text"`    // Unique identifier of unit. It can be Job ID for batch jobs, UUID for pods in k8s or VMs in Openstack
	Partition           string  `json:"partition"                  sql:"partition"                  sqlitetype:"text"`    // Name of partition. Only for HPC batchjobs
	QoS                 string  `json:"qos"                        sql:"qos"                        sqlitetype:"text"`    // Name of QoS. Only for HPC batchjobs
	Project             string  `json:"project"                    sql:"project"                    sqlitetype:"text"`    // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Grp                 string  `json:"grp"                        sql:"grp"                        sqlitetype:"text"`    // User group
	Gid                 int64   `json:"gid"                        sql:"gid"                        sqlitetype:"integer"` // User group GID: Only for HPC batchjobs
	Usr                 string  `json:"usr"                        sql:"usr"                        sqlitetype:"text"`    // Username
	Uid                 int64   `json:"uid"                        sql:"uid"                        sqlitetype:"integer"` // User UID: Only for HPC batchjobs
	Submit              string  `json:"submit"                     sql:"submit"                     sqlitetype:"text"`    // Submission time
	Start               string  `json:"start"                      sql:"start"                      sqlitetype:"text"`    // Start time
	End                 string  `json:"end"                        sql:"end"                        sqlitetype:"text"`    // End time
	SubmitTS            int64   `json:"submit_ts"                  sql:"submit_ts"                  sqlitetype:"integer"` // Submission timestamp
	StartTS             int64   `json:"start_ts"                   sql:"start_ts"                   sqlitetype:"integer"` // Start timestamp
	EndTS               int64   `json:"end_ts"                     sql:"end_ts"                     sqlitetype:"integer"` // End timestamp
	Elapsed             string  `json:"elapsed"                    sql:"elapsed"                    sqlitetype:"text"`    // Total elapsed time
	ElapsedRaw          int64   `json:"elapsed_raw"                sql:"elapsed_raw"                sqlitetype:"integer"` // Total elapsed time in seconds
	Exitcode            string  `json:"exitcode"                   sql:"exitcode"                   sqlitetype:"text"`    // Exit code of unit
	State               string  `json:"state"                      sql:"state"                      sqlitetype:"text"`    // Current state of unit
	AllocNodes          int     `json:"alloc_nodes"                sql:"alloc_nodes"                sqlitetype:"integer"` // Allocated number of nodes: Only for HPC batchjobs
	AllocCPUs           int     `json:"alloc_cpus"                 sql:"alloc_cpus"                 sqlitetype:"integer"` // Allocated number of CPUs
	AllocMem            string  `json:"alloc_mem"                  sql:"alloc_mem"                  sqlitetype:"text"`    // Allocated memory
	AllocGPUs           int     `json:"alloc_gpus"                 sql:"alloc_gpus"                 sqlitetype:"integer"` // Allocated number of GPUs
	Nodelist            string  `json:"nodelist"                   sql:"nodelist"                   sqlitetype:"text"`    // List of nodes in job: Only for HPC batchjobs
	NodelistExp         string  `json:"nodelist_exp"               sql:"nodelist_exp"               sqlitetype:"text"`    // Expanded list of nodes in job: only for HPC batchjobs
	Name                string  `json:"name"                       sql:"name"                       sqlitetype:"text"`    // Name of compute unit
	WorkDir             string  `json:"workdir"                    sql:"workdir"                    sqlitetype:"text"`    // Current working directory: Only for HPC batchjobs
	TotalCPUBilling     int64   `json:"total_cpu_billing"          sql:"total_cpu_billing"          sqlitetype:"integer"` // Total CPU billing for unit
	TotalGPUBilling     int64   `json:"total_gpu_billing"          sql:"total_gpu_billing"          sqlitetype:"integer"` // Total GPU billing for unit
	TotalMiscBilling    int64   `json:"total_misc_billing"         sql:"total_misc_billing"         sqlitetype:"integer"` // Total billing for unit that are not in CPU and GPU billing
	AveCPUUsage         float64 `json:"avg_cpu_usage"              sql:"avg_cpu_usage"              sqlitetype:"real"`    // Average CPU usage during lifetime of unit
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`    // Average CPU memory during lifetime of unit
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage_kwh" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`    // Total CPU energy usage in kWh during lifetime of unit
	TotalCPUEmissions   float64 `json:"total_cpu_emissions_gms"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`    // Total CPU emissions in grams during lifetime of unit
	AveGPUUsage         float64 `json:"avg_gpu_usage"              sql:"avg_gpu_usage"              sqlitetype:"real"`    // Average GPU usage during lifetime of unit
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`    // Average GPU memory during lifetime of unit
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage_kwh" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`    // Total GPU energy usage in kWh during lifetime of unit
	TotalGPUEmissions   float64 `json:"total_gpu_emissions_gms"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`    // Total GPU emissions in grams during lifetime of unit
	TotalIOWriteHot     float64 `json:"total_io_write_hot_gb"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`    // Total IO write on hot storage in GB during lifetime of unit
	TotalIOReadHot      float64 `json:"total_io_read_hot_gb"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`    // Total IO read on hot storage in GB during lifetime of unit
	TotalIOWriteCold    float64 `json:"total_io_write_cold_gb"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`    // Total IO write on cold storage in GB during lifetime of unit
	TotalIOReadCold     float64 `json:"total_io_read_cold_gb"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`    // Total IO read on cold storage in GB during lifetime of unit
	TotalIngress        float64 `json:"total_ingress_in_gb"        sql:"total_ingress_in_gb"        sqlitetype:"real"`    // Total ingress traffic in GB of unit
	TotalOutgress       float64 `json:"total_outgress_in_gb"       sql:"total_outgress_in_gb"       sqlitetype:"real"`    // Total outgress traffic in GB of unit
	Comment             string  `json:"comment"                    sql:"comment"                    sqlitetype:"blob"`    // A JSON string to store other generic info
	Ignore              int     `json:"-"                          sql:"ignore"                     sqlitetype:"integer"` // Whether to ignore unit
}

// Global usage statistics of each project/tenant/namespace
type Usage struct {
	ID                  int64   `json:"-"                          sql:"id"                         sqlitetype:"integer not null primary key"`
	NumUnits            int64   `json:"num_units"                  sql:"num_units"                  sqlitetype:"integer"` // Number of consumed units
	Project             string  `json:"project"                    sql:"project"                    sqlitetype:"text"`    // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Usr                 string  `json:"usr"                        sql:"usr"                        sqlitetype:"text"`    // Username
	Partition           string  `json:"partition"                  sql:"partition"                  sqlitetype:"text"`    // Name of partition. Only for HPC batchjobs
	QoS                 string  `json:"qos"                        sql:"qos"                        sqlitetype:"text"`    // Name of QoS. Only for HPC batchjobs
	TotalCPUBilling     int64   `json:"total_cpu_billing"          sql:"total_cpu_billing"          sqlitetype:"integer"` // Total CPU billing for project
	TotalGPUBilling     int64   `json:"total_gpu_billing"          sql:"total_gpu_billing"          sqlitetype:"integer"` // Total GPU billing for project
	TotalMiscBilling    int64   `json:"total_misc_billing"         sql:"total_misc_billing"         sqlitetype:"integer"` // Total billing for project that are not in CPU and GPU billing
	AveCPUUsage         float64 `json:"avg_cpu_usage"              sql:"avg_cpu_usage"              sqlitetype:"real"`    // Average CPU usage during lifetime of project
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`    // Average CPU memory during lifetime of project
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage_kwh" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`    // Total CPU energy usage in kWh during lifetime of project
	TotalCPUEmissions   float64 `json:"total_cpu_emissions_gms"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`    // Total CPU emissions in grams during lifetime of project
	AveGPUUsage         float64 `json:"avg_gpu_usage"              sql:"avg_gpu_usage"              sqlitetype:"real"`    // Average GPU usage during lifetime of project
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`    // Average GPU memory during lifetime of project
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage_kwh" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`    // Total GPU energy usage in kWh during lifetime of project
	TotalGPUEmissions   float64 `json:"total_gpu_emissions_gms"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`    // Total GPU emissions in grams during lifetime of project
	TotalIOWriteHot     float64 `json:"total_io_write_hot_gb"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`    // Total IO write on hot storage in GB during lifetime of project
	TotalIOReadHot      float64 `json:"total_io_read_hot_gb"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`    // Total IO read on hot storage in GB during lifetime of project
	TotalIOWriteCold    float64 `json:"total_io_write_cold_gb"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`    // Total IO write on cold storage in GB during lifetime of project
	TotalIOReadCold     float64 `json:"total_io_read_cold_gb"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`    // Total IO read on cold storage in GB during lifetime of project
	TotalIngress        float64 `json:"total_ingress_in_gb"        sql:"total_ingress_in_gb"        sqlitetype:"real"`    // Total ingress traffic in GB of project
	TotalOutgress       float64 `json:"total_outgress_in_gb"       sql:"total_outgress_in_gb"       sqlitetype:"real"`    // Total outgress traffic in GB of project
	Comment             string  `json:"comment"                    sql:"comment"                    sqlitetype:"blob"`    // A JSON string to store other generic info
}

// Project struct
type Project struct {
	Name string `json:"name,omitempty" sql:"project" sqlitetype:"text"`
}
