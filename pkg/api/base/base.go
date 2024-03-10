// Package base defines the names and variables that have global scope
// throughout which can be used in other subpackages
package base

import (
	"fmt"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/structset"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// CEEMSServerAppName is kingpin app name
const CEEMSServerAppName = "ceems_api_server"

// CEEMSServerApp is kinpin app
var CEEMSServerApp = *kingpin.New(
	CEEMSServerAppName,
	"API server for reporting usage statistics for batchjobs/VMs/Pods.",
)

// DB table names
var (
	UnitsDBTableName = models.Unit{}.TableName()
	UsageDBTableName = models.Usage{}.TableName()
)

// Slice of all field names of Unit struct
var (
	UnitsDBTableColNames = models.Unit{}.TagNames("sql")
	UsageDBTableColNames = models.Usage{}.TagNames("sql")
)

// Map of field names to DB column type
var (
	UnitsDBTableColTypeMap = structset.GetStructFieldTagMap(models.Unit{}, "sql", "sqlitetype")
	UsageDBTableColTypeMap = structset.GetStructFieldTagMap(models.Usage{}, "sql", "sqlitetype")
)

// DatetimeLayout to be used in the package
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)

// CLI args with global scope
var (
	GrafanaWebURL           string
	GrafanaWebSkipTLSVerify bool
	GrafanaAdminTeamID      string
)
