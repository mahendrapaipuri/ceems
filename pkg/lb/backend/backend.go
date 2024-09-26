// Package backend implements the backend TSDB server of load balancer app
package backend

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

// Custom errors.
var (
	ErrTypeAssertion = errors.New("failed type assertion")
)

// TSDBServer is the interface each backend TSDB server needs to implement.
type TSDBServer interface {
	SetAlive(alive bool)
	IsAlive() bool
	URL() *url.URL
	String() string
	ActiveConnections() int
	RetentionPeriod() time.Duration
	Serve(w http.ResponseWriter, r *http.Request)
}

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
	logger          log.Logger
}

// New returns an instance of backend TSDB server.
func New(webURL *url.URL, p *httputil.ReverseProxy, logger log.Logger) TSDBServer {
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

		level.Debug(logger).Log("msg", "Basic auth configured for backend", "backend", webURL.Redacted())
	}

	return &tsdbServer{
		url:             webURL,
		alive:           true,
		reverseProxy:    p,
		basicAuthHeader: basicAuthHeader,
		updateInterval:  3 * time.Hour,
		client:          tsdbClient,
		logger:          logger,
	}
}

// String returns name/web URL backend TSDB server.
func (b *tsdbServer) String() string {
	if b.url != nil {
		return b.url.Redacted()
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
			level.Error(b.logger).Log("msg", "Failed to update retention period", "backend", b.String(), "err", err)

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
				return b.retentionPeriod, ErrTypeAssertion
			}

			for _, retentionString := range strings.Split(vString, "or") {
				period, err = model.ParseDuration(strings.TrimSpace(retentionString))
				if err != nil {
					continue
				}

				goto outside
			}
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
	// Make query parameters
	values := url.Values{
		"query": []string{fmt.Sprintf(`up{instance="%s:%s"}`, b.url.Hostname(), b.url.Port())},
		"start": []string{time.Now().Add(-time.Duration(period)).UTC().Format(time.RFC3339Nano)},
		"end":   []string{time.Now().UTC().Format(time.RFC3339Nano)},
		"step":  []string{"10m"},
	}

	queryURL := fmt.Sprintf("%s?%s", b.url.JoinPath("api/v1/query_range").String(), values.Encode())

	data, err = tsdb.Request(ctx, queryURL, b.client)
	if err != nil {
		return time.Duration(period), nil
	}

	queryData, ok := data.(map[string]interface{})
	if !ok {
		return time.Duration(period), nil
	}

	// Check if results is not nil before converting it to slice of interfaces
	if r, exists := queryData["result"]; exists && r != nil {
		var results, values []interface{}

		var result map[string]interface{}

		var ok bool
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
							return time.Since(time.Unix(int64(t), 0)).Truncate(time.Hour), nil
						}
					}
				}
			}
		}
	}

	return b.retentionPeriod, fmt.Errorf("failed to find retention period in runtime config: %s", runtimeData)
}
