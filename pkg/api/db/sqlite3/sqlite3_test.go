package sqlite3

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDriver(t *testing.T) {
	db, err := sql.Open(DriverName, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Errorf("failed to open DB: %s", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Errorf("could not create connection with custom driver: %s", err)
	}
	if NumConns() != 1 {
		t.Errorf("expected 1 connection, got %d", NumConns())
	}

	// Get the underlying sqlite3 connection
	_, ok := GetLastConn()
	if !ok {
		t.Errorf("connection was not in connection map")
	}
	// require.IsType(t, &sqlite.Conn{}, sqlc, "connection of wrong type returned")

	err = conn.Close()
	if err != nil {
		t.Errorf("failed to close connection: %s", err)
	}

	err = db.Close()
	if err != nil {
		t.Errorf("failed to close DB: %s", err)
	}

	if NumConns() != 0 {
		t.Errorf("expected 0 connections, got %d", NumConns())
	}
}

func TestOpenMany(t *testing.T) {
	tmpdir := t.TempDir()
	expectedConnections := 12
	closers := make([]io.Closer, expectedConnections)
	conns := make([]*Conn, expectedConnections)

	for i := 0; i < expectedConnections; i++ {
		db, err := sql.Open(DriverName, filepath.Join(tmpdir, fmt.Sprintf("test-%d.db", i+1)))
		if err != nil {
			t.Errorf("could not open connection to database: %s", err)
		}
		if err := db.Ping(); err != nil {
			t.Errorf("could not ping database to establish a connection: %s", err)
		}
		closers[i] = db

		var ok bool
		conns[i], ok = GetLastConn()
		if !ok {
			t.Errorf("expected new connection")
		}
	}

	// Ensure that we created the expected number of connections
	if expectedConnections != NumConns() {
		t.Errorf("expected DB connecttions %d, got %d", expectedConnections, NumConns())
	}
	if expectedConnections != len(closers) {
		t.Errorf("expected closers %d, got %d", expectedConnections, len(closers))
	}
	if expectedConnections != len(conns) {
		t.Errorf("expected conns %d, got %d", expectedConnections, len(conns))
	}

	// Should have different connnections
	for i := 1; i < len(conns); i++ {
		if reflect.DeepEqual(conns[i-1], conns[i]) {
			t.Errorf("expected connections to be different")
		}
	}

	// Close each connection
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			t.Errorf("expected no error during close: %s", err)
		}
		expectedConnections--
		if expectedConnections != NumConns() {
			t.Errorf("expected connections %d, got %d", expectedConnections, NumConns())
		}
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
		if test.expected != got {
			t.Errorf("%s: expected %s, got %s", test.name, test.expected, got)
		}
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
			expected:       `{"a":3,"b":1,"c":4}`,
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
	}

	for _, test := range tests {
		got := avgMetricMap(test.existing, test.new, test.existingWeight, test.newWeight)
		if test.expected != got {
			t.Errorf("%s: expected %s, got %s", test.name, test.expected, got)
		}
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
	if aggMap != expectedMap {
		t.Errorf("expected sum %s, got %s", expectedMap, aggMap)
	}
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
	if aggMap != expectedMap {
		t.Errorf("expected avg %s, got %s", expectedMap, aggMap)
	}
}
