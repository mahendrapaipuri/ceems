// Package common provides general utility helper functions and types
package common

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"
)

// GenerateKey generates a reproducible key from a given URL string.
func GenerateKey(url string) uint64 {
	hash := fnv.New64a()
	hash.Write([]byte(url))

	return hash.Sum64()
}

// Round returns a value less than or equal to value that is multiple of nearest.
func Round(value int64, nearest int64) int64 {
	return (value / nearest) * nearest
}

// TimeTrack tracks execution time of each function.
func TimeTrack(start time.Time, name string, logger log.Logger) {
	elapsed := time.Since(start)
	level.Debug(logger).Log("msg", name, "elapsed_time", elapsed)
}

// SanitizeFloat replaces +/-Inf and NaN with zero.
func SanitizeFloat(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		// handle infinity, assign desired value to v
		return 0
	}

	return v
}

// GetUUIDFromString returns a UUID5 for given slice of strings.
func GetUUIDFromString(stringSlice []string) (string, error) {
	s := strings.Join(stringSlice, ",")
	h := xxh3.HashString128(s).Bytes()
	uuid, err := uuid.FromBytes(h[:])

	return uuid.String(), err
}

// MakeConfig reads config file, merges with passed default config and returns updated
// config instance.
func MakeConfig[T any](filePath string) (*T, error) {
	// Create a new pointer to config instance
	config := new(T)

	// If no config file path provided, return default config
	if filePath == "" {
		return config, errors.New("config file path missing")
	}

	// Read config file
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(configFile, config)
	if err != nil {
		return config, err
	}

	return config, nil
}

// GetFreePort in this case makes the closing of the listener the responsibility
// of the caller to allow for a guarantee that multiple random port allocations
// don't collide.
func GetFreePort() (int, *net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, nil, err
	}

	var tcpAddr *net.TCPAddr

	var ok bool
	if tcpAddr, ok = l.Addr().(*net.TCPAddr); !ok {
		return 0, nil, errors.New("failed type assertion")
	}

	return tcpAddr.Port, l, nil
}

// NewGrafanaClient instantiates a new instance of Grafana client.
func NewGrafanaClient(config *GrafanaWebConfig, logger log.Logger) (*grafana.Grafana, error) {
	grafanaClient, err := grafana.New(
		config.URL,
		config.HTTPClientConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Grafana client: %w", err)
	}

	if grafanaClient.Available() {
		if err := grafanaClient.Ping(); err != nil {
			//lint:ignore ST1005 Grafana is a noun and need to capitalize!
			return nil, fmt.Errorf( //nolint:stylecheck
				"Grafana at %s is unreachable: %w",
				grafanaClient.URL.Redacted(),
				err,
			)
		}
	}

	return grafanaClient, nil
}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
}

// ComputeExternalURL computes a sanitized external URL from a raw input. It infers unset
// URL parts from the OS and the given listen address.
func ComputeExternalURL(u, listenAddr string) (*url.URL, error) {
	if u == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		_, port, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return nil, err
		}

		u = fmt.Sprintf("http://%s/", net.JoinHostPort(hostname, port))
	}

	if startsOrEndsWithQuote(u) {
		return nil, errors.New("URL must not begin or end with quotes")
	}

	eu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(eu.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}

	eu.Path = ppref

	return eu, nil
}
