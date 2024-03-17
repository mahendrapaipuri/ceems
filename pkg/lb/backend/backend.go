// Package backend implements the backend TSDB server of load balancer app
package backend

import (
	"encoding/base64"
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

// TSDBServer is the interface each backend TSDB server needs to implement
type TSDBServer interface {
	SetAlive(bool)
	IsAlive() bool
	URL() *url.URL
	String() string
	ActiveConnections() int
	RetentionPeriod() time.Duration
	Serve(http.ResponseWriter, *http.Request)
}

// tsdbServer implements a given backend TSDB server
type tsdbServer struct {
	url             *url.URL
	alive           bool
	mux             sync.RWMutex
	connections     int
	retentionPeriod time.Duration
	reverseProxy    *httputil.ReverseProxy
	basicAuthHeader string
}

// NewTSDBServer returns an instance of backend TSDB server
func NewTSDBServer(webURL *url.URL, p *httputil.ReverseProxy, logger log.Logger) TSDBServer {
	var tsdbClient *http.Client
	var retentionPeriod time.Duration

	// Create a client
	tsdbClient = &http.Client{Timeout: time.Duration(2 * time.Second)}

	// Make a API request to TSDB
	if data, err := tsdb.Request(webURL.JoinPath("api/v1/status/runtimeinfo").String(), tsdbClient); err == nil {
		// Parse runtime config and get storageRetention
		runtimeData := data.(map[string]interface{})
		for k, v := range runtimeData {
			if k == "storageRetention" {
				for _, retentionString := range strings.Split(v.(string), "or") {
					period, err := model.ParseDuration(strings.TrimSpace(retentionString))
					if err != nil {
						continue
					}
					retentionPeriod = time.Duration(period)
					level.Debug(logger).
						Log("msg", "Retention period", "backend", webURL.Redacted(), "period", retentionPeriod)
				}
			}
		}
	}

	// Retrieve basic auth username and password if exists
	var basicAuthHeader string
	username := webURL.User.Username()
	password, exists := webURL.User.Password()
	if exists {
		auth := fmt.Sprintf("%s:%s", username, password)
		base64Auth := base64.StdEncoding.EncodeToString([]byte(auth))
		basicAuthHeader = fmt.Sprintf("Basic %s", base64Auth)
		level.Debug(logger).Log("msg", "Basic auth configured for backend", "backend", webURL.Redacted())
	}
	return &tsdbServer{
		url:             webURL,
		alive:           true,
		retentionPeriod: retentionPeriod,
		reverseProxy:    p,
		basicAuthHeader: basicAuthHeader,
	}
}

// String returns name/web URL backend TSDB server
func (b *tsdbServer) String() string {
	if b.url != nil {
		return b.url.Redacted()
	}
	return "No backend found"
}

// Returns the retention period of backend TSDB server
func (b *tsdbServer) RetentionPeriod() time.Duration {
	return b.retentionPeriod
}

// Returns current number of active connections
func (b *tsdbServer) ActiveConnections() int {
	b.mux.RLock()
	connections := b.connections
	b.mux.RUnlock()
	return connections
}

// Sets the backend TSDB server as alive
func (b *tsdbServer) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// Returns if backend TSDB server is alive
func (b *tsdbServer) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	defer b.mux.RUnlock()
	return alive
}

// Returns URL of backend TSDB server
func (b *tsdbServer) URL() *url.URL {
	return b.url
}

// Serves the request by the backend TSDB server
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
