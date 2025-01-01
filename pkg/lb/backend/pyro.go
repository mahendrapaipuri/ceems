package backend

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// pyroServer implements a given backend Pyroscope server.
type pyroServer struct {
	url             *url.URL
	alive           bool
	mux             sync.RWMutex
	connections     int
	reverseProxy    *httputil.ReverseProxy
	basicAuthHeader string
	client          *http.Client
	logger          *slog.Logger
}

// NewPyroscope returns an instance of backend Pyroscope server.
func NewPyroscope(webURL *url.URL, p *httputil.ReverseProxy, logger *slog.Logger) Server {
	// Create a client
	pyroClient := &http.Client{Timeout: 2 * time.Second}

	// Retrieve basic auth username and password if exists
	var basicAuthHeader string

	username := webURL.User.Username()
	password, exists := webURL.User.Password()

	if exists {
		auth := fmt.Sprintf("%s:%s", username, password)
		base64Auth := base64.StdEncoding.EncodeToString([]byte(auth))
		basicAuthHeader = "Basic " + base64Auth

		logger.Debug("Basic auth configured for backend Pyroscope server", "backend", webURL.Redacted())
	}

	return &pyroServer{
		url:             webURL,
		alive:           true,
		reverseProxy:    p,
		basicAuthHeader: basicAuthHeader,
		client:          pyroClient,
		logger:          logger,
	}
}

// Returns the retention period of backend Pyroscope server.
func (b *pyroServer) RetentionPeriod() time.Duration {
	// Return a very long duration so that query will always
	// get proxied to one of the backends
	return 10 * 365 * 24 * time.Hour
}

// String returns name/web URL backend Pyroscope server.
func (b *pyroServer) String() string {
	if b.url != nil {
		return b.url.Redacted()
	}

	return "No backend found"
}

// Returns current number of active connections.
func (b *pyroServer) ActiveConnections() int {
	b.mux.RLock()
	connections := b.connections
	b.mux.RUnlock()

	return connections
}

// Sets the backend Pyroscope server as alive.
func (b *pyroServer) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	b.mux.Unlock()
}

// Returns if backend Pyroscope server is alive.
func (b *pyroServer) IsAlive() bool {
	b.mux.RLock()
	alive := b.alive
	defer b.mux.RUnlock()

	return alive
}

// Returns URL of backend Pyroscope server.
func (b *pyroServer) URL() *url.URL {
	return b.url
}

// Serves the request by the backend Pyroscope server.
func (b *pyroServer) Serve(w http.ResponseWriter, r *http.Request) {
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
