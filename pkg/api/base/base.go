// Package base defines the names and variables that have global scope
// throughout which can be used in other subpackages
package base

import (
	"fmt"
	"regexp"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/ceems-dev/ceems/pkg/api/models"
)

// CEEMSServerAppName is kingpin app name.
const CEEMSServerAppName = "ceems_api_server"

// CEEMSDBName is the name of CEEMS DB file.
const CEEMSDBName = "ceems.db"

// CEEMSServerApp is kinpin app.
var CEEMSServerApp = *kingpin.New(
	CEEMSServerAppName,
	"API server for reporting usage statistics for batchjobs/VMs/Pods.",
)

// DB table names.
var (
	UnitsDBTableName      = models.Unit{}.TableName()
	UsageDBTableName      = models.Usage{}.TableName()
	DailyUsageDBTableName = models.DailyUsage{}.TableName()
	ProjectsDBTableName   = models.Project{}.TableName()
	UsersDBTableName      = models.User{}.TableName()
	AdminUsersDBTableName = models.AdminUser{}.TableName()
)

// Slice of field names of all tables
// This slice will not contain the DB columns that are ignored in the query.
var (
	UnitsDBTableColNames      = models.Unit{}.TagNames("json")
	UsageDBTableColNames      = models.Usage{}.TagNames("json")
	ProjectsDBTableColNames   = models.Project{}.TagNames("json")
	UsersDBTableColNames      = models.User{}.TagNames("json")
	AdminUsersDBTableColNames = models.User{}.TagNames("json")
)

// Map of struct field name to DB column name.
var (
	UnitsDBTableStructFieldColNameMap      = models.Unit{}.TagMap("", "sql")
	UsageDBTableStructFieldColNameMap      = models.Usage{}.TagMap("", "sql")
	ProjectsDBTableStructFieldColNameMap   = models.Project{}.TagMap("", "sql")
	UsersDBTableStructFieldColNameMap      = models.User{}.TagMap("", "sql")
	AdminUsersDBTableStructFieldColNameMap = models.User{}.TagMap("", "sql")
)

// DatetimeLayout to be used in the package.
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)

// DatetimezoneLayout to be used in the package.
var DatetimezoneLayout = DatetimeLayout + "-0700"

// CLI args with global scope.
var (
	ConfigFilePath          string
	ConfigFileExpandEnvVars bool
)

// APIVersion sets the version of API in paths.
const APIVersion = "v1"

// Cluster and Updater ID valid regex.
var (
	InvalidIDRegex = regexp.MustCompile("[^a-zA-Z0-9-_]")
)

// CEEMSServiceAccount is the internal service account that has admin status.
const CEEMSServiceAccount = "ceems-int-svc"

// Headers.
const (
	GrafanaUserHeader       = "X-Grafana-User"
	OAuth2ProxyUserHeader   = "X-Auth-Request-User"
	AuthenticatedUserHeader = "X-Authenticated-User"
	LoggedUserHeader        = "X-Ceems-Logged-User"
	AdminUserHeader         = "X-Ceems-Admin-User"
	ClusterIDHeader         = "X-Ceems-Cluster-Id"
)

// Username to be used for unknown users.
const UnknownUser = "unknown"
