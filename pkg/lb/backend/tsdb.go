// Package backend implements the backend TSDB server of load balancer app
package backend

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

// tsdbServer implements a given backend TSDB server.
type tsdbServer struct {
	url             *url.URL
	alive           bool
	mux             sync.RWMutex
	connections     int
	retentionPeriod time.Duration
	lastUpdate      time.Time
	updateInterval  time.Duration
	reverseProxy    *httputil.ReverseProxy
	basicAuthHeader string
	client          *http.Client
	logger          *slog.Logger
}

// NewTSDB returns an instance of backend TSDB server.
func NewTSDB(webURL *url.URL, p *httputil.ReverseProxy, logger *slog.Logger) Server {
	// Create a client
	tsdbClient := &http.Client{Timeout: 2 * time.Second}

	// Retrieve basic auth username and password if exists
	var basicAuthHeader string

	username := webURL.User.Username()
	password, exists := webURL.User.Password()

	if exists {
		auth := fmt.Sprintf("%s:%s", username, password)
		base64Auth := base64.StdEncoding.EncodeToString([]byte(auth))
		basicAuthHeader = "Basic " + base64Auth

		logger.Debug("Basic auth configured for backend TSDB server", "backend", webURL.Redacted())
	}

	// Make server struct
	server := &tsdbServer{
		url:             webURL,
		alive:           true,
		reverseProxy:    p,
		basicAuthHeader: basicAuthHeader,
		updateInterval:  3 * time.Hour,
		client:          tsdbClient,
		logger:          logger,
	}

	// Update retention period
	server.RetentionPeriod()

	return server
}

// String returns name/web URL backend TSDB server.
func (b *tsdbServer) String() string {
	if b.url != nil {
		return fmt.Sprintf("url: %s; retention: %s", b.url.Redacted(), b.retentionPeriod)
	}

	return "No backend found"
}

// Returns the retention period of backend TSDB server.
func (b *tsdbServer) RetentionPeriod() time.Duration {
	b.mux.RLock()
	retentionPeriod := b.retentionPeriod
	lastUpdate := b.lastUpdate
	b.mux.RUnlock()

	// Update retention period for every 3 hours
	if retentionPeriod == 0*time.Second || lastUpdate.IsZero() ||
		time.Since(lastUpdate) > b.updateInterval {
		newRetentionPeriod, err := b.fetchRetentionPeriod()
		// If errored, return last retention period
		if err != nil {
			b.logger.Error("Failed to update retention period", "backend", b.String(), "err", err)

			return retentionPeriod
		}

		// If update is successful, update struct and return new retention period
		b.mux.Lock()
		b.retentionPeriod = newRetentionPeriod
		b.lastUpdate = time.Now()
		b.mux.Unlock()

		return newRetentionPeriod
	}

	// If there is no need to update, return last updated value
	return retentionPeriod
}

// Returns current number of active connections.
func (b *tsdbServer) ActiveConnections() int {
	b.mux.RLock()
	connections := b.connections
	b.mux.RUnlock()

	return connections
}

// Sets the backend TSDB server as alive.
func (b *tsdbServer) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// Returns if backend TSDB server is alive.
func (b *tsdbServer) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	defer b.mux.RUnlock()

	return alive
}

// Returns URL of backend TSDB server.
func (b *tsdbServer) URL() *url.URL {
	return b.url
}

// Serves the request by the backend TSDB server.
func (b *tsdbServer) Serve(w http.ResponseWriter, r *http.Request) {
	defer func() {
		b.mux.Lock()
		b.connections--
		b.mux.Unlock()
	}()

	// Request header at this point will contain basic auth header of LB
	// If backend server has basic auth as well, we need to swap it to the
	// one from backend server
	if b.basicAuthHeader != "" {
		// Check if basic Auth header already exists and remove it
		r.Header.Del("Authorization")
		r.Header.Add("Authorization", b.basicAuthHeader)
	}

	b.mux.Lock()
	b.connections++
	b.mux.Unlock()
	b.reverseProxy.ServeHTTP(w, r)
}

// Fetches retention period from backend TSDB server.
func (b *tsdbServer) fetchRetentionPeriod() (time.Duration, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Make a API request to TSDB
	data, err := tsdb.Request(ctx, b.url.JoinPath("api/v1/status/runtimeinfo").String(), b.client)
	if err != nil {
		return b.retentionPeriod, fmt.Errorf("failed to make API request to backend: %w", err)
	}

	// Parse runtime config and get storageRetention
	runtimeData, ok := data.(map[string]interface{})
	if !ok {
		return b.retentionPeriod, ErrTypeAssertion
	}

	// Use an initial value so that even if we do not find it
	// in runtime config, we return a sane default
	period := model.Duration(b.retentionPeriod)

	for k, v := range runtimeData {
		if k == "storageRetention" {
			vString, ok := v.(string)
			if !ok {
				return time.Duration(period), ErrTypeAssertion
			}

			for _, retentionString := range strings.Split(vString, "or") {
				period, err = model.ParseDuration(strings.TrimSpace(retentionString))
				if err != nil {
					continue
				}

				goto outside
			}

			return time.Duration(
					period,
				), fmt.Errorf(
					"failed to find retention period in runtime config: %s",
					runtimeData,
				)
		}
	}

outside:

	// If both retention storage and retention period are set,
	// depending on whichever hit first, TSDB uses the data based
	// on that retention.
	// So just becase retention period is, say 30d, we might not
	// have data spanning 30d if retention size cannot accommodate
	// 30 days of data.
	//
	// Check if the data is actually available on TSDB by making
	// a range query on "up" from now to now - retention_period
	//
	// Seems like default limit of number of points per timeseries
	// is 11k and so we need to choose a step that should keep
	// the number of point below this limit. We choose 5k as limit
	// to be safe. Infact we dont need all the points but just
	// the first one.
	//
	// Make query parameters
	step := time.Duration(period) / 5000

	urlValues := url.Values{
		"query": []string{fmt.Sprintf(`up{instance="%s:%s"}`, b.url.Hostname(), b.url.Port())},
		"start": []string{time.Now().Add(-time.Duration(period)).UTC().Format(time.RFC3339Nano)},
		"end":   []string{time.Now().UTC().Format(time.RFC3339Nano)},
		"step":  []string{step.Truncate(time.Second).String()},
	}

	queryURL := fmt.Sprintf("%s?%s", b.url.JoinPath("api/v1/query_range").String(), urlValues.Encode())

	data, err = tsdb.Request(ctx, queryURL, b.client)
	if err != nil {
		return time.Duration(period), nil
	}

	queryData, ok := data.(map[string]interface{})
	if !ok {
		return time.Duration(period), nil
	}

	// Check if results is not nil before converting it to slice of interfaces
	r, exists := queryData["result"]
	if !exists || r == nil {
		return time.Duration(period), nil
	}

	var results, values []interface{}

	var result map[string]interface{}

	if results, ok = r.([]interface{}); !ok {
		return time.Duration(period), nil
	}

	for _, res := range results {
		if result, ok = res.(map[string]interface{}); !ok {
			continue
		}

		if val, exists := result["values"]; exists {
			if values, ok = val.([]interface{}); ok && len(values) > 0 {
				if v, ok := values[0].([]interface{}); ok && len(v) > 0 {
					if t, ok := v[0].(float64); ok {
						// We are updating retention period only at updateInterval
						// so we need to reduce the actual retention period.
						// Here we reduce twice the update interval just to be
						// in a safe land
						return (time.Since(time.Unix(int64(t), 0)) - 2*b.updateInterval).Truncate(time.Hour), nil
					}
				}
			}
		}
	}

	return time.Duration(period), nil
}
