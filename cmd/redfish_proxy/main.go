package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/common"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
)

const (
	appName = "redfish_proxy"
)

// Default API Resources that proxy will allow.
var (
	defaultAllowedAPIResources = []string{
		"^/redfish/v1/$",
		"^/redfish/v1/Sessions$",
		"^/redfish/v1/SessionService/Sessions$",
		"^/redfish/v1/SessionService/Sessions/[a-zA-Z0-9-_]*$",
		"^/redfish/v1/Chassis$",
		"^/redfish/v1/Chassis/[a-zA-Z0-9-_]*$",
		"^/redfish/v1/Chassis/[a-zA-Z0-9-_]*/Power$",
	}
)

var app = kingpin.New(
	appName,
	"A Reverse proxy to Redfish API server.",
)

type Target struct {
	HostAddrs []string `yaml:"host_ip_addrs"`
	URL       *url.URL `yaml:"url"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *Target) UnmarshalYAML(unmarshal func(any) error) error {
	var tmp struct {
		HostAddrs []string `yaml:"host_ip_addrs"`
		URL       string   `yaml:"url"`
	}

	if err := unmarshal(&tmp); err != nil {
		return err
	}

	// Parse url string
	u, err := url.Parse(tmp.URL)
	if err != nil {
		return err
	}
	// url.Parse passes a lot of URL types. Need
	// to check Host and Scheme
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid url string: %s", tmp.URL)
	}

	// Set target
	t.HostAddrs = tmp.HostAddrs
	t.URL = u

	return nil
}

type ProxyConfig struct {
	Targets []Target `yaml:"targets"`
	// List of allowed API resources that will be proxied. Each
	// string must be a valid regular expression. Ensure
	// that each string use start and end delimiters (^$) to
	// ensure the entire string will be captured. All the strings
	// will be joined by | delimiter to form a regular expression.
	//
	// Default values for this will ensure to allow API requests
	// to root, sessions, chassis and power resources.
	// Ref: https://regex101.com/r/9dy4JE/1
	AllowedAPIResources []string `yaml:"allowed_api_resources"`
	// Deprecated: InSecure exists for historical compatibility
	// and should not be used. This must be configured under
	// `tls_config.insecure_skip_verify` from now on.
	Insecure                  bool                    `yaml:"insecure_skip_verify"`
	HTTPClientConfig          config.HTTPClientConfig `yaml:",inline"`
	allowedAPIResourcesRegexp *regexp.Regexp
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *ProxyConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	*r = ProxyConfig{}
	r.AllowedAPIResources = defaultAllowedAPIResources
	r.HTTPClientConfig = config.DefaultHTTPClientConfig

	type plain ProxyConfig

	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	// If InSecure is set to true
	if r.Insecure {
		r.HTTPClientConfig.TLSConfig = config.TLSConfig{
			InsecureSkipVerify: r.Insecure,
		}
	}

	var err error

	// Compile regex
	r.allowedAPIResourcesRegexp, err = regexp.Compile(strings.Join(r.AllowedAPIResources, "|"))
	if err != nil {
		return fmt.Errorf("invalid regexp in allowed_resources: %w", err)
	}

	return nil
}

type Redfish struct {
	Config ProxyConfig `yaml:"redfish_proxy"`
	// Deprecated: `redfish_config` exists for historical compatibility
	// and should not be used. This must be configured under
	// `redfish_proxy` from now on.
	ConfigDeprecated ProxyConfig `yaml:"redfish_config"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Redfish) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Redfish

	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	// If ConfigDeprecated.allowedAPIResourcesRegexp is non-nil and Config.allowedAPIResourcesRegexp is nil, config is set on
	// deprecated tag.
	if r.ConfigDeprecated.allowedAPIResourcesRegexp != nil && r.Config.allowedAPIResourcesRegexp == nil {
		r.Config = r.ConfigDeprecated
	}

	return nil
}

// WebConfig makes HTTP web config from CLI args.
type WebConfig struct {
	Addresses         []string
	WebSystemdSocket  bool
	WebConfigFile     string
	EnableDebugServer bool
}

// Config makes a server config.
type Config struct {
	Logger  *slog.Logger
	Web     WebConfig
	Redfish *Redfish
}

func main() {
	var (
		webConfigFile, configFile                                 string
		configFileExpandEnvVars, enableDebugServer, systemdSocket bool
		webListenAddresses                                        []string
		maxProcs                                                  int
	)

	// Config file CLI flags
	app.Flag(
		"config.file",
		"Path to configuration file of redfish proxy.",
	).Envar("REDFISH_PROXY_CONFIG_FILE").Default("").StringVar(&configFile)
	app.Flag(
		"config.file.expand-env-vars",
		"Any environment variables that are referenced in the config file will be expanded. To escape $ use $$ (default: false).",
	).Default("false").BoolVar(&configFileExpandEnvVars)

	// Web server CLI flags
	app.Flag(
		"web.listen-address",
		"Addresses on which to expose proxy server and web interface.",
	).Default(":5000").StringsVar(&webListenAddresses)
	app.Flag(
		"web.config.file",
		"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
	).Default("").StringVar(&webConfigFile)
	app.Flag(
		"web.debug-server",
		"Enable /debug/pprof profiling (default: disabled).",
	).Default("false").BoolVar(&enableDebugServer)

	if runtime.GOOS == "linux" {
		app.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Default("false").BoolVar(&systemdSocket)
	}

	// Runtime CLI flags
	app.Flag(
		"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
	).Envar("GOMAXPROCS").Default("1").IntVar(&maxProcs)

	// Setup logger config
	promslogConfig := &promslog.Config{}
	flag.AddFlags(app, promslogConfig)
	app.Version(version.Print(app.Name))
	app.UsageWriter(os.Stdout)
	app.HelpFlag.Short('h')

	if _, err := app.Parse(os.Args[1:]); err != nil {
		panic(err)
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+appName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Read config from file only when file path is provided
	var redfish *Redfish

	if configFile != "" {
		configFilePath, err := filepath.Abs(configFile)
		if err != nil {
			logger.Error("Failed to get absolute path of config file", "err", err)

			os.Exit(1)
		}

		// Make config from file
		redfish, err = common.MakeConfig[Redfish](configFilePath, configFileExpandEnvVars)
		if err != nil {
			logger.Error("Failed to parse Redfish proxy config file", "err", err)

			os.Exit(1)
		}
	} else {
		// If no config file provided, start with a default config
		redfish = &Redfish{}
	}

	// Check if config is provided with deprecated tag and if so, log a warning
	if redfish.ConfigDeprecated.allowedAPIResourcesRegexp != nil {
		logger.Warn("Redfish proxy config provided under redfish_config section which is deprecated. Move it under redfish_proxy")
	}

	// If webConfigFile is set, get absolute path
	var webConfigFilePath string

	var err error
	if webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(webConfigFile)
		if err != nil {
			logger.Error("Failed to get absolute path of web config file", "err", err)

			os.Exit(1)
		}
	}

	// Make a new config based
	config := &Config{
		Logger: logger,
		Web: WebConfig{
			Addresses:         webListenAddresses,
			WebSystemdSocket:  systemdSocket,
			WebConfigFile:     webConfigFilePath,
			EnableDebugServer: enableDebugServer,
		},
		Redfish: redfish,
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a new proxy instance
	server, err := NewRedfishProxyServer(config)
	if err != nil {
		panic(err)
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below.
	go func() {
		if err := server.Start(); err != nil {
			logger.Error("Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	logger.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Failed to gracefully shutdown server", "err", err)
	}

	logger.Info("Server exiting")
	logger.Info("See you next time!!")
}
