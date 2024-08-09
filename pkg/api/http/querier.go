package http

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/structset"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

var queryRegexp = regexp.MustCompile("SELECT (.*?) FROM (.*)")

// Query builder struct.
type Query struct {
	builder strings.Builder
	params  []string
}

// Add query to builder.
func (q *Query) query(s string) {
	q.builder.WriteString(s)
}

// Add parameter and its placeholder.
func (q *Query) param(val []string) {
	q.builder.WriteString(fmt.Sprintf("(%s)", strings.Join(strings.Split(strings.Repeat("?", len(val)), ""), ",")))
	q.params = append(q.params, val...)
}

// Add sub query to builder.
func (q *Query) subQuery(sq Query) {
	subQuery, subQueryParams := sq.get()
	q.builder.WriteString(fmt.Sprintf("(%s)", subQuery))
	q.params = append(q.params, subQueryParams...)
}

// Get current query string and its parameters.
func (q *Query) get() (string, []string) {
	return q.builder.String(), q.params
}

// projectsSubQuery returns a sub query that returns projects of users
// With my limited SQL skills the best query I came up with is following:
// SELECT * FROM usage WHERE project IN (SELECT name FROM projects WHERE EXISTS (SELECT 1 FROM json_each(users) WHERE value = 'usr1'))
// Not sure if it is the most optimal but will do for the time being.
func projectsSubQuery(users []string) Query {
	// Make a sub query that will fetch projects of users
	// SELECT name FROM projects WHERE EXISTS (SELECT 1 FROM json_each(users) WHERE value = 'usr1')
	innerQuery := Query{}
	innerQuery.query("SELECT 1 FROM json_each(users)")

	// Add conditions to sub query
	if len(users) > 0 {
		innerQuery.query(" WHERE value IN ")
		innerQuery.param(users)
	}

	// Sub query with inner query
	qSub := Query{}
	qSub.query("SELECT name FROM " + base.ProjectsDBTableName)
	qSub.query(" WHERE EXISTS ")
	qSub.subQuery(innerQuery)

	return qSub
}

// Scan rows
// We use numRows only for units query as returned number of units can be very big
// and preallocating can have positive impact on performance
// Ref: https://oilbeater.com/en/2024/03/04/golang-slice-performance/
// For the rest of queries, they should return fewer rows and hence, we can live with
// dynamic allocation.
func scanRows[T any](rows *sql.Rows, numRows int) ([]T, error) {
	var columns []string

	values := make([]T, numRows)

	var value T

	var err error

	scanErrs := 0
	rowIdx := 0

	// Get indexes
	indexes := structset.CachedFieldIndexes(reflect.TypeOf(&value).Elem())

	// Get columns
	if columns, err = rows.Columns(); err != nil {
		return nil, fmt.Errorf("cannot fetch columns: %w", err)
	}

	// Scan each row
	for rows.Next() {
		if err := structset.ScanRow(rows, columns, indexes, &value); err != nil {
			scanErrs++
		}

		if numRows > 0 {
			values[rowIdx] = value
		} else {
			values = append(values, value) //nolint:makezero
		}

		rowIdx++
	}

	// If we failed to scan any rows, return error which will be included in warnings
	// in the response
	if scanErrs > 0 {
		err = fmt.Errorf("failed to scan %d rows", scanErrs)
	}

	return values, err
}

func countRows(ctx context.Context, dbConn *sql.DB, query Query) (int, error) {
	var numRows int

	// Get query string and params
	queryString, queryParams := query.get()

	// Prepare SQL statements
	countQuery := queryRegexp.ReplaceAllString(queryString, "SELECT COUNT(*) FROM $2")

	countStmt, err := dbConn.Prepare(countQuery)
	if err != nil {
		return 0, err
	}

	defer countStmt.Close()

	// queryParams has to be an inteface. Do casting here
	qParams := make([]interface{}, len(queryParams))
	for i, v := range queryParams {
		qParams[i] = v
	}

	// First make a query to get number of rows that will be returned by query
	countRows, err := countStmt.QueryContext(ctx, qParams...)
	if err != nil || countRows.Err() != nil {
		return 0, err
	}
	defer countRows.Close()

	// Iterate through rows. For GROUP BY queries we get multiple rows with each row
	// containing aggregate count.
	// For usage model we use number of rows returned by query as numRows where as
	// for units model we return number returned by first row
	//
	// Not the best solution but can work for now
	irow := 0
	for countRows.Next() {
		irow++

		if err := countRows.Scan(&numRows); err != nil {
			continue
		}
	}

	return numRows, nil
}

// Querier queries the DB and return the response.
func Querier[T any](ctx context.Context, dbConn *sql.DB, query Query, logger log.Logger) ([]T, error) {
	var numRows int

	var err error

	// If requested model is units, get number of rows
	switch any(*new(T)).(type) {
	case models.Unit:
		if numRows, err = countRows(ctx, dbConn, query); err != nil {
			level.Error(logger).Log("msg", "Failed to get rows count", "err", err)

			return nil, err
		}
	default:
		numRows = 0
	}

	// Get query string and params
	queryString, queryParams := query.get()

	queryStmt, err := dbConn.Prepare(queryString)
	if err != nil {
		level.Error(logger).Log("msg", "Failed prepare query statement",
			"query", queryString, "queryParams", strings.Join(queryParams, ","), "err", err,
		)

		return nil, err
	}
	defer queryStmt.Close()

	// queryParams has to be an inteface. Do casting here
	qParams := make([]interface{}, len(queryParams))
	for i, v := range queryParams {
		qParams[i] = v
	}

	rows, err := queryStmt.QueryContext(ctx, qParams...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to get rows",
			"query", queryString, "queryParams", strings.Join(queryParams, ","), "err", err,
		)

		return nil, err
	}
	defer rows.Close()

	// Loop through rows, using Scan to assign column data to struct fields.
	level.Debug(logger).Log(
		"msg", "Rows", "query", queryString, "queryParams", strings.Join(queryParams, ","),
		"num_rows", numRows, "error", err,
	)

	return scanRows[T](rows, numRows)
}
