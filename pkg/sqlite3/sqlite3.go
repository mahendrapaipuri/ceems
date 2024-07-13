/*
Package sqlite3 implements a connect hook around the sqlite3 driver so that the
underlying connection can be fetched from the driver for more advanced operations such
as backups.

Nicked from  github.com/rotationalio/ensign

The reason for repeating the code is that we will add custom functions to the
driver that will help in updating metrics that use Generic types.

AFAIK there is no way to register custom functions to the existing drivers.
*/
package sqlite3

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mattn/go-sqlite3"
)

// Init creates the connections map and registers the driver with the SQL package.
func init() {
	conns = make(map[uint64]*Conn)
	sql.Register(DriverName, &Driver{
		sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("add_metric_map", addMetricMap, true); err != nil {
					return err
				}
				if err := conn.RegisterFunc("avg_metric_map", avgMetricMap, true); err != nil {
					return err
				}
				if err := conn.RegisterAggregator("sum_metric_map_agg", newSumMetricMap, true); err != nil {
					return err
				}
				if err := conn.RegisterAggregator("avg_metric_map_agg", newAvgMetricMapAgg, true); err != nil {
					return err
				}
				return nil
			},
		},
	})
}

// In order to use this driver, specify the DriverName to sql.Open.
const (
	DriverName = "ceems_sqlite3"
)

var (
	seq   uint64
	mu    sync.Mutex
	conns map[uint64]*Conn
)

// Driver embeds a sqlite3 driver but overrides the Open function to ensure the
// connection created is a local connection with a sequence ID. It then maintains the
// connection locally until it is closed so that the underlying sqlite3 connection can
// be returned on demand.
type Driver struct {
	sqlite3.SQLiteDriver
}

// Open implements the sql.Driver interface and returns a sqlite3 connection that can
// be fetched by the user using GetLastConn. The connection ensures it's cleaned up
// when it's closed. This method is not used by the user, but rather by sql.Open.
func (d *Driver) Open(dsn string) (_ driver.Conn, err error) {
	var inner driver.Conn
	if inner, err = d.SQLiteDriver.Open(dsn); err != nil {
		return nil, err
	}

	var (
		ok    bool
		sconn *sqlite3.SQLiteConn
	)

	if sconn, ok = inner.(*sqlite3.SQLiteConn); !ok {
		return nil, fmt.Errorf("unknown connection type %T", inner)
	}

	mu.Lock()
	seq++
	conn := &Conn{cid: seq, SQLiteConn: sconn}
	conns[conn.cid] = conn
	mu.Unlock()

	return conn, nil
}

// Conn wraps a sqlite3.SQLiteConn and maintains an ID so that the connection can be
// closed.
type Conn struct {
	cid uint64
	*sqlite3.SQLiteConn
}

// Close the DB connection
// When the connection is closed, it is removed from the array of connections.
func (c *Conn) Close() error {
	mu.Lock()
	delete(conns, c.cid)
	mu.Unlock()
	return c.SQLiteConn.Close()
}

// Backup creates a backup of the DB using backup API
//
// The entire point of this package is to provide access to SQLite3 backup functionality
// on the sqlite3 connection. For more details on how to use the backup see the
// following links:
//
// https://www.sqlite.org/backup.html
// https://github.com/mattn/go-sqlite3/blob/master/_example/hook/hook.go
// https://github.com/mattn/go-sqlite3/blob/master/backup_test.go
//
// This is primarily used by the backups package and this method provides access
// directly to the underlying CGO call. This means the CGO call must be called correctly
// for example: the Finish() method MUST BE CALLED otherwise your code will panic.
func (c *Conn) Backup(dest string, srcConn *Conn, src string) (*sqlite3.SQLiteBackup, error) {
	return c.SQLiteConn.Backup(dest, srcConn.SQLiteConn, src)
}

// GetLastConn returns the last connection created by the driver. Unfortunately, there
// is no way to guarantee which connection will be returned since the sql.Open package
// does not provide any interface to the underlying connection object. The best a
// process can do is ping the server to open a new connection and then fetch the last
// connection immediately.
func GetLastConn() (*Conn, bool) {
	mu.Lock()
	defer mu.Unlock()
	conn, ok := conns[seq]
	return conn, ok
}

// NumConns returns the number of active connections. Only for testing purposes.
func NumConns() int {
	mu.Lock()
	defer mu.Unlock()
	return len(conns)
}

// addMetricMap adds the existing metricMap to newMetricMap
func addMetricMap(existing, new string) string {
	// Unmarshal strings into MetricMap type
	var existingMetricMap, newMetricMap models.MetricMap
	if err := json.Unmarshal([]byte(existing), &existingMetricMap); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(new), &newMetricMap); err != nil {
		panic(err)
	}

	// Make a deep copy of existingMetricMap into updatedMetricMap
	var updatedMetricMap = make(models.MetricMap)
	for metricName, metricValue := range existingMetricMap {
		updatedMetricMap[metricName] = metricValue
	}

	// Walk through new map and update existing with new.
	for metricName, newMetricValue := range newMetricMap {
		if existingMetricValue, ok := existingMetricMap[metricName]; ok {
			updatedMetricMap[metricName] = newMetricValue + existingMetricValue
		} else {
			updatedMetricMap[metricName] = newMetricValue
		}
	}

	// Finally, marshal the type into string and return
	updatedMetricMapBytes, err := json.Marshal(updatedMetricMap)
	if err != nil {
		panic(err)
	}
	return string(updatedMetricMapBytes)
}

// avgMetricMap makes average between a existing metricMap and a newMetricMap using
// weights
func avgMetricMap(existing, new string, existingWeight, newWeight float64) string {
	// Unmarshal strings into MetricMap type
	var existingMetricMap, newMetricMap models.MetricMap
	if err := json.Unmarshal([]byte(existing), &existingMetricMap); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(new), &newMetricMap); err != nil {
		panic(err)
	}

	// Make slice of maps and weights
	var metricMaps = []models.MetricMap{existingMetricMap, newMetricMap}
	var weights = []float64{existingWeight, newWeight}

	// Initialize vars
	var avgMetricMap = make(models.MetricMap)
	var totalWeights = make(map[string]models.JSONFloat)
	for imetricMap, metricMap := range metricMaps {
		for metricName, metricValue := range metricMap {
			weight := models.JSONFloat(weights[imetricMap])
			avgMetricMap[metricName] += metricValue * weight
			totalWeights[metricName] += weight
		}
	}

	// Divide weighted sum by counts to get weighted average
	for metricName := range avgMetricMap {
		if totalWeight, ok := totalWeights[metricName]; ok {
			if totalWeight > 0 {
				avgMetricMap[metricName] = avgMetricMap[metricName] / totalWeight
			}
		}
	}

	// Finally, marshal the type into string and return
	updatedMetricMapBytes, err := json.Marshal(avgMetricMap)
	if err != nil {
		panic(err)
	}
	return string(updatedMetricMapBytes)
}

// sumMetricMap aggregate sums MetricMaps.
// For int or float types, they will be summed up
// String types will be ignored and treated as zero
type sumMetricMap struct {
	metricMaps   []models.MetricMap
	aggMetricMap models.MetricMap
	n            int64
}

// newSumMetricMap returns an instance of sumMetricMap
func newSumMetricMap() *sumMetricMap {
	return &sumMetricMap{aggMetricMap: make(models.MetricMap)}
}

// Step adds the element to slice
func (g *sumMetricMap) Step(m string) {
	var metricMap models.MetricMap
	if err := json.Unmarshal([]byte(m), &metricMap); err != nil {
		panic(err)
	}
	g.metricMaps = append(g.metricMaps, metricMap)
	g.n++
}

// Done aggregates all the elements added to slice
func (g *sumMetricMap) Done() string {
	for _, metricMap := range g.metricMaps {
		for metricName, metricValue := range metricMap {
			g.aggMetricMap[metricName] += metricValue
		}
	}

	// Finally, marshal the type into string and return
	aggMetricMapBytes, err := json.Marshal(g.aggMetricMap)
	if err != nil {
		panic(err)
	}
	return string(aggMetricMapBytes)
}

// avgMetricMap averages MetricMaps based on weights.
// For int or float types, they will be weighed averaged
// For string types, they will be ignored
type avgMetricMapAgg struct {
	metricMaps   []models.MetricMap
	weights      []float64
	avgMetricMap models.MetricMap
	totalWeights map[string]models.JSONFloat
	n            int64
}

// newAvgMetricMap returns an instance of avgMetricMap
func newAvgMetricMapAgg() *avgMetricMapAgg {
	return &avgMetricMapAgg{
		avgMetricMap: make(models.MetricMap),
		totalWeights: make(map[string]models.JSONFloat),
	}
}

// Step adds the element to slice
func (g *avgMetricMapAgg) Step(m string, w float64) {
	var metricMap models.MetricMap
	if err := json.Unmarshal([]byte(m), &metricMap); err != nil {
		panic(err)
	}
	g.metricMaps = append(g.metricMaps, metricMap)
	g.weights = append(g.weights, w)
	g.n++
}

// Done aggregates all the elements added to slice
func (g *avgMetricMapAgg) Done() string {
	// Get weighed sum of all metric maps
	for imetricMap, metricMap := range g.metricMaps {
		for metricName, metricValue := range metricMap {
			weight := models.JSONFloat(g.weights[imetricMap])
			g.avgMetricMap[metricName] += metricValue * weight
			g.totalWeights[metricName] += weight
		}
	}

	// Divide weighted sum by counts to get weighted average
	for metricName := range g.avgMetricMap {
		if totalWeight, ok := g.totalWeights[metricName]; ok {
			if totalWeight > 0 {
				g.avgMetricMap[metricName] = g.avgMetricMap[metricName] / totalWeight
			}
		}
	}

	// Finally, marshal the type into string and return
	avgMetricMapBytes, err := json.Marshal(g.avgMetricMap)
	if err != nil {
		panic(err)
	}
	return string(avgMetricMapBytes)
}
