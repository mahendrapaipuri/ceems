package http

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// adminUsers returns a slice of admin users fetched from DB
func adminUsers(dbConn *sql.DB, logger log.Logger) []string {
	var users []string
	rows, err := dbConn.Query(
		fmt.Sprintf("SELECT users FROM %s", base.AdminUsersDBTableName),
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to query for admin users", "err", err)
		return nil
	}

	// Scan users rows
	var usersList models.List
	for rows.Next() {
		if err := rows.Scan(&usersList); err != nil {
			level.Error(logger).Log("msg", "Failed to scan row for admin users query", "err", err)
			continue
		}
		for _, user := range usersList {
			users = append(users, user.(string))
		}
	}
	return users
}

// VerifyOwnership returns true if user is the owner of queried units
func VerifyOwnership(
	ctx context.Context,
	user string,
	clusterIDs []string,
	uuids []string,
	db *sql.DB,
	logger log.Logger,
) bool {
	// If the data is incomplete, forbid the request
	if db == nil || len(clusterIDs) == 0 || user == "" {
		level.Debug(logger).Log(
			"msg", "Incomplete data for unit ownership verification", "user", user,
			"cluster_id", strings.Join(clusterIDs, ","), "queried_uuids", strings.Join(uuids, ","),
		)
		return false
	}

	// If current user is in list of admin users, pass the check
	if slices.Contains(adminUsers(db, logger), user) {
		return true
	}
	level.Debug(logger).
		Log("msg", "UUIDs in query", "user", user,
			"cluster_id", strings.Join(clusterIDs, ","), "queried_uuids", strings.Join(uuids, ","),
		)

	// Get sub query for projects
	qSub := projectsSubQuery([]string{user})

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT uuid,cluster_id FROM %s", base.UnitsDBTableName))

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
		level.Error(logger).
			Log("msg", "Failed to check uuid ownership. Query unauthorized", "user", user,
				"queried_uuids", strings.Join(uuids, ","),
				"cluster_id", strings.Join(clusterIDs, ","), "err", err,
			)
		return false
	}

	// If returned number of UUIDs is not same as queried UUIDs, user is attempting
	// to query for jobs of other user
	if len(units) != len(uuids) {
		level.Debug(logger).
			Log("msg", "Unauthorized query", "user", user,
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
