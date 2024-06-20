// Package models defines different models used in stats
package models

import (
	"github.com/mahendrapaipuri/ceems/internal/structset"
)

const (
	unitsTableName      = "units"
	usageTableName      = "usage"
	projectsTableName   = "projects"
	usersTableName      = "users"
	adminUsersTableName = "admin_users"
)

// Unit is an abstract compute unit that can mean Job (batchjobs), VM (cloud) or Pod (k8s)
type Unit struct {
	ID                  int64      `json:"-"                                    sql:"id"                         sqlitetype:"integer not null primary key"`
	ClusterID           string     `json:"cluster_id,omitempty"                 sql:"cluster_id"                 sqlitetype:"text"`    // Identifier of the resource manager that owns compute unit. It is used to differentiate multiple clusters of same resource manager.
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
	State               string     `json:"state,omitempty"                      sql:"state"                      sqlitetype:"text"`    // Current state of unit
	Allocation          Allocation `json:"allocation,omitempty"                 sql:"allocation"                 sqlitetype:"text"`    // Allocation map of unit. Only string and int64 values are supported in map
	TotalWallTime       int64      `json:"total_walltime_seconds,omitempty"     sql:"total_walltime_seconds"     sqlitetype:"integer"` // Total elapsed wall time in seconds
	TotalCPUTime        int64      `json:"total_cputime_seconds,omitempty"      sql:"total_cputime_seconds"      sqlitetype:"integer"` // Total number of CPU seconds consumed by the unit
	TotalGPUTime        int64      `json:"total_gputime_seconds,omitempty"      sql:"total_gputime_seconds"      sqlitetype:"integer"` // Total number of GPU seconds consumed by the unit
	TotalCPUMemTime     int64      `json:"-"                                    sql:"total_cpumemtime_seconds"   sqlitetype:"integer"` // Total number of CPU memory (in MB) seconds consumed by the unit. This is used internally to update aggregate metrics
	TotalGPUMemTime     int64      `json:"-"                                    sql:"total_gpumemtime_seconds"   sqlitetype:"integer"` // Total number of GPU memory (in MB) seconds consumed by the unit. This is used internally to update aggregate metrics
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
	LastUpdatedAt       string     `json:"-"                                    sql:"last_updated_at"            sqlitetype:"text"`    // Last updated time. It can be used to clean up DB
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
	ID                  int64     `json:"-"                                    sql:"id"                         sqlitetype:"integer not null primary key"`
	ClusterID           string    `json:"cluster_id,omitempty"                 sql:"cluster_id"                 sqlitetype:"text"`    // Identifier of the resource manager that owns compute unit. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager     string    `json:"resource_manager,omitempty"           sql:"resource_manager"           sqlitetype:"text"`    // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	NumUnits            int64     `json:"num_units"                            sql:"num_units"                  sqlitetype:"integer"` // Number of consumed units
	Project             string    `json:"project"                              sql:"project"                    sqlitetype:"text"`    // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Usr                 string    `json:"usr"                                  sql:"usr"                        sqlitetype:"text"`    // Username
	LastUpdatedAt       string    `json:"-"                                    sql:"last_updated_at"            sqlitetype:"text"`    // Last updated time. It can be used to clean up DB
	TotalWallTime       int64     `json:"total_walltime_seconds,omitempty"     sql:"total_walltime_seconds"     sqlitetype:"integer"` // Total elapsed wall time in seconds consumed by the project
	TotalCPUTime        int64     `json:"total_cputime_seconds,omitempty"      sql:"total_cputime_seconds"      sqlitetype:"integer"` // Total number of CPU seconds consumed by the project
	TotalGPUTime        int64     `json:"total_gputime_seconds,omitempty"      sql:"total_gputime_seconds"      sqlitetype:"integer"` // Total number of GPU seconds consumed by the project
	TotalCPUMemTime     int64     `json:"total_cpumemtime_seconds,omitempty"   sql:"total_cpumemtime_seconds"   sqlitetype:"integer"` // Total number of CPU memory (in MB) seconds consumed by the project
	TotalGPUMemTime     int64     `json:"total_gpumemtime_seconds,omitempty"   sql:"total_gpumemtime_seconds"   sqlitetype:"integer"` // Total number of GPU memory (in MB) seconds consumed by the project
	AveCPUUsage         JSONFloat `json:"avg_cpu_usage,omitempty"              sql:"avg_cpu_usage"              sqlitetype:"real"`    // Average CPU usage during lifetime of project
	AveCPUMemUsage      JSONFloat `json:"avg_cpu_mem_usage,omitempty"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`    // Average CPU memory during lifetime of project
	TotalCPUEnergyUsage JSONFloat `json:"total_cpu_energy_usage_kwh,omitempty" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`    // Total CPU energy usage in kWh during lifetime of project
	TotalCPUEmissions   JSONFloat `json:"total_cpu_emissions_gms,omitempty"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`    // Total CPU emissions in grams during lifetime of project
	AveGPUUsage         JSONFloat `json:"avg_gpu_usage,omitempty"              sql:"avg_gpu_usage"              sqlitetype:"real"`    // Average GPU usage during lifetime of project
	AveGPUMemUsage      JSONFloat `json:"avg_gpu_mem_usage,omitempty"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`    // Average GPU memory during lifetime of project
	TotalGPUEnergyUsage JSONFloat `json:"total_gpu_energy_usage_kwh,omitempty" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`    // Total GPU energy usage in kWh during lifetime of project
	TotalGPUEmissions   JSONFloat `json:"total_gpu_emissions_gms,omitempty"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`    // Total GPU emissions in grams during lifetime of project
	TotalIOWriteHot     JSONFloat `json:"total_io_write_hot_gb,omitempty"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`    // Total IO write on hot storage in GB during lifetime of project
	TotalIOReadHot      JSONFloat `json:"total_io_read_hot_gb,omitempty"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`    // Total IO read on hot storage in GB during lifetime of project
	TotalIOWriteCold    JSONFloat `json:"total_io_write_cold_gb,omitempty"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`    // Total IO write on cold storage in GB during lifetime of project
	TotalIOReadCold     JSONFloat `json:"total_io_read_cold_gb,omitempty"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`    // Total IO read on cold storage in GB during lifetime of project
	TotalIngress        JSONFloat `json:"total_ingress_in_gb,omitempty"        sql:"total_ingress_in_gb"        sqlitetype:"real"`    // Total ingress traffic in GB of project
	TotalOutgress       JSONFloat `json:"total_outgress_in_gb,omitempty"       sql:"total_outgress_in_gb"       sqlitetype:"real"`    // Total outgress traffic in GB of project
	NumUpdates          int64     `json:"-"                                    sql:"num_updates"                sqlitetype:"integer"` // Number of updates. This is used internally to update aggregate metrics
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

// Project is the container for a given account/tenant/namespace of cluster
type Project struct {
	ID              int64  `json:"-"                sql:"id"               sqlitetype:"integer not null primary key"`
	UID             string `json:"uid,omitempty"    sql:"uid"              sqlitetype:"text"` // Unique identifier of the project provided by cluster
	ClusterID       string `json:"cluster_id"       sql:"cluster_id"       sqlitetype:"text"` // Identifier of the resource manager that owns project. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager string `json:"resource_manager" sql:"resource_manager" sqlitetype:"text"` // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	Name            string `json:"name"             sql:"name"             sqlitetype:"text"` // Name of the project
	Users           List   `json:"users"            sql:"users"            sqlitetype:"text"` // List of users of the project
	Tags            List   `json:"tags,omitempty"   sql:"tags"             sqlitetype:"text"` // List of meta data tags of the project
	LastUpdatedAt   string `json:"-"                sql:"last_updated_at"  sqlitetype:"text"` // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (Project) TableName() string {
	return projectsTableName
}

// TagNames returns a slice of all tag names.
func (p Project) TagNames(tag string) []string {
	return structset.GetStructFieldTagValues(p, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (p Project) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.GetStructFieldTagMap(p, keyTag, valueTag)
}

// User is the container for a given user of cluster
type User struct {
	ID              int64  `json:"-"                sql:"id"               sqlitetype:"integer not null primary key"`
	UID             string `json:"uid,omitempty"    sql:"uid"              sqlitetype:"text"` // Unique identifier of the user provided by cluster
	ClusterID       string `json:"cluster_id"       sql:"cluster_id"       sqlitetype:"text"` // Identifier of the resource manager that owns user. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager string `json:"resource_manager" sql:"resource_manager" sqlitetype:"text"` // Name of the resource manager that owns user. Eg slurm, openstack, kubernetes, etc
	Name            string `json:"name"             sql:"name"             sqlitetype:"text"` // Name of the user
	Projects        List   `json:"projects"         sql:"projects"         sqlitetype:"text"` // List of projects of the user
	Tags            List   `json:"tags,omitempty"   sql:"tags"             sqlitetype:"text"` // List of meta data tags of the user
	LastUpdatedAt   string `json:"-"                sql:"last_updated_at"  sqlitetype:"text"` // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (User) TableName() string {
	return usersTableName
}

// TagNames returns a slice of all tag names.
func (u User) TagNames(tag string) []string {
	return structset.GetStructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u User) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.GetStructFieldTagMap(u, keyTag, valueTag)
}

// AdminUsers from different sources
type AdminUsers struct {
	ID            int64  `json:"-"      sql:"id"              sqlitetype:"integer not null primary key"`
	Source        string `json:"source" sql:"source"          sqlitetype:"text"` // Source of admin users
	Users         List   `json:"users"  sql:"users"           sqlitetype:"text"` // List of users. In DB, users will be stored with | delimiter
	LastUpdatedAt string `json:"-"      sql:"last_updated_at" sqlitetype:"text"` // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (AdminUsers) TableName() string {
	return adminUsersTableName
}

// TagNames returns a slice of all tag names.
func (a AdminUsers) TagNames(tag string) []string {
	return structset.GetStructFieldTagValues(a, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (a AdminUsers) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.GetStructFieldTagMap(a, keyTag, valueTag)
}

// // Ownership mode for a given compute unit
// type Ownership struct {
// 	UUID string `json:"uuid"` // UUID of the compute unit
// 	Mode string `json:"mode"` // Ownership mode: self when user is owner, project when user belongs to the project of compute unit
// }

// // UnitsOwnership is the container that returns the ownership status of compute units
// type UnitsOwnership struct {
// 	User      string      `json:"user"` // User name
// 	Ownership []Ownership `json:"ownership"`
// }
