//go:build cgo
// +build cgo

package sqlite3

import (
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriver(t *testing.T) {
	db, err := sql.Open(DriverName, filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)

	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, NumConns())

	// Get the underlying sqlite3 connection
	sqlc, ok := GetLastConn()
	require.True(t, ok, "connection was not in connection map")
	require.IsType(t, &Conn{}, sqlc, "connection of wrong type returned")

	err = conn.Close()
	require.NoError(t, err, "could not close connection")

	err = db.Close()
	require.NoError(t, err, "could not close database")
	require.Equal(t, 0, NumConns())
}

func TestOpenMany(t *testing.T) {
	tmpdir := t.TempDir()
	expectedConnections := 12
	closers := make([]io.Closer, expectedConnections)
	conns := make([]*Conn, expectedConnections)

	for i := range expectedConnections {
		db, err := sql.Open(DriverName, filepath.Join(tmpdir, fmt.Sprintf("test-%d.db", i+1)))
		require.NoError(t, err, "could not open connection to database")
		require.NoError(t, db.Ping(), "could not ping database to establish a connection")
		closers[i] = db

		var ok bool
		conns[i], ok = GetLastConn()
		require.True(t, ok, "expected new connection")
	}

	// Ensure that we created the expected number of connections
	require.Equal(t, expectedConnections, NumConns())
	require.Len(t, closers, expectedConnections)
	require.Len(t, conns, expectedConnections)

	// Should have different connnections
	for i := 1; i < len(conns); i++ {
		require.NotSame(t, conns[i-1], conns[i], "expected connections to be different")
	}

	// Close each connection
	for _, closer := range closers {
		require.NoError(t, closer.Close(), "expected no error during close")

		expectedConnections--
		require.Equal(t, expectedConnections, NumConns())
	}
}

func TestAddMetricMap(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		new      string
		expected string
	}{
		{
			name:     "when existing and new have same signature",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2,"c":4,"b":1}`,
			expected: `{"a":3,"b":3,"c":7}`,
		},
		{
			name:     "when new has new keys",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2,"c":4,"b":1,"d":9}`,
			expected: `{"a":3,"b":3,"c":7,"d":9}`,
		},
		{
			name:     "when new has fewer keys",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2}`,
			expected: `{"a":3,"b":2,"c":3}`,
		},
		{
			name:     "when new has signature change",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2,"c":"+infinity","b":1,"d":9}`,
			expected: `{"a":3,"b":3,"c":3,"d":9}`,
		},
		{
			name:     "when new has invalid types",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2,"c":3,"b":1}`,
			expected: `{"a":3,"b":3,"c":6}`,
		},
		{
			name:     "when new has inf/nan types",
			existing: `{"a":1,"b":2,"c":3}`,
			new:      `{"a":2,"c":"inf","b":"nan"}`,
			expected: `{"a":3,"b":2,"c":3}`,
		},
		{
			name:     "when existing has inf/nan types",
			existing: `{"a":1,"b":"inf","c":3}`,
			new:      `{"a":2,"c":2,"b":"NaN"}`,
			expected: `{"a":3,"b":0,"c":5}`,
		},
	}

	for _, test := range tests {
		got := addMetricMap(test.existing, test.new)
		assert.Equal(t, test.expected, got)
	}
}

func TestAvgMetricMap(t *testing.T) {
	tests := []struct {
		name           string
		existing       string
		new            string
		existingWeight float64
		newWeight      float64
		expected       string
	}{
		{
			name:           "when existing and new have same signature",
			existing:       `{"a":1,"b":4,"c":8}`,
			new:            `{"a":4,"c":2,"b":1}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":2,"c":4}`,
		},
		{
			name:           "when new has new keys",
			existing:       `{"a":1,"b":4,"c":8}`,
			new:            `{"a":4,"c":2,"b":1,"d":9}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":2,"c":4,"d":9}`,
		},
		{
			name:           "when new has fewer keys",
			existing:       `{"a":1,"b":3,"c":8}`,
			new:            `{"a":4,"c":2}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":3,"c":4}`,
		},
		{
			name:           "when new has invalid types",
			existing:       `{"a":1,"b":4,"c":8}`,
			new:            `{"a":4,"c":2,"b":1}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":2,"c":4}`,
		},
		{
			name:           "when new has inf/nan types",
			existing:       `{"a":1,"b":3,"c":9}`,
			new:            `{"a":4,"c":"inf","b":"nan"}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":1,"c":3}`,
		},
		{
			name:           "when existing has inf/nan types",
			existing:       `{"a":1,"b":"inf","c":null}`,
			new:            `{"a":4,"c":3,"b":"-infinity"}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":3,"b":0,"c":2}`,
		},
		{
			name:           "when existing is empty",
			existing:       `{}`,
			new:            `{"a":4,"c":3,"b":1}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":4,"b":1,"c":3}`,
		},
		{
			name:           "when new is empty",
			existing:       `{"a":4,"c":3,"b":1}`,
			new:            `{}`,
			existingWeight: 1,
			newWeight:      2,
			expected:       `{"a":4,"b":1,"c":3}`,
		},
		{
			name:           "when existing weight is 0",
			existing:       `{"a":4,"c":3,"b":1}`,
			new:            `{"a":4,"c":2,"b":1}`,
			existingWeight: 0,
			newWeight:      2,
			expected:       `{"a":4,"b":1,"c":2}`,
		},
		{
			name:           "when new weight is 0",
			existing:       `{"a":4,"c":3,"b":1}`,
			new:            `{"a":4,"c":2,"b":1}`,
			existingWeight: 1,
			newWeight:      0,
			expected:       `{"a":4,"b":1,"c":3}`,
		},
	}

	for _, test := range tests {
		got := avgMetricMap(test.existing, test.new, test.existingWeight, test.newWeight)
		assert.Equal(t, test.expected, got, test.name)
	}
}

func TestSumMetricMap(t *testing.T) {
	testSlice := []string{
		`{"a":null,"b":2,"c":3,"d":"-infinity"}`, `{"a":2,"c":4,"b":1,"d":9,"e":"+inf"}`,
		`{"a":2,"c":"infinity","b":1,"d":9,"e":"+Infinity"}`, `{"a":2,"c":"nan","b":1,"d":9,"e":null}`,
	}
	expectedMap := `{"a":6,"b":5,"c":7,"d":27,"e":0}`

	gMap := newSumMetricMap()
	for _, m := range testSlice {
		gMap.Step(m)
	}

	// Finally do the aggregation
	aggMap := gMap.Done()
	assert.Equal(t, expectedMap, aggMap)
}

func TestAvgMetricMapAgg(t *testing.T) {
	testSlice := []string{
		`{"a":"+Inf","b":1,"c":6,"d":"NaN","e":3}`, `{"a":2,"c":4,"b":1,"d":9,"e":"-Inf"}`,
		`{"a":3,"c":"-inf","b":1,"d":9,"e":"nan"}`, `{"a":2,"c":2,"b":1,"d":6,"e":1}`,
	}
	weights := []float64{10, 0, 20, 1}
	expectedMap := `{"a":2,"b":1,"c":2,"d":6,"e":1}`

	gMap := newAvgMetricMapAgg()
	for im, m := range testSlice {
		gMap.Step(m, weights[im])
	}

	// Finally do the aggregation
	aggMap := gMap.Done()
	assert.Equal(t, expectedMap, aggMap)
}

func setupDB(tmpDir string, aggMetric bool, units []models.Unit) (models.MetricMap, models.MetricMap, error) {
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open(DriverName, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create DB: %w", err)
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE units (
	"id" integer not null primary key,
	"uuid" text,
	"total_time_seconds" text,
	"avg_cpu_usage" text
);
CREATE UNIQUE INDEX uq_cluster_id_uuid_start ON units (uuid);`

	_, err = db.Exec(stmts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create table in DB: %w", err)
	}

	updateStmt := `
INSERT INTO units (uuid,total_time_seconds,avg_cpu_usage) VALUES(:uuid,:total_time_seconds,:avg_cpu_usage) ON CONFLICT(uuid) DO UPDATE SET
  total_time_seconds = add_metric_map(total_time_seconds, :total_time_seconds),
  avg_cpu_usage = avg_metric_map(avg_cpu_usage, :avg_cpu_usage, CAST(json_extract(total_time_seconds, '$.alloc_cputime') AS REAL), CAST(json_extract(:total_time_seconds, '$.alloc_cputime') AS REAL))`

	sqlStmt, err := db.Prepare(updateStmt)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare statement for table %w", err)
	}
	defer sqlStmt.Close()

	for _, unit := range units {
		if _, err := sqlStmt.Exec(
			sql.Named("uuid", unit.UUID),
			sql.Named("total_time_seconds", unit.TotalTime),
			sql.Named("avg_cpu_usage", unit.AveCPUUsage),
		); err != nil {
			return nil, nil, fmt.Errorf("failed to insert data for table %w", err)
		}
	}

	// Make units query
	var cpuUsage, totalTimes models.MetricMap
	if aggMetric {
		_ = db.QueryRow("SELECT avg_metric_map_agg(avg_cpu_usage, CAST(json_extract(total_time_seconds, '$.alloc_cputime') AS REAL)) AS avg_cpu_usage, sum_metric_map_agg(total_time_seconds) AS total_time_seconds FROM units").
			Scan(&cpuUsage, &totalTimes)
	} else {
		_ = db.QueryRow("SELECT avg_cpu_usage, total_time_seconds FROM units").Scan(&cpuUsage, &totalTimes)
	}

	return cpuUsage, totalTimes, nil
}

func TestCustomFuncsInDB(t *testing.T) {
	tests := []struct {
		name               string
		aggMetric          bool
		units              []models.Unit
		expectedCPUUsage   models.MetricMap
		expectedTotalTimes models.MetricMap
	}{
		{
			name: "first update",
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 10},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 1000},
		},
		{
			name: "second update",
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{"global": 25},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 15},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 1500},
		},
		{
			name: "second update with null update value",
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 10},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 1500},
		},
		{
			name: "second update with zero update value",
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 0},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 5},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 2000},
		},
		{
			name: "second update with zero weight value",
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 0},
					AveCPUUsage: models.MetricMap{"global": 25},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 10},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 1000},
		},
		{
			name:      "aggregate query",
			aggMetric: true,
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "2",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{"global": 25},
				},
				{
					UUID:        "3",
					TotalTime:   models.MetricMap{"alloc_cputime": 1500},
					AveCPUUsage: models.MetricMap{"global": 5},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 10},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 3000},
		},
		{
			name:      "aggregate query with null metric",
			aggMetric: true,
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 1000},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "2",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{},
				},
				{
					UUID:        "3",
					TotalTime:   models.MetricMap{"alloc_cputime": 1500},
					AveCPUUsage: models.MetricMap{"global": 5},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 7},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 3000},
		},
		{
			name:      "aggregate query with zero value",
			aggMetric: true,
			units: []models.Unit{
				{
					UUID:        "1",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{"global": 10},
				},
				{
					UUID:        "2",
					TotalTime:   models.MetricMap{"alloc_cputime": 500},
					AveCPUUsage: models.MetricMap{"global": 0},
				},
				{
					UUID:        "3",
					TotalTime:   models.MetricMap{"alloc_cputime": 2000},
					AveCPUUsage: models.MetricMap{"global": 20},
				},
			},
			expectedCPUUsage:   models.MetricMap{"global": 15},
			expectedTotalTimes: models.MetricMap{"alloc_cputime": 3000},
		},
	}

	for _, test := range tests {
		gotCPUUsage, gotTotalTimes, err := setupDB(t.TempDir(), test.aggMetric, test.units)
		require.NoError(t, err)
		assert.Equal(t, test.expectedCPUUsage, gotCPUUsage, test.name)
		assert.Equal(t, test.expectedTotalTimes, gotTotalTimes, test.name)
	}
}
