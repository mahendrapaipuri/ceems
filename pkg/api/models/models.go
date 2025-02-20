// Package models defines different models used in stats
package models

import (
	"github.com/mahendrapaipuri/ceems/internal/structset"
)

const (
	unitsTableName      = "units"
	usageTableName      = "usage"
	dailyUsageTableName = "daily_usage"
	projectsTableName   = "projects"
	usersTableName      = "users"
	adminUsersTableName = "admin_users"
)

// Unit is an abstract compute unit that can mean Job (batchjobs), VM (cloud) or Pod (k8s).
type Unit struct {
	ID                  int64      `json:"-"                                                                                              sql:"id"                                    sqlitetype:"integer not null primary key"`
	ClusterID           string     `json:"cluster_id,omitempty"                                                                           sql:"cluster_id"                            sqlitetype:"text"`                                                                       // Identifier of the resource manager that owns compute unit. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager     string     `json:"resource_manager,omitempty"                                                                     sql:"resource_manager"                      sqlitetype:"text"`                                                                       // Name of the resource manager that owns compute unit. Eg slurm, openstack, kubernetes, etc
	UUID                string     `json:"uuid"                                                                                           sql:"uuid"                                  sqlitetype:"text"`                                                                       // Unique identifier of unit. It can be Job ID for batch jobs, UUID for pods in k8s or VMs in Openstack
	Name                string     `json:"name,omitempty"                                                                                 sql:"name"                                  sqlitetype:"text"`                                                                       // Name of compute unit
	Project             string     `json:"project,omitempty"                                                                              sql:"project"                               sqlitetype:"text"`                                                                       // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Group               string     `json:"groupname,omitempty"                                                                            sql:"groupname"                             sqlitetype:"text"`                                                                       // User group
	User                string     `json:"username,omitempty"                                                                             sql:"username"                              sqlitetype:"text"`                                                                       // Username
	CreatedAt           string     `json:"created_at,omitempty"                                                                           sql:"created_at"                            sqlitetype:"text"`                                                                       // Creation time
	StartedAt           string     `json:"started_at,omitempty"                                                                           sql:"started_at"                            sqlitetype:"text"`                                                                       // Start time
	EndedAt             string     `json:"ended_at,omitempty"                                                                             sql:"ended_at"                              sqlitetype:"text"`                                                                       // End time
	CreatedAtTS         int64      `json:"created_at_ts,omitempty"                                                                        sql:"created_at_ts"                         sqlitetype:"integer"`                                                                    // Creation timestamp
	StartedAtTS         int64      `json:"started_at_ts,omitempty"                                                                        sql:"started_at_ts"                         sqlitetype:"integer"`                                                                    // Start timestamp
	EndedAtTS           int64      `json:"ended_at_ts,omitempty"                                                                          sql:"ended_at_ts"                           sqlitetype:"integer"`                                                                    // End timestamp
	Elapsed             string     `json:"elapsed,omitempty"                                                                              sql:"elapsed"                               sqlitetype:"text"`                                                                       // Human readable total elapsed time string
	State               string     `json:"state,omitempty"                                                                                sql:"state"                                 sqlitetype:"text"`                                                                       // Current state of unit
	Allocation          Allocation `example:"cpus:1,mem:10,gpus:1"                                                                        json:"allocation,omitempty"                 sql:"allocation"                          sqlitetype:"text" swaggertype:"object,number"` // Allocation map of unit. Only string and int64 values are supported in map
	TotalTime           MetricMap  `example:"walltime:100,alloc_cputime:100,alloc_cpumemtime:1000,alloc_gputime:100,alloc_gpumemtime:100" json:"total_time_seconds,omitempty"         sql:"total_time_seconds"                  sqlitetype:"text" swaggertype:"object,number"` // Different types of times in seconds consumed by the unit. This map contains at minimum `walltime`, `alloc_cputime`, `alloc_cpumemtime`, `alloc_gputime` and `alloc_gpumem_time` keys.
	AveCPUUsage         MetricMap  `example:"global:70.12"                                                                                json:"avg_cpu_usage,omitempty"              sql:"avg_cpu_usage"                       sqlitetype:"text" swaggertype:"object,number"` // Average CPU usage(s) during lifetime of unit
	AveCPUMemUsage      MetricMap  `example:"global:45.26"                                                                                json:"avg_cpu_mem_usage,omitempty"          sql:"avg_cpu_mem_usage"                   sqlitetype:"text" swaggertype:"object,number"` // Average CPU memory usage(s) during lifetime of unit
	TotalCPUEnergyUsage MetricMap  `example:"total:0.73"                                                                                  json:"total_cpu_energy_usage_kwh,omitempty" sql:"total_cpu_energy_usage_kwh"          sqlitetype:"text" swaggertype:"object,number"` // Total CPU energy usage(s) in kWh during lifetime of unit
	TotalCPUEmissions   MetricMap  `example:"owid_total:5.22,emaps_total:3.09"                                                            json:"total_cpu_emissions_gms,omitempty"    sql:"total_cpu_emissions_gms"             sqlitetype:"text" swaggertype:"object,number"` // Total CPU emissions from source(s) in grams during lifetime of unit
	AveGPUUsage         MetricMap  `example:"global:70.12"                                                                                json:"avg_gpu_usage,omitempty"              sql:"avg_gpu_usage"                       sqlitetype:"text" swaggertype:"object,number"` // Average GPU usage(s) during lifetime of unit
	AveGPUMemUsage      MetricMap  `example:"global:45.26"                                                                                json:"avg_gpu_mem_usage,omitempty"          sql:"avg_gpu_mem_usage"                   sqlitetype:"text" swaggertype:"object,number"` // Average GPU memory usage(s) during lifetime of unit
	TotalGPUEnergyUsage MetricMap  `example:"total:5.39"                                                                                  json:"total_gpu_energy_usage_kwh,omitempty" sql:"total_gpu_energy_usage_kwh"          sqlitetype:"text" swaggertype:"object,number"` // Total GPU energy usage(s) in kWh during lifetime of unit
	TotalGPUEmissions   MetricMap  `example:"owid_total:15.22,emaps_total:12.09"                                                          json:"total_gpu_emissions_gms,omitempty"    sql:"total_gpu_emissions_gms"             sqlitetype:"text" swaggertype:"object,number"` // Total GPU emissions from source(s) in grams during lifetime of unit
	TotalIOWriteStats   MetricMap  `example:"total:1.2"                                                                                   json:"total_io_write_stats,omitempty"       sql:"total_io_write_stats"                sqlitetype:"text" swaggertype:"object,number"` // Total IO write statistics during lifetime of unit
	TotalIOReadStats    MetricMap  `example:"total:4.6"                                                                                   json:"total_io_read_stats,omitempty"        sql:"total_io_read_stats"                 sqlitetype:"text" swaggertype:"object,number"` // Total IO read statistics GB during lifetime of unit
	TotalIngressStats   MetricMap  `example:"total:0.5"                                                                                   json:"total_ingress_stats,omitempty"        sql:"total_ingress_stats"                 sqlitetype:"text" swaggertype:"object,number"` // Total Ingress statistics of unit
	TotalOutgressStats  MetricMap  `example:"total:0.1"                                                                                   json:"total_outgress_stats,omitempty"       sql:"total_outgress_stats"                sqlitetype:"text" swaggertype:"object,number"` // Total Outgress statistics of unit
	Tags                Tag        `example:"uid:1000,gid:1000,workdir:/home/user"                                                        json:"tags,omitempty"                       sql:"tags"                                sqlitetype:"text" swaggertype:"object,string"` // A map to store generic info. String and int64 are valid value types of map
	Ignore              int        `json:"-"                                                                                              sql:"ignore"                                sqlitetype:"integer"`                                                                    // Whether to ignore unit
	NumUpdates          int64      `json:"-"                                                                                              sql:"num_updates"                           sqlitetype:"integer"`                                                                    // Number of updates. This is used internally to update aggregate metrics
	LastUpdatedAt       string     `json:"-"                                                                                              sql:"last_updated_at"                       sqlitetype:"text"`                                                                       // Last updated time. It can be used to clean up DB
}

// TableName returns the table which units are stored into.
func (Unit) TableName() string {
	return unitsTableName
}

// TagNames returns a slice of all tag names.
func (u Unit) TagNames(tag string) []string {
	return structset.StructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u Unit) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(u, keyTag, valueTag)
}

// Usage statistics of each project/tenant/namespace.
type Usage struct {
	ID                  int64     `json:"-"                                                                                              sql:"id"                                    sqlitetype:"integer not null primary key"`
	ClusterID           string    `json:"cluster_id"                                                                                     sql:"cluster_id"                            sqlitetype:"text"`                                                                       // Identifier of the resource manager that owns compute unit. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager     string    `json:"resource_manager"                                                                               sql:"resource_manager"                      sqlitetype:"text"`                                                                       // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	NumUnits            int64     `json:"num_units"                                                                                      sql:"num_units"                             sqlitetype:"integer"`                                                                    // Number of consumed units
	Project             string    `json:"project"                                                                                        sql:"project"                               sqlitetype:"text"`                                                                       // Account in batch systems, Tenant in Openstack, Namespace in k8s
	Group               string    `json:"groupname"                                                                                      sql:"groupname"                             sqlitetype:"text"`                                                                       // User group
	User                string    `json:"username"                                                                                       sql:"username"                              sqlitetype:"text"`                                                                       // Username
	LastUpdatedAt       string    `json:"-"                                                                                              sql:"last_updated_at"                       sqlitetype:"text"`                                                                       // Last updated time. It can be used to clean up DB
	TotalTime           MetricMap `example:"walltime:100,alloc_cputime:100,alloc_cpumemtime:1000,alloc_gputime:100,alloc_gpumemtime:100" json:"total_time_seconds,omitempty"         sql:"total_time_seconds"                  sqlitetype:"text" swaggertype:"object,number"` // Different times in seconds consumed by the unit. This map must contain `walltime`, `alloc_cputime`, `alloc_cpumemtime`, `alloc_gputime` and `alloc_gpumem_time` keys.
	AveCPUUsage         MetricMap `example:"global:70.12"                                                                                json:"avg_cpu_usage,omitempty"              sql:"avg_cpu_usage"                       sqlitetype:"text" swaggertype:"object,number"` // Average CPU usage(s) during lifetime of project
	AveCPUMemUsage      MetricMap `example:"global:45.26"                                                                                json:"avg_cpu_mem_usage,omitempty"          sql:"avg_cpu_mem_usage"                   sqlitetype:"text" swaggertype:"object,number"` // Average CPU memory usage(s) during lifetime of project
	TotalCPUEnergyUsage MetricMap `example:"total:0.73"                                                                                  json:"total_cpu_energy_usage_kwh,omitempty" sql:"total_cpu_energy_usage_kwh"          sqlitetype:"text" swaggertype:"object,number"` // Total CPU energy usage(s) in kWh during lifetime of project
	TotalCPUEmissions   MetricMap `example:"owid_total:5.22,emaps_total:3.09"                                                            json:"total_cpu_emissions_gms,omitempty"    sql:"total_cpu_emissions_gms"             sqlitetype:"text" swaggertype:"object,number"` // Total CPU emissions from source(s) in grams during lifetime of project
	AveGPUUsage         MetricMap `example:"global:70.12"                                                                                json:"avg_gpu_usage,omitempty"              sql:"avg_gpu_usage"                       sqlitetype:"text" swaggertype:"object,number"` // Average GPU usage(s) during lifetime of project
	AveGPUMemUsage      MetricMap `example:"global:45.26"                                                                                json:"avg_gpu_mem_usage,omitempty"          sql:"avg_gpu_mem_usage"                   sqlitetype:"text" swaggertype:"object,number"` // Average GPU memory usage(s) during lifetime of project
	TotalGPUEnergyUsage MetricMap `example:"total:5.39"                                                                                  json:"total_gpu_energy_usage_kwh,omitempty" sql:"total_gpu_energy_usage_kwh"          sqlitetype:"text" swaggertype:"object,number"` // Total GPU energy usage(s) in kWh during lifetime of project
	TotalGPUEmissions   MetricMap `example:"owid_total:15.22,emaps_total:12.09"                                                          json:"total_gpu_emissions_gms,omitempty"    sql:"total_gpu_emissions_gms"             sqlitetype:"text" swaggertype:"object,number"` // Total GPU emissions from source(s) in grams during lifetime of project
	TotalIOWriteStats   MetricMap `example:"total:1.2"                                                                                   json:"total_io_write_stats,omitempty"       sql:"total_io_write_stats"                sqlitetype:"text" swaggertype:"object,number"` // Total IO write statistics during lifetime of unit
	TotalIOReadStats    MetricMap `example:"total:4.6"                                                                                   json:"total_io_read_stats,omitempty"        sql:"total_io_read_stats"                 sqlitetype:"text" swaggertype:"object,number"` // Total IO read statistics GB during lifetime of unit
	TotalIngressStats   MetricMap `example:"total:0.5"                                                                                   json:"total_ingress_stats,omitempty"        sql:"total_ingress_stats"                 sqlitetype:"text" swaggertype:"object,number"` // Total Ingress statistics of unit
	TotalOutgressStats  MetricMap `example:"total:0.1"                                                                                   json:"total_outgress_stats,omitempty"       sql:"total_outgress_stats"                sqlitetype:"text" swaggertype:"object,number"` // Total Outgress statistics of unit
	NumUpdates          int64     `json:"-"                                                                                              sql:"num_updates"                           sqlitetype:"text"`                                                                       // Number of updates. This is used internally to update aggregate metrics
}

// TableName returns the table which usage stats are stored into.
func (Usage) TableName() string {
	return usageTableName
}

// TagNames returns a slice of all tag names.
func (u Usage) TagNames(tag string) []string {
	return structset.StructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u Usage) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(u, keyTag, valueTag)
}

// DailyUsage statistics of each project/tenant/namespace.
type DailyUsage struct {
	Usage
}

// TableName returns the table which usage stats are stored into.
func (DailyUsage) TableName() string {
	return dailyUsageTableName
}

// Stat represents high level statistics of each cluster.
type Stat struct {
	ClusterID        string `json:"cluster_id"         sql:"cluster_id"         sqlitetype:"text"`    // Identifier of the resource manager that owns compute unit. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager  string `json:"resource_manager"   sql:"resource_manager"   sqlitetype:"text"`    // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	NumUnits         int64  `json:"num_units"          sql:"num_units"          sqlitetype:"integer"` // Number of active and terminated units
	NumInActiveUnits int64  `json:"num_inactive_units" sql:"num_inactive_units" sqlitetype:"integer"` // Number of inactive units that are in terminated/cancelled/error state
	NumActiveUnits   int64  `json:"num_active_units"   sql:"num_active_units"   sqlitetype:"integer"` // Number of active units that are in running state
	NumProjects      int64  `json:"num_projects"       sql:"num_projects"       sqlitetype:"integer"` // Number of projects
	NumUsers         int64  `json:"num_users"          sql:"num_users"          sqlitetype:"integer"` // Number of users
}

// TagNames returns a slice of all tag names.
func (s Stat) TagNames(tag string) []string {
	return structset.StructFieldTagValues(s, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (s Stat) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(s, keyTag, valueTag)
}

// Project is the container for a given account/tenant/namespace of cluster.
type Project struct {
	ID              int64  `json:"-"                sql:"id"               sqlitetype:"integer not null primary key"`
	UID             string `json:"uid,omitempty"    sql:"uid"              sqlitetype:"text"`                                                                      // Unique identifier of the project provided by cluster
	ClusterID       string `json:"cluster_id"       sql:"cluster_id"       sqlitetype:"text"`                                                                      // Identifier of the resource manager that owns project. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager string `json:"resource_manager" sql:"resource_manager" sqlitetype:"text"`                                                                      // Name of the resource manager that owns project. Eg slurm, openstack, kubernetes, etc
	Name            string `json:"name"             sql:"name"             sqlitetype:"text"`                                                                      // Name of the project
	Users           List   `example:"usr1,usr2"     json:"users"           sql:"users"                               sqlitetype:"text" swaggertype:"array,string"` // List of users of the project
	Tags            List   `example:"tag1,tag2"     json:"tags,omitempty"  sql:"tags"                                sqlitetype:"text" swaggertype:"array,string"` // List of meta data tags of the project
	LastUpdatedAt   string `json:"-"                sql:"last_updated_at"  sqlitetype:"text"`                                                                      // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (Project) TableName() string {
	return projectsTableName
}

// TagNames returns a slice of all tag names.
func (p Project) TagNames(tag string) []string {
	return structset.StructFieldTagValues(p, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (p Project) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(p, keyTag, valueTag)
}

// User is the container for a given user of cluster.
type User struct {
	ID              int64  `json:"-"                sql:"id"               sqlitetype:"integer not null primary key"`
	UID             string `json:"uid,omitempty"    sql:"uid"              sqlitetype:"text"`                                                                      // Unique identifier of the user provided by cluster
	ClusterID       string `json:"cluster_id"       sql:"cluster_id"       sqlitetype:"text"`                                                                      // Identifier of the resource manager that owns user. It is used to differentiate multiple clusters of same resource manager.
	ResourceManager string `json:"resource_manager" sql:"resource_manager" sqlitetype:"text"`                                                                      // Name of the resource manager that owns user. Eg slurm, openstack, kubernetes, etc
	Name            string `json:"name"             sql:"name"             sqlitetype:"text"`                                                                      // Name of the user
	Projects        List   `example:"prj1,prj2"     json:"projects"        sql:"projects"                            sqlitetype:"text" swaggertype:"array,string"` // List of projects of the user
	Tags            List   `example:"tag1,tag2"     json:"tags,omitempty"  sql:"tags"                                sqlitetype:"text" swaggertype:"array,string"` // List of meta data tags of the user
	LastUpdatedAt   string `json:"-"                sql:"last_updated_at"  sqlitetype:"text"`                                                                      // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (User) TableName() string {
	return usersTableName
}

// TagNames returns a slice of all tag names.
func (u User) TagNames(tag string) []string {
	return structset.StructFieldTagValues(u, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (u User) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(u, keyTag, valueTag)
}

// AdminUsers from different sources.
type AdminUsers struct {
	ID            int64  `json:"-"            sql:"id"              sqlitetype:"integer not null primary key"`
	Source        string `json:"source"       sql:"source"          sqlitetype:"text"`                                                                      // Source of admin users
	Users         List   `example:"adm1,adm2" json:"users"          sql:"users"                               sqlitetype:"text" swaggertype:"array,string"` // List of users. In DB, users will be stored with | delimiter
	LastUpdatedAt string `json:"-"            sql:"last_updated_at" sqlitetype:"text"`                                                                      // Last Updated time
}

// TableName returns the table which admin users list is stored into.
func (AdminUsers) TableName() string {
	return adminUsersTableName
}

// TagNames returns a slice of all tag names.
func (a AdminUsers) TagNames(tag string) []string {
	return structset.StructFieldTagValues(a, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (a AdminUsers) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(a, keyTag, valueTag)
}

// Key represents arbritrary keys used in metric maps.
type Key struct {
	Name string `json:"name" sql:"name" sqlitetype:"text"` // Name of the metric key
}

// TagNames returns a slice of all tag names.
func (k Key) TagNames(tag string) []string {
	return structset.StructFieldTagValues(k, tag)
}

// TagMap returns a map of tags based on keyTag and valueTag. If keyTag is empty,
// field names are used as map keys.
func (k Key) TagMap(keyTag string, valueTag string) map[string]string {
	return structset.StructFieldTagMap(k, keyTag, valueTag)
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
