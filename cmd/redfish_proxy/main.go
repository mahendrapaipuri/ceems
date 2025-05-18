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
		"^/redfish/v1/Chassis$",
		"^/redfish/v1/Chassis/[a-zA-Z0-9-_]*$",
		"^/redfish/v1/Chassis/[a-zA-Z0-9-_]*/Power$",
	}
)

var (
	app = kingpin.New(
		appName,
		"A Reverse proxy to Redfish API server.",
	)
	webListenAddresses = app.Flag(
		"web.listen-address",
		"Addresses on which to expose proxy server and web interface.",
	).Default(":5000").Strings()
	webConfigFile = app.Flag(
		"web.config.file",
		"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
	).Default("").String()
	configFile = app.Flag(
		"config.file",
		"Path to configuration file of redfish proxy.",
	).Envar("REDFISH_PROXY_CONFIG_FILE").Default("").String()
	maxProcs = app.Flag(
		"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
	).Envar("GOMAXPROCS").Default("1").Int()
	enableDebugServer = app.Flag(
		"web.debug-server",
		"Enable /debug/pprof profiling (default: disabled).",
	).Default("false").Bool()
)

type Target struct {
	HostAddrs []string `yaml:"host_ip_addrs"`
	URL       *url.URL `yaml:"url"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *Target) UnmarshalYAML(unmarshal func(interface{}) error) error {
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

type Redfish struct {
	Config struct {
		Web struct {
			HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
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
			Insecure                  bool `yaml:"insecure_skip_verify"`
			allowedAPIResourcesRegexp *regexp.Regexp
		} `yaml:"web"`
		Targets []Target `yaml:"targets"`
	} `yaml:"redfish_config"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Redfish) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*r = Redfish{}
	r.Config.Web.AllowedAPIResources = defaultAllowedAPIResources
	r.Config.Web.HTTPClientConfig = config.DefaultHTTPClientConfig

	type plain Redfish

	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	// If InSecure is set to true
	if r.Config.Web.Insecure {
		r.Config.Web.HTTPClientConfig.TLSConfig = config.TLSConfig{
			InsecureSkipVerify: r.Config.Web.Insecure,
		}
	}

	var err error

	// Compile regex
	r.Config.Web.allowedAPIResourcesRegexp, err = regexp.Compile(strings.Join(r.Config.Web.AllowedAPIResources, "|"))
	if err != nil {
		return fmt.Errorf("invalid regexp in allowed_resources: %w", err)
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
	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }() //nolint:nlreturn
	if runtime.GOOS == "linux" {
		systemdSocket = app.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

	// Setup logger config
	promslogConfig := &promslog.Config{}
	flag.AddFlags(app, promslogConfig)
	app.Version(version.Print(app.Name))
	app.UsageWriter(os.Stdout)
	app.HelpFlag.Short('h')

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		panic(err)
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+appName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(*maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Read config from file only when file path is provided
	var redfish *Redfish

	if *configFile != "" {
		configFilePath, err := filepath.Abs(*configFile)
		if err != nil {
			logger.Error("Failed to get absolute path of config file", "err", err)

			os.Exit(1)
		}

		// Make config from file
		redfish, err = common.MakeConfig[Redfish](configFilePath)
		if err != nil {
			logger.Error("Failed to parse Redfish proxy config file", "err", err)

			os.Exit(1)
		}
	} else {
		// If no config file provided, start with a default config
		redfish = &Redfish{}
	}

	// If webConfigFile is set, get absolute path
	var webConfigFilePath string
	if *webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(*webConfigFile)
		if err != nil {
			logger.Error("Failed to get absolute path of web config file", "err", err)

			os.Exit(1)
		}
	}

	// Make a new config based
	config := &Config{
		Logger: logger,
		Web: WebConfig{
			Addresses:         *webListenAddresses,
			WebSystemdSocket:  *systemdSocket,
			WebConfigFile:     webConfigFilePath,
			EnableDebugServer: *enableDebugServer,
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
