package http

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/models"
)

// Null logger for adminUsers function. We dont need to log
// admin users query for each request.
var (
	nullLogger = slog.New(slog.DiscardHandler)
)

// AdminUsers returns a slice of admin users fetched from DB.
// Errors must always be checked to ensure no row scanning has failed.
func AdminUsers(ctx context.Context, dbConn *sql.DB) ([]models.User, error) {
	// Initialise query
	q := Query{}
	q.query("SELECT cluster_id,name,tags FROM " + base.AdminUsersDBTableName)

	return Querier[models.User](ctx, dbConn, q, nullLogger)
}

// AdminUserNames returns a slice of admin users names.
func AdminUserNames(ctx context.Context, dbConn *sql.DB) ([]string, error) {
	var users []string

	// Fetch users
	admins, err := AdminUsers(ctx, dbConn)
	if err != nil {
		return nil, err
	}

	// Get all admin users
	for _, admin := range admins {
		users = append(users, admin.Name)
	}

	return users, nil
}

// VerifyOwnership returns true if user is the owner of queried units.
func VerifyOwnership(
	ctx context.Context,
	user string,
	clusterIDs []string,
	uuids []string,
	db *sql.DB,
	logger *slog.Logger,
) bool {
	// If no DB connection is provided, log warning and return true
	// This should not happen, just in case!
	if db == nil {
		logger.Warn("DB connection is empty. Skipping UUID verification")

		return true
	}

	// If the data is incomplete, forbid the request
	if len(clusterIDs) == 0 || user == "" || len(uuids) == 0 {
		logger.Debug(
			"Incomplete data for unit ownership verification", "user", user,
			"cluster_id", strings.Join(clusterIDs, ","), "queried_uuids", strings.Join(uuids, ","),
		)

		return false
	}

	logger.Debug("UUIDs in query", "user", user, "cluster_id", strings.Join(clusterIDs, ","), "queried_uuids", strings.Join(uuids, ","))

	// Get sub query for projects
	qSub := projectsSubQuery([]string{user})

	// Make query
	q := Query{}
	q.query("SELECT uuid,cluster_id FROM " + base.UnitsDBTableName)

	// Add project sub query
	q.query(" WHERE project IN ")
	q.subQuery(qSub)

	// Add cluster IDs conditional clause
	q.query(" AND cluster_id IN ")
	q.param(clusterIDs)

	// Add uuids in question
	q.query(" AND uuid IN ")
	q.param(uuids)

	// Run query and get response
	units, err := Querier[models.Unit](ctx, db, q, logger)
	if err != nil {
		logger.Error("Failed to check uuid ownership. Query unauthorized", "user", user,
			"queried_uuids", strings.Join(uuids, ","), "cluster_id", strings.Join(clusterIDs, ","),
			"err", err,
		)

		return false
	}

	// If returned number of UUIDs is less than requested UUIDs, it means user do not have
	// access to some of the queried UUIDs and we need to forbid the request
	// As SLURM JobIDs can overflow and start over, there can be more than 1 units with given
	// attributes that is why we check for less than condition.
	// To use the equality, we need to add start/end time to the query and that will start to
	// become complicated as for certain queries we cannot realiably get start/end times. So,
	// we "loosen" the query a little for better usability.
	// NOTE that this does not/should not have any impact on security which means user's will not
	// be able to access metrics of others.
	if len(units) < len(uuids) {
		logger.Debug("Unauthorized query", "user", user,
			"queried_uuids", len(uuids), "found_uuids", len(units),
		)

		return false
	}

	return true
}
