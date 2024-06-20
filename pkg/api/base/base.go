// Package base defines the names and variables that have global scope
// throughout which can be used in other subpackages
package base

import (
	"fmt"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// CEEMSServerAppName is kingpin app name
const CEEMSServerAppName = "ceems_api_server"

// CEEMSDBName is the name of CEEMS DB file
const CEEMSDBName = "ceems.db"

// CEEMSServerApp is kinpin app
var CEEMSServerApp = *kingpin.New(
	CEEMSServerAppName,
	"API server for reporting usage statistics for batchjobs/VMs/Pods.",
)

// DB table names
var (
	UnitsDBTableName      = models.Unit{}.TableName()
	UsageDBTableName      = models.Usage{}.TableName()
	ProjectsDBTableName   = models.Project{}.TableName()
	UsersDBTableName      = models.User{}.TableName()
	AdminUsersDBTableName = models.AdminUsers{}.TableName()
)

// Slice of all field names of Unit struct
var (
	UnitsDBTableColNames      = models.Unit{}.TagNames("sql")
	UsageDBTableColNames      = models.Usage{}.TagNames("sql")
	ProjectsDBTableColNames   = models.Project{}.TagNames("sql")
	UsersDBTableColNames      = models.User{}.TagNames("sql")
	AdminUsersDBTableColNames = models.AdminUsers{}.TagNames("sql")
)

// Map of struct field name to DB column name
var (
	UnitsDBTableStructFieldColNameMap      = models.Unit{}.TagMap("", "sql")
	UsageDBTableStructFieldColNameMap      = models.Usage{}.TagMap("", "sql")
	ProjectsDBTableStructFieldColNameMap   = models.Project{}.TagMap("", "sql")
	UsersDBTableStructFieldColNameMap      = models.User{}.TagMap("", "sql")
	AdminUsersDBTableStructFieldColNameMap = models.AdminUsers{}.TagMap("", "sql")
)

// Map of DB column names to DB column type
var (
	UnitsDBTableColTypeMap      = models.Unit{}.TagMap("sql", "sqlitetype")
	UsageDBTableColTypeMap      = models.Usage{}.TagMap("sql", "sqlitetype")
	ProjectsDBTableColTypeMap   = models.Project{}.TagMap("sql", "sqlitetype")
	UsersDBTableColTypeMap      = models.User{}.TagMap("sql", "sqlitetype")
	AdminUsersDBTableColTypeMap = models.AdminUsers{}.TagMap("sql", "sqlitetype")
)

// DatetimeLayout to be used in the package
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)

// CLI args with global scope
var (
	ConfigFilePath string
)

// APIVersion sets the version of API in paths
const APIVersion = "v1"
