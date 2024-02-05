package server

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/helper"
)

var (
	queryRegexp = regexp.MustCompile("SELECT (.*?) FROM (.*)")
)

// Query builder struct
type Query struct {
	builder strings.Builder
	params  []string
}

// Add query to builder
func (q *Query) query(s string) {
	q.builder.WriteString(s)
}

// Add parameter and its placeholder
func (q *Query) param(val []string) {
	q.builder.WriteString(fmt.Sprintf("(%s)", strings.Join(strings.Split(strings.Repeat("?", len(val)), ""), ",")))
	q.params = append(q.params, val...)
}

// Add sub query to builder
func (q *Query) subQuery(sq Query) {
	subQuery, subQueryParams := sq.get()
	q.builder.WriteString(fmt.Sprintf("(%s)", subQuery))
	q.params = append(q.params, subQueryParams...)
}

// Get current query string and its parameters
func (q *Query) get() (string, []string) {
	return q.builder.String(), q.params
}

// Scan rows to get usage statistics
// Ignore numRows as getting correct number of rows is bit fragile at the moment.
// We dont want panics due to insufficient allocation. We should look into improving
// this for future
func scanUsage(numRows int, rows *sql.Rows) interface{} {
	var usageRows []base.Usage
	var usage base.Usage
	for rows.Next() {
		if err := helper.ScanRow(rows, &usage); err != nil {
			continue
		}
		usageRows = append(usageRows, usage)
	}
	return usageRows
}

// Scan account rows
func scanAccounts(numRows int, rows *sql.Rows) interface{} {
	var accounts = make([]base.Account, numRows)
	var account base.Account
	rowIdx := 0
	for rows.Next() {
		if err := helper.ScanRow(rows, &account); err != nil {
			continue
		}
		accounts[rowIdx] = account
		rowIdx++
	}
	return accounts
}

// Scan job rows
func scanJobs(numRows int, rows *sql.Rows) interface{} {
	var jobs = make([]base.Job, numRows)
	var job base.Job
	rowIdx := 0
	for rows.Next() {
		if err := helper.ScanRow(rows, &job); err != nil {
			continue
		}
		jobs[rowIdx] = job
		rowIdx++
	}
	return jobs
}

// Get data from DB
func querier(dbConn *sql.DB, query Query, model string, logger log.Logger) (interface{}, error) {
	var numRows int

	// Get query string and params
	queryString, queryParams := query.get()

	// Prepare SQL statements
	countQuery := queryRegexp.ReplaceAllString(queryString, "SELECT COUNT(*) FROM $2")
	countStmt, err := dbConn.Prepare(countQuery)
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare count SQL statement for query", "query", countQuery,
			"queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer countStmt.Close()

	queryStmt, err := dbConn.Prepare(queryString)
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare query SQL statement for query", "query", queryString,
			"queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer queryStmt.Close()

	// queryParams has to be an inteface. Do casting here
	qParams := make([]interface{}, len(queryParams))
	for i, v := range queryParams {
		qParams[i] = v
	}

	// First make a query to get number of rows that will be returned by query
	countRows, err := countStmt.Query(qParams...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to query for number of rows",
			"query", countQuery, "queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer countRows.Close()

	// Iterate through rows. For GROUP BY queries we get multiple rows with each row
	// containing aggregate count.
	// For usage model we use number of rows returned by query as numRows where as
	// for jobs model we return number returned by first row
	//
	// Not the best solution but can work for now
	irow := 0
	for countRows.Next() {
		irow++
		if err := countRows.Scan(&numRows); err != nil {
			level.Error(logger).Log("msg", "Failed to scan count row",
				"query", countQuery, "queryParams", strings.Join(queryParams, ","), "err", err,
			)
			continue
		}
	}
	// It should be incremented by 1 as index starts from 0
	if model == base.UsageResourceName {
		numRows = irow + 1
	}

	rows, err := queryStmt.Query(qParams...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to get rows",
			"query", queryString, "queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer rows.Close()

	// Loop through rows, using Scan to assign column data to struct fields.
	if model == base.JobsResourceName {
		var jobs = scanJobs(numRows, rows)
		level.Debug(logger).Log(
			"msg", "Jobs found", "numRows", numRows, "query", queryString,
			"queryParams", strings.Join(queryParams, ","),
		)
		return jobs, nil
	} else if model == base.UsageResourceName {
		var usageStats = scanUsage(numRows, rows)
		level.Debug(logger).Log(
			"msg", "Usage stats found", "numRows", numRows, "query", queryString,
			"queryParams", strings.Join(queryParams, ","),
		)
		return usageStats, nil
	} else if model == "accounts" {
		var accounts = scanAccounts(numRows, rows)
		level.Debug(logger).Log(
			"msg", "Accounts found", "numRows", numRows, "query", queryString,
			"queryParams", strings.Join(queryParams, ","),
		)
		return accounts, nil
	}
	return nil, fmt.Errorf("unknown model: %s", model)
}
