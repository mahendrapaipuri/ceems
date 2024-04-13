// Package models defines different models used in stats
package models

import (
	"github.com/mahendrapaipuri/ceems/internal/structset"
)

const (
	unitsTableName = "units"
	usageTableName = "usage"
)

// Unit is an abstract compute unit that can mean Job (batchjobs), VM (cloud) or Pod (k8s)
type Unit struct {
	ID                  int64      `json:"-"                                    sql:"id"                         sqlitetype:"integer not null primary key"`
	ResourceManager     string     `json:"resource_manager,omitempty"           sql:"resource_manager"           sqlitetype:"text"`    // Name of the resource manager that owns compute unit. Eg slurm, openstack, kubernetes, etc
	UUID                string     `json:"uuid"                                 sql:"uuid"                       sqlitetype:"text"`    // Unique identifier of unit. It can be Job ID for batch jobs, UUID for pods in k8s or VMs in Openstack
	Name                string     `json:"name,omitempty"                       sql:"name"                       sqlitetype:"text"`    // Name of compute unit
	Project             string     `json:"project,omitempty"                    sql:"project"                    sqlitetype:"text"`    // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Grp                 string     `json:"grp,omitempty"                        sql:"grp"                        sqlitetype:"text"`    // User group
	Usr                 string     `json:"usr,omitempty"                        sql:"usr"                        sqlitetype:"text"`    // Username
	CreatedAt           string     `json:"created_at,omitempty"                 sql:"created_at"                 sqlitetype:"text"`    // Creation time
	StartedAt           string     `json:"started_at,omitempty"                 sql:"started_at"                 sqlitetype:"text"`    // Start time
	EndedAt             string     `json:"ended_at,omitempty"                   sql:"ended_at"                   sqlitetype:"text"`    // End time
	CreatedAtTS         int64      `json:"created_at_ts,omitempty"              sql:"created_at_ts"              sqlitetype:"integer"` // Creation timestamp
	StartedAtTS         int64      `json:"started_at_ts,omitempty"              sql:"started_at_ts"              sqlitetype:"integer"` // Start timestamp
	EndedAtTS           int64      `json:"ended_at_ts,omitempty"                sql:"ended_at_ts"                sqlitetype:"integer"` // End timestamp
	Elapsed             string     `json:"elapsed,omitempty"                    sql:"elapsed"                    sqlitetype:"text"`    // Human readable total elapsed time string
	ElapsedRaw          int64      `json:"elapsed_raw,omitempty"                sql:"elapsed_raw"                sqlitetype:"integer"` // Total elapsed time in seconds
	State               string     `json:"state,omitempty"                      sql:"state"                      sqlitetype:"text"`    // Current state of unit
	Allocation          Allocation `json:"allocation,omitempty"                 sql:"allocation"                 sqlitetype:"text"`    // Allocation map of unit. Only string and int64 values are supported in map
	TotalCPUBilling     int64      `json:"total_cpu_billing,omitempty"          sql:"total_cpu_billing"          sqlitetype:"integer"` // Total CPU billing for unit
	TotalGPUBilling     int64      `json:"total_gpu_billing,omitempty"          sql:"total_gpu_billing"          sqlitetype:"integer"` // Total GPU billing for unit
	TotalMiscBilling    int64      `json:"total_misc_billing,omitempty"         sql:"total_misc_billing"         sqlitetype:"integer"` // Total billing for unit that are not in CPU and GPU billing
	AveCPUUsage         JSONFloat  `json:"avg_cpu_usage,omitempty"              sql:"avg_cpu_usage"              sqlitetype:"real"`    // Average CPU usage during lifetime of unit
	AveCPUMemUsage      JSONFloat  `json:"avg_cpu_mem_usage,omitempty"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`    // Average CPU memory during lifetime of unit
	TotalCPUEnergyUsage JSONFloat  `json:"total_cpu_energy_usage_kwh,omitempty" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`    // Total CPU energy usage in kWh during lifetime of unit
	TotalCPUEmissions   JSONFloat  `json:"total_cpu_emissions_gms,omitempty"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`    // Total CPU emissions in grams during lifetime of unit
	AveGPUUsage         JSONFloat  `json:"avg_gpu_usage,omitempty"              sql:"avg_gpu_usage"              sqlitetype:"real"`    // Average GPU usage during lifetime of unit
	AveGPUMemUsage      JSONFloat  `json:"avg_gpu_mem_usage,omitempty"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`    // Average GPU memory during lifetime of unit
	TotalGPUEnergyUsage JSONFloat  `json:"total_gpu_energy_usage_kwh,omitempty" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`    // Total GPU energy usage in kWh during lifetime of unit
	TotalGPUEmissions   JSONFloat  `json:"total_gpu_emissions_gms,omitempty"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`    // Total GPU emissions in grams during lifetime of unit
	TotalIOWriteHot     JSONFloat  `json:"total_io_write_hot_gb,omitempty"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`    // Total IO write on hot storage in GB during lifetime of unit
	TotalIOReadHot      JSONFloat  `json:"total_io_read_hot_gb,omitempty"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`    // Total IO read on hot storage in GB during lifetime of unit
	TotalIOWriteCold    JSONFloat  `json:"total_io_write_cold_gb,omitempty"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`    // Total IO write on cold storage in GB during lifetime of unit
	TotalIOReadCold     JSONFloat  `json:"total_io_read_cold_gb,omitempty"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`    // Total IO read on cold storage in GB during lifetime of unit
	TotalIngress        JSONFloat  `json:"total_ingress_in_gb,omitempty"        sql:"total_ingress_in_gb"        sqlitetype:"real"`    // Total ingress traffic in GB of unit
	TotalOutgress       JSONFloat  `json:"total_outgress_in_gb,omitempty"       sql:"total_outgress_in_gb"       sqlitetype:"real"`    // Total outgress traffic in GB of unit
	Tags                Tag        `json:"tags,omitempty"                       sql:"tags"                       sqlitetype:"text"`    // A map to store generic info. String and int64 are valid value types of map
	Ignore              int        `json:"-"                                    sql:"ignore"                     sqlitetype:"integer"` // Whether to ignore unit
	NumUpdates          int64      `json:"-"                                    sql:"num_updates"                sqlitetype:"integer"` // Number of updates. This is used internally to update aggregate metrics
}

// TableName returns the table which units are stored into.
func (Unit) TableName() string {
	return unitsTableName
}

// TagNames returns a slice of all tag names.
func (u Unit) TagNames(tag string) []string {
	return structset.GetStructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u Unit) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.GetStructFieldTagMap(u, keyTag, valueTag)
}

// Usage statistics of each project/tenant/namespace
type Usage struct {
	ID                  int64     `json:"-"                          sql:"id"                         sqlitetype:"integer not null primary key"`
	ResourceManager     string    `json:"resource_manager"           sql:"resource_manager"           sqlitetype:"text"`    // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	NumUnits            int64     `json:"num_units"                  sql:"num_units"                  sqlitetype:"integer"` // Number of consumed units
	Project             string    `json:"project"                    sql:"project"                    sqlitetype:"text"`    // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Usr                 string    `json:"usr"                        sql:"usr"                        sqlitetype:"text"`    // Username
	LastUpdatedAt       string    `json:"-"                          sql:"last_updated_at"            sqlitetype:"text"`    // Last updated time. It can be used to clean up DB
	TotalCPUBilling     int64     `json:"total_cpu_billing"          sql:"total_cpu_billing"          sqlitetype:"integer"` // Total CPU billing for project
	TotalGPUBilling     int64     `json:"total_gpu_billing"          sql:"total_gpu_billing"          sqlitetype:"integer"` // Total GPU billing for project
	TotalMiscBilling    int64     `json:"total_misc_billing"         sql:"total_misc_billing"         sqlitetype:"integer"` // Total billing for project that are not in CPU and GPU billing
	AveCPUUsage         JSONFloat `json:"avg_cpu_usage"              sql:"avg_cpu_usage"              sqlitetype:"real"`    // Average CPU usage during lifetime of project
	AveCPUMemUsage      JSONFloat `json:"avg_cpu_mem_usage"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`    // Average CPU memory during lifetime of project
	TotalCPUEnergyUsage JSONFloat `json:"total_cpu_energy_usage_kwh" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`    // Total CPU energy usage in kWh during lifetime of project
	TotalCPUEmissions   JSONFloat `json:"total_cpu_emissions_gms"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`    // Total CPU emissions in grams during lifetime of project
	AveGPUUsage         JSONFloat `json:"avg_gpu_usage"              sql:"avg_gpu_usage"              sqlitetype:"real"`    // Average GPU usage during lifetime of project
	AveGPUMemUsage      JSONFloat `json:"avg_gpu_mem_usage"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`    // Average GPU memory during lifetime of project
	TotalGPUEnergyUsage JSONFloat `json:"total_gpu_energy_usage_kwh" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`    // Total GPU energy usage in kWh during lifetime of project
	TotalGPUEmissions   JSONFloat `json:"total_gpu_emissions_gms"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`    // Total GPU emissions in grams during lifetime of project
	TotalIOWriteHot     JSONFloat `json:"total_io_write_hot_gb"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`    // Total IO write on hot storage in GB during lifetime of project
	TotalIOReadHot      JSONFloat `json:"total_io_read_hot_gb"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`    // Total IO read on hot storage in GB during lifetime of project
	TotalIOWriteCold    JSONFloat `json:"total_io_write_cold_gb"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`    // Total IO write on cold storage in GB during lifetime of project
	TotalIOReadCold     JSONFloat `json:"total_io_read_cold_gb"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`    // Total IO read on cold storage in GB during lifetime of project
	TotalIngress        JSONFloat `json:"total_ingress_in_gb"        sql:"total_ingress_in_gb"        sqlitetype:"real"`    // Total ingress traffic in GB of project
	TotalOutgress       JSONFloat `json:"total_outgress_in_gb"       sql:"total_outgress_in_gb"       sqlitetype:"real"`    // Total outgress traffic in GB of project
	NumUpdates          int64     `json:"-"                          sql:"num_updates"                sqlitetype:"integer"` // Number of updates. This is used internally to update aggregate metrics
}

// TableName returns the table which usage stats are stored into.
func (Usage) TableName() string {
	return usageTableName
}

// TagNames returns a slice of all tag names.
func (u Usage) TagNames(tag string) []string {
	return structset.GetStructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u Usage) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.GetStructFieldTagMap(u, keyTag, valueTag)
}

// Project struct
type Project struct {
	Name string `json:"name,omitempty" sql:"project" sqlitetype:"text"`
}
