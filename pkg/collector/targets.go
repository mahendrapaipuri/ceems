package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	labelPID         = "__process_pid__"
	labelServiceName = "service_name"
)

const selfTargetID = "__internal_ceems_exporter"

const (
	contentTypeHeader = "Content-Type"
	contentType       = "application/json"
)

// Security context names.
const (
	profilingTargetFilterCtx = "profiling_targets_filter"
)

// targetDiscovererSecurityCtxData contains the input/output data for
// discoverer function to execute inside security context.
type targetDiscovererSecurityCtxData = perfProcFilterSecurityCtxData

type Target struct {
	Targets []string           `json:"targets"`
	Labels  sd.DiscoveryTarget `json:"labels"`
}

type Discoverer interface {
	Discover() ([]Target, error)
	Enabled() bool
}

type discovererConfig struct {
	logger              *slog.Logger
	enabled             bool
	targetEnvVars       []string
	selfProfile         bool
	disableCapAwareness bool
}

type targetDiscoverer struct {
	logger           *slog.Logger
	cgroupManager    *cgroupManager
	targetEnvVars    []string
	selfProfile      bool
	enabled          bool
	securityContexts map[string]*security.SecurityContext
}

// NewTargetDiscoverer returns a new profiling target discoverer.
func NewTargetDiscoverer(c *discovererConfig) (Discoverer, error) {
	// If not enabled, return ealry
	if !c.enabled {
		return &targetDiscoverer{logger: c.logger, enabled: false}, nil
	}

	var cgManager manager

	// Check if either SLURM or k8s collector is enabled
	switch {
	case *collectorState["slurm"]:
		cgManager = slurm
	case *collectorState["k8s"]:
		cgManager = k8s
	}

	// If supported collector is not enabled
	if cgManager == 0 {
		return &targetDiscoverer{logger: c.logger, enabled: false}, nil
	}

	// Get resource manager's cgroup details
	cgroupManager, err := NewCgroupManager(cgManager, c.logger)
	if err != nil {
		c.logger.Info("Failed to create cgroup manager", "err", err)

		return nil, err
	}

	c.logger.Info("cgroup: " + cgroupManager.String())

	discoverer := &targetDiscoverer{
		logger:        c.logger,
		cgroupManager: cgroupManager,
		enabled:       c.enabled,
		targetEnvVars: c.targetEnvVars,
		selfProfile:   c.selfProfile,
	}

	// Setup new security context(s)
	discoverer.securityContexts = make(map[string]*security.SecurityContext)

	// If we need to inspect env vars of processes, we will need cap_sys_ptrace and
	// cap_dac_read_search caps
	if len(discoverer.targetEnvVars) > 0 {
		capabilities := []string{"cap_sys_ptrace", "cap_dac_read_search"}

		auxCaps, err := setupAppCaps(capabilities)
		if err != nil {
			c.logger.Warn("Failed to parse capability name(s)", "err", err)
		}

		// Setup security context
		cfg := &security.SCConfig{
			Name:         profilingTargetFilterCtx,
			Caps:         auxCaps,
			Func:         filterTargets,
			Logger:       c.logger,
			ExecNatively: c.disableCapAwareness,
		}

		discoverer.securityContexts[profilingTargetFilterCtx], err = security.NewSecurityContext(cfg)
		if err != nil {
			c.logger.Error("Failed to create a security context for profiling target discoverer", "err", err)

			return nil, err
		}
	}

	return discoverer, nil
}

// Discover targets for profiling.
func (d *targetDiscoverer) Discover() ([]Target, error) {
	// If the discoverer is not enabled, return empty targets
	if !d.enabled {
		d.logger.Debug("Profiling targets discoverer not enabled")

		return nil, errors.New("profiling targets discoverer not enabled")
	}

	begin := time.Now()
	targets, err := d.discover()
	duration := time.Since(begin)

	if err != nil {
		d.logger.Debug("discoverer failed", "duration_seconds", duration.Seconds())
	} else {
		d.logger.Debug("discoverer succeeded", "duration_seconds", duration.Seconds())
	}

	return targets, err
}

// Enabled returns status of discoverer.
func (d *targetDiscoverer) Enabled() bool {
	return d.enabled
}

// discover targets by reading processes and mapping them to cgroups.
func (d *targetDiscoverer) discover() ([]Target, error) {
	// Get active cgroups
	cgroups, err := d.cgroupManager.discover()
	if err != nil {
		return nil, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Read discovered cgroups into data pointer
	dataPtr := &targetDiscovererSecurityCtxData{
		cgroups:       cgroups,
		targetEnvVars: d.targetEnvVars,
		ignoreProc:    d.cgroupManager.ignoreProc,
	}

	// If there is a need to read processes' environ, use security context
	// else execute function natively
	if len(d.targetEnvVars) > 0 {
		if securityCtx, ok := d.securityContexts[profilingTargetFilterCtx]; ok {
			if err := securityCtx.Exec(dataPtr); err != nil {
				return nil, err
			}
		} else {
			return nil, security.ErrNoSecurityCtx
		}
	}

	if len(dataPtr.cgroups) == 0 {
		d.logger.Debug("No targets found for profiling")

		return nil, nil
	}

	// Make targets from cgrpoups
	var targets []Target

	for _, cgroup := range dataPtr.cgroups {
		for _, proc := range cgroup.procs {
			// Reading files in /proc is expensive. So, return minimal
			// info needed for target
			target := Target{
				Targets: []string{cgroup.id},
				Labels: map[string]string{
					labelPID:         strconv.FormatInt(int64(proc.PID), 10),
					labelServiceName: cgroup.uuid,
				},
			}

			targets = append(targets, target)
		}
	}

	// If self profiler is enabled add current process to targets
	if d.selfProfile {
		targets = append(targets, Target{
			Targets: []string{selfTargetID},
			Labels: map[string]string{
				labelPID:         strconv.FormatInt(int64(os.Getpid()), 10),
				labelServiceName: selfTargetID,
			},
		})
	}

	return targets, nil
}

// filterTargets filters the targets based on target env vars and return filtered targets.
func filterTargets(data any) error {
	// Assert data is of targetDiscovererSecurityCtxData
	var d *targetDiscovererSecurityCtxData

	var ok bool
	if d, ok = data.(*targetDiscovererSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	// Read filtered cgroups into d
	d.cgroups = cgroupProcFilterer(d.cgroups, d.targetEnvVars, d.ignoreProc)

	return nil
}

// TargetsHandlerFor returns http.Handler for Alloy targets.
func TargetsHandlerFor(discoverer Discoverer, opts promhttp.HandlerOpts) http.Handler {
	var inFlightSem chan struct{}

	if opts.MaxRequestsInFlight > 0 {
		inFlightSem = make(chan struct{}, opts.MaxRequestsInFlight)
	}

	h := http.HandlerFunc(func(rsp http.ResponseWriter, req *http.Request) {
		if inFlightSem != nil {
			select {
			case inFlightSem <- struct{}{}: // All good, carry on.
				defer func() { <-inFlightSem }()
			default:
				http.Error(rsp, fmt.Sprintf(
					"Limit of concurrent requests reached (%d), try again later.", opts.MaxRequestsInFlight,
				), http.StatusServiceUnavailable)

				return
			}
		}

		targets, err := discoverer.Discover()
		if err != nil {
			if opts.ErrorLog != nil {
				opts.ErrorLog.Println("error gathering targets:", err)
			}

			switch opts.ErrorHandling {
			case promhttp.PanicOnError:
				panic(err)
			case promhttp.ContinueOnError:
				if len(targets) == 0 {
					// Still report the error if no targets have been gathered.
					httpError(rsp, err)

					return
				}
			case promhttp.HTTPErrorOnError:
				httpError(rsp, err)

				return
			}
		}

		rsp.Header().Set(contentTypeHeader, contentType)
		httpEncode(rsp, targets)
	})

	if opts.Timeout <= 0 {
		return h
	}

	return http.TimeoutHandler(h, opts.Timeout, fmt.Sprintf(
		"Exceeded configured timeout of %v.\n",
		opts.Timeout,
	))
}

// httpEncode encodes response to http.ResponseWriter.
func httpEncode(rsp http.ResponseWriter, response []Target) {
	if err := json.NewEncoder(rsp).Encode(&response); err != nil {
		rsp.Write([]byte("KO"))
	}
}

// httpError calls http.Error with the provided error and http.StatusInternalServerError.
func httpError(rsp http.ResponseWriter, err error) {
	http.Error(
		rsp,
		"An error has occurred while gathering targets:\n\n"+err.Error(),
		http.StatusInternalServerError,
	)
}
