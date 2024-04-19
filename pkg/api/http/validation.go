package http

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
)

// VerifyOwnership returns true if user is the owner of queried units
func VerifyOwnership(user string, uuids []string, db *sql.DB, logger log.Logger) bool {
	// If there is no active DB conn or if uuids is empty, return
	if db == nil || len(uuids) == 0 {
		return true
	}

	level.Debug(logger).
		Log("msg", "UUIDs in query", "user", user, "queried_uuids", strings.Join(uuids, ","))

	// First get a list of projects that user is part of
	rows, err := db.Query(
		fmt.Sprintf("SELECT DISTINCT project FROM %s WHERE usr = ?", base.UsageDBTableName),
		user,
	)
	if err != nil {
		level.Warn(logger).
			Log("msg", "Failed to get user projects. Allowing query", "user", user,
				"queried_uuids", strings.Join(uuids, ","), "err", err,
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
				"queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return false
	}

	// Make a query and query args. Query args must be converted to slice of interfaces
	// and it is sql driver's responsibility to cast them to proper types
	query := fmt.Sprintf(
		"SELECT uuid FROM %s WHERE project IN (%s) AND uuid IN (%s)",
		base.UnitsDBTableName,
		strings.Join(strings.Split(strings.Repeat("?", len(projects)), ""), ","),
		strings.Join(strings.Split(strings.Repeat("?", len(uuids)), ""), ","),
	)
	queryData := islice(append(projects, uuids...))

	// Make query. If query fails for any reason, we allow request to avoid false negatives
	rows, err = db.Query(query, queryData...)
	if err != nil {
		level.Warn(logger).
			Log("msg", "Failed to check uuid ownership. Query unauthorized", "user", user, "query", query,
				"user_projects", strings.Join(projects, ","), "queried_uuids", strings.Join(uuids, ","),
				"err", err,
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
