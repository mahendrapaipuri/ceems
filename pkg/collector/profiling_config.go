package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

// Valid Demangle options.
var (
	validDemangleOpts = []string{"none", "simplified", "templates", "full"}
)

// Default session config
// Using same defaults used by Grafana Alloy
// Ref: https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/?pg=oss-alloy&plcmt=hero-btn-3.
var (
	defaultSessionConfig = SessionConfig{
		PIDCacheSize:      32,
		BuildIDCacheSize:  64,
		SymbolsMapSize:    16384,
		PIDMapSize:        2048,
		SameFileCacheSize: 8,
		CacheRounds:       3,
		CollectInterval:   model.Duration(30 * time.Second),
		DiscoverInterval:  model.Duration(30 * time.Second),
		CollectUser:       true,
		CollectKernel:     false,
		PythonEnabled:     true,
		SampleRate:        97,
		Demangle:          "none",
	}

	defaultPyroscopeURL = "http://localhost:4040"
)

type SessionConfig struct {
	CollectInterval   model.Duration `yaml:"collect_interval"`
	DiscoverInterval  model.Duration `yaml:"discover_interval"`
	CollectUser       bool           `yaml:"collect_user_profile"`
	CollectKernel     bool           `yaml:"collect_kernel_profile"`
	PythonEnabled     bool           `yaml:"python_enabled"`
	SampleRate        int            `yaml:"sample_rate"`
	Demangle          string         `yaml:"demangle"`
	BuildIDCacheSize  int            `yaml:"build_id_cache_size"`
	PIDCacheSize      int            `yaml:"pid_cache_size"`
	PIDMapSize        uint32         `yaml:"pid_map_size"`
	SameFileCacheSize int            `yaml:"same_file_cache_size"`
	SymbolsMapSize    uint32         `yaml:"symbols_map_size"`
	CacheRounds       int            `yaml:"cache_rounds"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SessionConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	*c = defaultSessionConfig

	type plain SessionConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Remove any spaces and convert to lower
	c.Demangle = strings.TrimSpace(strings.ToLower(c.Demangle))

	// Validate config
	if err := c.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate validates the config.
func (c *SessionConfig) Validate() error {
	// Check if demangle is none/simplified/templates/full
	if !slices.Contains(validDemangleOpts, c.Demangle) {
		return fmt.Errorf("invalid demangle options %s. expected one of %s", c.Demangle, strings.Join(validDemangleOpts, ","))
	}

	return nil
}

type PyroscopeConfig struct {
	URL              string                  `yaml:"url"`
	ExternalLabels   map[string]string       `yaml:"external_labels"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PyroscopeConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	*c = PyroscopeConfig{
		URL:              defaultPyroscopeURL,
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}

	type plain PyroscopeConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Validate config
	if err := c.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate validates the config.
func (c *PyroscopeConfig) Validate() error {
	// Check if URL is valid
	if _, err := url.Parse(c.URL); err != nil {
		return fmt.Errorf("invalid pyroscope URL: %w", err)
	}

	return nil
}

type CEEMSProfilerConfig struct {
	Session   SessionConfig   `yaml:"ebpf"`
	Pyroscope PyroscopeConfig `yaml:"pyroscope"`
}

type ProfilerConfig struct {
	Profiler CEEMSProfilerConfig `yaml:"ceems_profiler"`
}

type profilerConfig struct {
	logger                  *slog.Logger
	logLevel                string
	enabled                 bool
	configFile              string
	configFileExpandEnvVars bool
	targetEnvVars           []string
	selfProfile             bool
}

// Profiler is the interface different profilers must implement.
type Profiler interface {
	Start(ctx context.Context) error
	Stop()
	Enabled() bool
}
