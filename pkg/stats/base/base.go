package base

import (
	"fmt"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/structset"
	"github.com/mahendrapaipuri/ceems/pkg/stats/types"
)

// Name of ceems server kingpin app
const CEEMSServerAppName = "ceems_server"

// `ceems_server` CLI app
var CEEMSServerApp = *kingpin.New(
	CEEMSServerAppName,
	"API server for reporting usage statistics for batchjobs/VMs/Pods.",
)

// API Resources names
var (
	UnitsResourceName    = "units"
	UsageResourceName    = "usage"
	ProjectsResourceName = "projects"
)

// Endpoints
var (
	UnitsEndpoint    = UnitsResourceName
	UsageEndpoint    = UsageResourceName
	ProjectsEndpoint = ProjectsResourceName
)

// DB table names
var (
	UnitsDBTableName = UnitsResourceName
	UsageDBTableName = UsageResourceName
)

// Slice of all field names of Unit struct
var (
	UnitsDBTableColNames = structset.GetStructFieldTagValues(types.Unit{}, "sql")
	UsageDBTableColNames = structset.GetStructFieldTagValues(types.Usage{}, "sql")
)

// Map of field names to DB column type
var (
	UnitsDBTableColTypeMap = structset.GetStructFieldTagMap(types.Unit{}, "sql", "sqlitetype")
	UsageDBTableColTypeMap = structset.GetStructFieldTagMap(types.Usage{}, "sql", "sqlitetype")
)

// Layout of datetime to be used in the package
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)

// CLI args with global scope
var (
	TSDBWebURL           string
	TSDBWebSkipTLSVerify bool

	GrafanaWebURL           string
	GrafanaWebSkipTLSVerify bool
	GrafanaAdminTeamID      string
)
