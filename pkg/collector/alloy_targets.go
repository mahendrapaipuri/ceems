package collector

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// CLI opts.
var (
	enableDiscoverer = CEEMSExporterApp.Flag(
		"discoverer.alloy-targets",
		"Enable Grafana Alloy targets discoverer (default: false).",
	).Default("false").Bool()
	alloyTargetEnvVars = CEEMSExporterApp.Flag(
		"discoverer.alloy-targets.env-var",
		"Enable continuous profiling by Grafana Alloy only on the processes having any of these environment variables.",
	).Strings()
	alloySelfTarget = CEEMSExporterApp.Flag(
		"discoverer.alloy-targets.self-profiler",
		"Enable continuous profiling by Grafana Alloy on current process (default: false).",
	).Default("false").Bool()
)

const selfTargetID = "__internal_ceems_exporter"

const (
	contentTypeHeader = "Content-Type"
	contentType       = "application/json"
)

const (
	alloyTargetDiscovererSubSystem = "alloy_targets"
)

// Security context names.
const (
	alloyTargetFilterCtx = "alloy_targets_filter"
)

// alloyTargetDiscovererSecurityCtxData contains the input/output data for
// discoverer function to execute inside security context.
type alloyTargetFilterSecurityCtxData = perfProcFilterSecurityCtxData

type Target struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

type alloyTargetOpts struct {
	targetEnvVars []string
}

type CEEMSAlloyTargetDiscoverer struct {
	logger           *slog.Logger
	cgroupManager    *cgroupManager
	opts             alloyTargetOpts
	enabled          bool
	securityContexts map[string]*security.SecurityContext
}

// NewAlloyTargetDiscoverer returns a new HTTP alloy discoverer.
func NewAlloyTargetDiscoverer(logger *slog.Logger) (*CEEMSAlloyTargetDiscoverer, error) {
	var cgManager string

	// Check if either SLURM or k8s collector is enabled
	switch {
	case *collectorState["slurm"]:
		cgManager = "slurm"
	}

	// Discoverer is not enabled or supported collector is not enabled
	if !*enableDiscoverer || cgManager == "" {
		return &CEEMSAlloyTargetDiscoverer{logger: logger, enabled: false}, nil
	}

	// Get SLURM's cgroup details
	cgroupManager, err := NewCgroupManager(cgManager, logger)
	if err != nil {
		logger.Info("Failed to create cgroup manager", "err", err)

		return nil, err
	}

	logger.Info("cgroup: " + cgroupManager.String())

	// Make alloyTargetOpts
	opts := alloyTargetOpts{
		targetEnvVars: *alloyTargetEnvVars,
	}

	discoverer := &CEEMSAlloyTargetDiscoverer{
		logger:        logger,
		cgroupManager: cgroupManager,
		opts:          opts,
		enabled:       true,
	}

	// Setup new security context(s)
	// Security context for openining profilers
	discoverer.securityContexts = make(map[string]*security.SecurityContext)

	// If we need to inspect env vars of processes, we will need cap_sys_ptrace and
	// cap_dac_read_search caps
	if len(discoverer.opts.targetEnvVars) > 0 {
		capabilities := []string{"cap_sys_ptrace", "cap_dac_read_search"}
		auxCaps := setupCollectorCaps(logger, alloyTargetDiscovererSubSystem, capabilities)

		discoverer.securityContexts[alloyTargetFilterCtx], err = security.NewSecurityContext(
			alloyTargetFilterCtx,
			auxCaps,
			filterTargets,
			logger,
		)
		if err != nil {
			logger.Error("Failed to create a security context for alloy target discoverer", "err", err)

			return nil, err
		}
	}

	return discoverer, nil
}

// Discover targets for Grafana Alloy.
func (d *CEEMSAlloyTargetDiscoverer) Discover() ([]Target, error) {
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

// discover targets by reading processes and mapping them to cgroups.
func (d *CEEMSAlloyTargetDiscoverer) discover() ([]Target, error) {
	// If the discoverer is not enabled, return empty targets
	if !d.enabled {
		d.logger.Debug("Grafana Alloy targets discoverer not enabled")

		return []Target{}, nil
	}

	// Get active cgroups
	cgroups, err := d.cgroupManager.discover()
	if err != nil {
		return nil, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Read discovered cgroups into data pointer
	dataPtr := &alloyTargetFilterSecurityCtxData{
		cgroups:       cgroups,
		targetEnvVars: d.opts.targetEnvVars,
		ignoreProc:    d.cgroupManager.ignoreProc,
	}

	// If there is a need to read processes' environ, use security context
	// else execute function natively
	if len(d.opts.targetEnvVars) > 0 {
		if securityCtx, ok := d.securityContexts[alloyTargetFilterCtx]; ok {
			if err := securityCtx.Exec(dataPtr); err != nil {
				return nil, err
			}
		} else {
			return nil, security.ErrNoSecurityCtx
		}
	}

	if len(dataPtr.cgroups) == 0 {
		d.logger.Debug("No targets found for Grafana Alloy")

		return []Target{}, nil
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
					"__process_pid__": strconv.FormatInt(int64(proc.PID), 10),
					"service_name":    cgroup.uuid,
				},
			}

			targets = append(targets, target)
		}
	}

	// If self profiler is enabled add current process to targets
	if *alloySelfTarget {
		targets = append(targets, Target{
			Targets: []string{selfTargetID},
			Labels: map[string]string{
				"__process_pid__": strconv.FormatInt(int64(os.Getpid()), 10),
				"service_name":    selfTargetID,
			},
		})
	}

	return targets, nil
}

// filterTargets filters the targets based on target env vars and return filtered targets.
func filterTargets(data interface{}) error {
	// Assert data is of alloyTargetDiscovererSecurityCtxData
	var d *alloyTargetFilterSecurityCtxData

	var ok bool
	if d, ok = data.(*alloyTargetFilterSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	// Read filtered cgroups into d
	d.cgroups = cgroupProcFilterer(d.cgroups, d.targetEnvVars, d.ignoreProc)

	return nil
}

// TargetsHandlerFor returns http.Handler for Alloy targets.
func TargetsHandlerFor(discoverer *CEEMSAlloyTargetDiscoverer, opts promhttp.HandlerOpts) http.Handler {
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
				opts.ErrorLog.Println("error gathering metrics:", err)
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
		"An error has occurred while serving targets:\n\n"+err.Error(),
		http.StatusInternalServerError,
	)
}
