package http

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

const (
	startTimeTol = 3600000 // 1 hour in milliseconds
)

// Null logger for adminUsers function. We dont need to log
// admin users query for each request.
var (
	nullLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// AdminUsers returns a slice of admin users fetched from DB.
// Errors must always be checked to ensure no row scanning has failed.
func AdminUsers(ctx context.Context, dbConn *sql.DB) ([]models.AdminUsers, error) {
	// Initialise query
	q := Query{}
	q.query("SELECT * FROM " + base.AdminUsersDBTableName)

	return Querier[models.AdminUsers](ctx, dbConn, q, nullLogger)
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
		for _, user := range admin.Users {
			if userString, ok := user.(string); ok {
				users = append(users, userString)
			}
		}
	}

	return users, nil
}

// VerifyOwnership returns true if user is the owner of queried units.
func VerifyOwnership(
	ctx context.Context,
	user string,
	clusterIDs []string,
	uuids []string,
	starts []int64,
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

	// Get min and max of starts and use 1 hour as tolerance for boundaries
	if len(starts) > 0 {
		q.query(" AND started_at_ts BETWEEN ")
		q.param([]string{strconv.FormatInt(slices.Min(starts)-startTimeTol, 10)})
		q.query(" AND ")
		q.param([]string{strconv.FormatInt(slices.Max(starts)+startTimeTol, 10)})
	}

	// Run query and get response
	units, err := Querier[models.Unit](ctx, db, q, logger)
	if err != nil {
		logger.Error("Failed to check uuid ownership. Query unauthorized", "user", user,
			"queried_uuids", strings.Join(uuids, ","), "cluster_id", strings.Join(clusterIDs, ","),
			"err", err,
		)

		return false
	}

	// If returned number of UUIDs is not same as queried UUIDs, user is attempting
	// to query for jobs of other user
	if len(units) != len(uuids) {
		logger.Debug("Unauthorized query", "user", user,
			"queried_uuids", len(uuids), "found_uuids", len(units),
		)

		return false
	}

	// // In the case when verification is success, get ownership mode of each unit
	// var mode string
	// var ownershipModes []models.Ownership
	// for _, unit := range units {
	// 	if unit.Usr == user {
	// 		mode = "self"
	// 	} else {
	// 		mode = "project"
	// 	}
	// 	ownershipModes = append(ownershipModes, models.Ownership{UUID: unit.UUID, Mode: mode})
	// }
	return true
}
