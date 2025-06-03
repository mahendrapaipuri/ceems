package collector

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	ebpfspy "github.com/grafana/pyroscope/ebpf"
	"github.com/grafana/pyroscope/ebpf/cpp/demangle"
	ebpfmetrics "github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/prometheus/client_golang/prometheus"
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
func (c *SessionConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
func (c *PyroscopeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	logger        *slog.Logger
	logLevel      string
	enabled       bool
	configFile    string
	targetEnvVars []string
	selfProfile   bool
}

type Profiler struct {
	logger         *slog.Logger
	config         *ProfilerConfig
	session        ebpfspy.Session
	sessionOptions ebpfspy.SessionOptions
	targetsFinder  Discoverer
	externalLabels []*typesv1.LabelPair
	enabled        bool
}

// NeweBPFProfiler returns a new instance of continuous profiler based on eBPF.
func NeweBPFProfiler(c *profilerConfig) (*Profiler, error) {
	var err error

	// If profiler is not enabled, return early
	if !c.enabled {
		return &Profiler{logger: c.logger, enabled: false}, nil
	}

	// Make a new instance of discoverer that gathers targets
	discovererConfig := &discovererConfig{
		logger:        c.logger,
		enabled:       true,
		targetEnvVars: c.targetEnvVars,
		selfProfile:   c.selfProfile,
	}

	targets, err := NewTargetDiscoverer(discovererConfig)
	if err != nil {
		c.logger.Error("Failed to setup targets finder for eBPF based profiling", "err", err)

		return nil, err
	}

	// If no cgroupManager set on discoverer, we cannot gather targets
	if !targets.Enabled() {
		c.logger.Warn("eBPF based profiling is only available when one of slurm or k8s collectors are enabled")

		return &Profiler{logger: c.logger, enabled: false}, nil
	}

	// Initialise config
	var cfg *ProfilerConfig

	// When config file is provided, read config
	if c.configFile != "" {
		// Get absolute config file path
		configFilePath, err := filepath.Abs(c.configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path of the config file: %w", err)
		}

		// Make config from file
		cfg, err = common.MakeConfig[ProfilerConfig](configFilePath)
		if err != nil {
			c.logger.Error("Failed to parse profiling config file", "err", err)

			return nil, fmt.Errorf("failed to parse profiling config file: %w", err)
		}
	}

	// If profiler config is empty, create a new instance
	if cfg == nil {
		cfg = &ProfilerConfig{Profiler: CEEMSProfilerConfig{}}
	}

	// If SessionOptions is empty, use default options
	if cfg.Profiler.Session == (SessionConfig{}) {
		cfg.Profiler.Session = defaultSessionConfig
	}

	// If Pyroscope URL is empty, set it to default URL of Pyroscope
	if cfg.Profiler.Pyroscope.URL == "" {
		cfg.Profiler.Pyroscope.URL = defaultPyroscopeURL
	}

	// Make a new targetFinder
	targetOpts := sd.TargetsOptions{
		TargetsOnly:        true,
		ContainerCacheSize: 1024, // Not relevant in our case but setting it to non zero is essential
	}

	// We need go-kit/logger just to pass it to ebpf profiling session
	// No ideal as it will make go-kit/logger as direct dependency but we dont
	// have a lot of options here
	//
	// This helper will create a go-kit logger based on current slog.Logger
	// instance so that we get a unified logs
	gokitLogger := NewGokitLogger(c.logLevel, c.logger)

	targetFinder, err := sd.NewTargetFinder(os.DirFS("/"), gokitLogger, targetOpts)
	if err != nil {
		c.logger.Info("Failed to create new target finder", "err", err)

		return nil, err
	}

	// Make session options
	sessionOpts := convertSessionOptions(cfg)

	// New instance of session
	session, err := ebpfspy.NewSession(gokitLogger, targetFinder, sessionOpts)
	if err != nil {
		c.logger.Info("Failed to create new profiling session", "err", err)

		return nil, err
	}

	// Create external labels
	externalLabels := make([]*typesv1.LabelPair, 0, len(cfg.Profiler.Pyroscope.ExternalLabels))

	for name, value := range cfg.Profiler.Pyroscope.ExternalLabels {
		value := strings.ReplaceAll(value, hostnamePlaceholder, hostname)
		externalLabels = append(externalLabels, &typesv1.LabelPair{
			Name:  name,
			Value: value,
		})
	}

	// Finally setup required capabilities
	capabilities := []string{
		"cap_sys_ptrace",
		"cap_dac_read_search",
		"cap_bpf",
		"cap_perfmon",
		"cap_sys_resource",
	}

	if _, err = setupAppCaps(capabilities); err != nil {
		c.logger.Warn("Failed to parse capability name(s)", "err", err)
	}

	return &Profiler{
		logger:         c.logger,
		config:         cfg,
		session:        session,
		sessionOptions: sessionOpts,
		targetsFinder:  targets,
		externalLabels: externalLabels,
		enabled:        true,
	}, nil
}

// Enabled returns if profiler is enabled or not.
func (p *Profiler) Enabled() bool {
	return p.enabled
}

// Start a new profiling session.
func (p *Profiler) Start(ctx context.Context) error {
	p.logger.Debug("Starting profiling session")

	// Start a new profiling session
	if err := p.session.Start(); err != nil {
		p.logger.Error("Failed to start a profiling session", "err", err)

		return err
	}

	// Ingest profiles in a separate go routine
	profiles := make(chan *pushv1.PushRequest, 512)
	go func() {
		if err := p.ingest(ctx, profiles); err != nil {
			p.logger.Error("Failed to setup profiles ingest", "err", err)
		}
	}()

	// Start tickers
	discoverTicker := time.NewTicker(time.Duration(p.config.Profiler.Session.DiscoverInterval))
	collectTicker := time.NewTicker(time.Duration(p.config.Profiler.Session.CollectInterval))

	// Update targets and collect profiles
	for {
		select {
		case <-discoverTicker.C:
			p.session.UpdateTargets(p.convertTargetOptions())
		case <-collectTicker.C:
			if err := p.collectProfiles(ctx, profiles); err != nil {
				p.logger.Error("Failed to collect profiles", "err", err)
			}
		case <-ctx.Done():
			p.logger.Error("Context done. Stopping profiling")

			// Stop tickers.
			discoverTicker.Stop()
			collectTicker.Stop()

			return nil
		}
	}
}

// Stop current profiling session.
func (p *Profiler) Stop() {
	p.logger.Debug("Stopping profiling session")

	// Stop session
	p.session.Stop()
}

// collectProfiles fetches profiles from current session and sends them to ingester
// on profiles channel.
func (p *Profiler) collectProfiles(ctx context.Context, profiles chan *pushv1.PushRequest) error {
	// Build profiles
	builders := pprof.NewProfileBuilders(pprof.BuildersOptions{
		SampleRate:    int64(p.sessionOptions.SampleRate),
		PerPIDProfile: true,
	})
	if err := pprof.Collect(builders, p.session); err != nil {
		return err
	}

	bytesSent := 0

	for _, builder := range builders.Builders {
		// check if the context is done
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Setup profile sample labels
		protoLabels := make([]*typesv1.LabelPair, 0, builder.Labels.Len()+len(p.externalLabels))
		for _, label := range builder.Labels {
			protoLabels = append(protoLabels, &typesv1.LabelPair{
				Name:  label.Name,
				Value: label.Value,
			})
		}

		protoLabels = append(protoLabels, p.externalLabels...)

		// Read profile sample into buffer
		buf := bytes.NewBuffer(nil)
		if _, err := builder.Write(buf); err != nil {
			p.logger.Error("Failed to write profile data into buffer. Dropping sample", "target", builder.Labels.String(), "err", err)

			continue
		}

		rawProfile := buf.Bytes()
		bytesSent += len(rawProfile)

		// Push profile sample to Pyroscope server
		req := &pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
			Labels: protoLabels,
			Samples: []*pushv1.RawSample{{
				RawProfile: rawProfile,
			}},
		}}}
		select {
		case profiles <- req:
		default:
			p.logger.Error("Dropping this sample", "target", builder.Labels.String())
		}
	}

	p.logger.Debug("Collected ebpf profiles", "profiles", len(builders.Builders), "bytes", bytesSent)

	return nil
}

// ingest pushes the profile samples to Pyroscope server.
func (p *Profiler) ingest(ctx context.Context, profiles chan *pushv1.PushRequest) error {
	httpClient, err := config.NewClientFromConfig(p.config.Profiler.Pyroscope.HTTPClientConfig, "ceems_profiling")
	if err != nil {
		return err
	}

	// Setup a new client to push profile samples
	client := pushv1connect.NewPusherServiceClient(httpClient, p.config.Profiler.Pyroscope.URL)

	for {
		it := <-profiles

		if _, err := client.Push(ctx, connect.NewRequest(it)); err != nil {
			p.logger.Error("Failed to push profile sample", "err", err)
		}
	}
}

// convertTargetOptions converts the discovered Alloy targets to TargetOptions.
func (p *Profiler) convertTargetOptions() sd.TargetsOptions {
	// Discover new targets
	targets, err := p.targetsFinder.Discover()
	if err != nil {
		p.logger.Error("Failed to discover new targets", "err", err)

		return sd.TargetsOptions{}
	}

	// Convert AlloyTargets to TargetOptions
	discoveryTargets := make([]sd.DiscoveryTarget, len(targets))
	for itarget, target := range targets {
		discoveryTargets[itarget] = target.Labels
	}

	return sd.TargetsOptions{Targets: discoveryTargets, TargetsOnly: true}
}

// convertSessionOptions returns sessions options based on profiling config.
func convertSessionOptions(c *ProfilerConfig) ebpfspy.SessionOptions {
	return ebpfspy.SessionOptions{
		CollectUser:   c.Profiler.Session.CollectUser,
		CollectKernel: c.Profiler.Session.CollectKernel,
		PythonEnabled: c.Profiler.Session.PythonEnabled,
		CacheOptions: symtab.CacheOptions{
			PidCacheOptions: symtab.GCacheOptions{
				Size:       c.Profiler.Session.PIDCacheSize,
				KeepRounds: c.Profiler.Session.CacheRounds,
			},
			BuildIDCacheOptions: symtab.GCacheOptions{
				Size:       c.Profiler.Session.BuildIDCacheSize,
				KeepRounds: c.Profiler.Session.CacheRounds,
			},
			SameFileCacheOptions: symtab.GCacheOptions{
				Size:       c.Profiler.Session.SameFileCacheSize,
				KeepRounds: c.Profiler.Session.CacheRounds,
			},
		},
		SymbolOptions: symtab.SymbolOptions{
			GoTableFallback:    true,
			PythonFullFilePath: false,
			DemangleOptions:    demangle.ConvertDemangleOptions(c.Profiler.Session.Demangle),
		},
		Metrics:         ebpfmetrics.New(prometheus.NewRegistry()),
		SampleRate:      c.Profiler.Session.SampleRate,
		VerifierLogSize: 1024 * 1024 * 1024,
		BPFMapsOptions: ebpfspy.BPFMapsOptions{
			PIDMapSize:     c.Profiler.Session.SymbolsMapSize,
			SymbolsMapSize: c.Profiler.Session.PIDMapSize,
		},
	}
}
