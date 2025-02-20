package backend

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Custom errors.
var (
	ErrTypeAssertion = errors.New("failed type assertion")
)

// Server is the interface each backend server (TSDB/Pyroscope) needs to implement.
type Server interface {
	SetAlive(alive bool)
	IsAlive() bool
	URL() *url.URL
	String() string
	ActiveConnections() int
	RetentionPeriod() time.Duration
	ReverseProxy() *httputil.ReverseProxy
	Serve(w http.ResponseWriter, r *http.Request)
}

// ServerConfig contains the configuration of backend server.
type ServerConfig struct {
	Web          *models.WebConfig `yaml:"web"`
	FilterLabels []string          `yaml:"filter_labels"`
}

// Backend defines backend server.
type Backend struct {
	ID    string          `yaml:"id"`
	TSDBs []*ServerConfig `yaml:"tsdb"`
	Pyros []*ServerConfig `yaml:"pyroscope"`
}
