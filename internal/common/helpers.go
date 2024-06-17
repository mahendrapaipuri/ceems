// Package common provides general utility helper functions and types
package common

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"
)

// GetUUIDFromString returns a UUID5 for given slice of strings
func GetUUIDFromString(stringSlice []string) (string, error) {
	s := strings.Join(stringSlice[:], ",")
	h := xxh3.HashString128(s).Bytes()
	uuid, err := uuid.FromBytes(h[:])
	return uuid.String(), err
}

// MakeConfig reads config file, merges with passed default config and returns updated
// config instance
func MakeConfig[T any](filePath string) (*T, error) {
	// If no config file path provided, return default config
	if filePath == "" {
		return new(T), fmt.Errorf("config file path missing")
	}

	// Read config file
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return new(T), err
	}

	// Update config from YAML file
	config := new(T)
	err = yaml.Unmarshal(configFile, config)
	if err != nil {
		return new(T), err
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
	return l.Addr().(*net.TCPAddr).Port, l, nil
}

// CreateGrafanaClient instantiates a new instance of Grafana client
func CreateGrafanaClient(config *GrafanaWebConfig, logger log.Logger) (*grafana.Grafana, error) {
	grafanaClient, err := grafana.NewGrafana(
		config.URL,
		config.HTTPClientConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Grafana client: %s", err)
	}
	if grafanaClient.Available() {
		if err := grafanaClient.Ping(); err != nil {
			//lint:ignore ST1005 Grafana is a noun and need to capitalize!
			return nil, fmt.Errorf("Grafana at %s is unreachable: %s", grafanaClient.URL.Redacted(), err)
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
		u = fmt.Sprintf("http://%s:%s/", hostname, port)
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
