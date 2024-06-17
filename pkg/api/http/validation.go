package http

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	ceems_db "github.com/mahendrapaipuri/ceems/pkg/api/db"
)

// adminUsers returns a slice of admin users fetched from DB
func adminUsers(dbConn *sql.DB, logger log.Logger) []string {
	var users []string
	for _, source := range ceems_db.AdminUsersSources {
		rows, err := dbConn.Query(
			fmt.Sprintf("SELECT users FROM %s WHERE source = ?", base.AdminUsersDBTableName),
			source,
		)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to query for admin users", "source", source, "err", err)
			continue
		}

		// Scan users rows
		var usersList string
		for rows.Next() {
			if err := rows.Scan(&usersList); err != nil {
				level.Error(logger).Log("msg", "Failed to scan row for admin users query", "source", source, "err", err)
				continue
			}
			users = append(users, strings.Split(usersList, ceems_db.AdminUsersSeparator)...)
		}
	}
	return users
}

// VerifyOwnership returns true if user is the owner of queried units
func VerifyOwnership(user string, rmIDs []string, uuids []string, db *sql.DB, logger log.Logger) bool {
	// If there are no UUIDs pass
	// If current user is in list of admin users, pass the check
	if len(uuids) == 0 || slices.Contains(adminUsers(db, logger), user) {
		return true
	}

	// If the data is incomplete, forbid the request
	if db == nil || len(rmIDs) == 0 || user == "" {
		level.Debug(logger).Log(
			"msg", "Incomplete data for unit ownership verification", "user", user,
			"cluster_id", strings.Join(rmIDs, ","), "queried_uuids", strings.Join(uuids, ","),
		)
		return false
	}

	level.Debug(logger).
		Log("msg", "UUIDs in query", "user", user,
			"cluster_id", strings.Join(rmIDs, ","), "queried_uuids", strings.Join(uuids, ","),
		)

	// First get a list of projects that user is part of
	queryData := islice(append([]string{user}, rmIDs...))
	rows, err := db.Query(
		fmt.Sprintf(
			"SELECT DISTINCT project FROM %s WHERE usr = ? AND cluster_id IN (%s)",
			base.UsageDBTableName, strings.Join(strings.Split(strings.Repeat("?", len(rmIDs)), ""), ","),
		),
		queryData...,
	)
	if err != nil {
		level.Warn(logger).
			Log("msg", "Failed to get user projects. Query unauthorized", "user", user,
				"cluster_id", strings.Join(rmIDs, ","), "queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return false
	}

	// Scan project rows
	var projects []string
	var project string
	for rows.Next() {
		if err := rows.Scan(&project); err != nil {
			continue
		}
		projects = append(projects, project)
	}

	// If no projects found, return. This should not be the case and if it happens
	// something is wrong. Spit a warning log
	if len(projects) == 0 {
		level.Warn(logger).
			Log("msg", "No user projects found. Query unauthorized", "user", user,
				"cluster_id", strings.Join(rmIDs, ","), "queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return false
	}

	// Make a query and query args. Query args must be converted to slice of interfaces
	// and it is sql driver's responsibility to cast them to proper types
	query := fmt.Sprintf(
		"SELECT uuid FROM %s WHERE cluster_id IN (%s) AND project IN (%s) AND uuid IN (%s)",
		base.UnitsDBTableName,
		strings.Join(strings.Split(strings.Repeat("?", len(rmIDs)), ""), ","),
		strings.Join(strings.Split(strings.Repeat("?", len(projects)), ""), ","),
		strings.Join(strings.Split(strings.Repeat("?", len(uuids)), ""), ","),
	)
	qData := rmIDs
	qData = append(qData, projects...)
	qData = append(qData, uuids...)
	queryData = islice(qData)

	// Make query. If query fails for any reason, we block the request
	rows, err = db.Query(query, queryData...)
	if err != nil {
		level.Warn(logger).
			Log("msg", "Failed to check uuid ownership. Query unauthorized", "user", user, "query", query,
				"user_projects", strings.Join(projects, ","), "queried_uuids", strings.Join(uuids, ","),
				"cluster_id", strings.Join(rmIDs, ","), "err", err,
			)
		return false
	}
	defer rows.Close()

	// Get number of rows returned by query
	uuidCount := 0
	for rows.Next() {
		uuidCount++
	}

	// If returned number of UUIDs is not same as queried UUIDs, user is attempting
	// to query for jobs of other user
	if uuidCount != len(uuids) {
		level.Debug(logger).
			Log("msg", "Unauthorized query", "user", user, "user_projects", strings.Join(projects, ","),
				"queried_uuids", len(uuids), "found_uuids", uuidCount,
			)
		return false
	}
	return true
}

// Convert a slice of types into slice of interfaces
func islice(x interface{}) []interface{} {
	xv := reflect.ValueOf(x)
	out := make([]interface{}, xv.Len())
	for i := range out {
		out[i] = xv.Index(i).Interface()
	}
	return out
}
