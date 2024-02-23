// Package backend implements the backend TSDB server of load balancer app
package backend

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

// TSDBServer is the interface each backend TSDB server needs to implement
type TSDBServer interface {
	SetAlive(bool)
	IsAlive() bool
	URL() *url.URL
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
}

// NewTSDBServer returns an instance of backend TSDB server
func NewTSDBServer(webURL *url.URL, skipTLSVerify bool, p *httputil.ReverseProxy) TSDBServer {
	var tsdbClient *http.Client
	var retentionPeriod time.Duration

	// Skip TLS verification
	if skipTLSVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		tsdbClient = &http.Client{Transport: tr, Timeout: time.Duration(2 * time.Second)}
	} else {
		tsdbClient = &http.Client{Timeout: time.Duration(2 * time.Second)}
	}

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
				}
			}
		}
	}
	return &tsdbServer{
		url:             webURL,
		alive:           true,
		retentionPeriod: retentionPeriod,
		reverseProxy:    p,
	}
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

	b.mux.Lock()
	b.connections++
	b.mux.Unlock()
	b.reverseProxy.ServeHTTP(w, r)
}
