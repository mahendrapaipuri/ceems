package base

import "log/slog"

const (
	AppName = "ceems_k8s_admission_controller"
)

// WebConfig makes HTTP web config from CLI args.
type WebConfig struct {
	Addresses         []string
	WebSystemdSocket  bool
	WebConfigFile     string
	EnableDebugServer bool
}

// Config makes a server config.
type Config struct {
	Logger *slog.Logger
	Web    WebConfig
}
