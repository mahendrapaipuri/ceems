package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/procfs"
)

// CLI opts.
var (
	cgManager = CEEMSExporterApp.Flag(
		"discoverer.alloy-targets.resource-manager",
		"Discover Grafana Alloy targets from this resource manager [supported: slurm].",
	).Enum("slurm")
	alloyTargetEnvVars = CEEMSExporterApp.Flag(
		"discoverer.alloy-targets.env-var",
		"Enable continuous profiling by Grafana Alloy only on the processes having any of these environment variables.",
	).Strings()
)

const (
	contentTypeHeader = "Content-Type"
	contentType       = "application/json"
)

const (
	alloyTargetDiscovererSubSystem = "alloy_targets"
)

// Security context names.
const (
	alloyTargetDiscovererCtx = "alloy_targets_discoverer"
)

// alloyTargetDiscovererSecurityCtxData contains the input/output data for
// discoverer function to execute inside security context.
type alloyTargetDiscovererSecurityCtxData = perfDiscovererSecurityCtxData

type Target struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

type alloyTargetOpts struct {
	targetEnvVars []string
}

type CEEMSAlloyTargetDiscoverer struct {
	logger           log.Logger
	cgroupManager    *cgroupManager
	fs               procfs.FS
	opts             alloyTargetOpts
	enabled          bool
	securityContexts map[string]*security.SecurityContext
}

// NewAlloyTargetDiscoverer returns a new HTTP alloy discoverer.
func NewAlloyTargetDiscoverer(logger log.Logger) (*CEEMSAlloyTargetDiscoverer, error) {
	// If no resource manager is provided, return an instance with enabled set to false
	if *cgManager == "" {
		level.Warn(logger).Log("msg", "No resource manager selected for discoverer")

		return &CEEMSAlloyTargetDiscoverer{logger: logger, enabled: false}, nil
	}

	// Make alloyTargetOpts
	opts := alloyTargetOpts{
		targetEnvVars: *alloyTargetEnvVars,
	}

	// Instantiate a new Proc FS
	fs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	// Get SLURM's cgroup details
	cgroupManager, err := NewCgroupManager(*cgManager)
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup manager", "err", err)

		return nil, err
	}

	level.Info(logger).Log("cgroup", cgroupManager)

	discoverer := &CEEMSAlloyTargetDiscoverer{
		logger:        logger,
		fs:            fs,
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

		discoverer.securityContexts[alloyTargetDiscovererCtx], err = security.NewSecurityContext(
			alloyTargetDiscovererCtx,
			auxCaps,
			targetDiscoverer,
			logger,
		)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create a security context for alloy target discoverer", "err", err)

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
		level.Debug(d.logger).Log("msg", "discoverer failed", "duration_seconds", duration.Seconds())
	} else {
		level.Debug(d.logger).Log("msg", "discoverer succeeded", "duration_seconds", duration.Seconds())
	}

	return targets, err
}

// discover targets by reading processes and mapping them to cgroups.
func (d *CEEMSAlloyTargetDiscoverer) discover() ([]Target, error) {
	// If the discoverer is not enabled, return empty targets
	if !d.enabled {
		level.Debug(d.logger).Log("msg", "Grafana Alloy targets discoverer not enabled")

		return []Target{}, nil
	}

	// Read discovered cgroups into data pointer
	dataPtr := &alloyTargetDiscovererSecurityCtxData{
		procfs:        d.fs,
		cgroupManager: d.cgroupManager,
		targetEnvVars: d.opts.targetEnvVars,
	}

	// If there is a need to read processes' environ, use security context
	// else execute function natively
	if len(d.opts.targetEnvVars) > 0 {
		if securityCtx, ok := d.securityContexts[alloyTargetDiscovererCtx]; ok {
			if err := securityCtx.Exec(dataPtr); err != nil {
				return nil, err
			}
		} else {
			return nil, security.ErrNoSecurityCtx
		}
	} else {
		if err := targetDiscoverer(dataPtr); err != nil {
			return nil, err
		}
	}

	if len(dataPtr.cgroups) > 0 {
		level.Debug(d.logger).Log("msg", "Discovered targets for Grafana Alloy")
	} else {
		level.Debug(d.logger).Log("msg", "No targets found for Grafana Alloy")
	}

	// Make targets from cgrpoups
	var targets []Target

	for _, cgroup := range dataPtr.cgroups {
		for _, proc := range cgroup.procs {
			exe, _ := proc.Executable()
			comm, _ := proc.CmdLine()

			var realUID, effecUID uint64
			if status, err := proc.NewStatus(); err == nil {
				realUID = status.UIDs[0]
				effecUID = status.UIDs[1]
			}

			target := Target{
				Targets: []string{cgroup.id},
				Labels: map[string]string{
					"__process_pid__":         strconv.FormatInt(int64(proc.PID), 10),
					"__process_exe":           exe,
					"__process_commandline":   strings.Join(comm, " "),
					"__process_real_uid":      strconv.FormatUint(realUID, 10),
					"__process_effective_uid": strconv.FormatUint(effecUID, 10),
					"service_name":            cgroup.id,
				},
			}

			targets = append(targets, target)
		}
	}

	return targets, nil
}

// discoverer returns a map of discovered cgroup ID to procs by looking at each process
// in proc FS. Walking through cgroup fs is not really an option here as cgroups v1
// wont have all PIDs of cgroup if the PID controller is not turned on.
// The current implementation should work for both cgroups v1 and v2.
// This function might be executed in a security context if targetEnvVars is not
// empty.
func targetDiscoverer(data interface{}) error {
	// Assert data is of alloyTargetDiscovererSecurityCtxData
	var d *alloyTargetDiscovererSecurityCtxData

	var ok bool
	if d, ok = data.(*alloyTargetDiscovererSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	cgroups, err := getCgroups(d.procfs, d.cgroupManager.idRegex, d.targetEnvVars, d.cgroupManager.procFilter)
	if err != nil {
		return err
	}

	// Read cgroups proc map into d
	d.cgroups = cgroups

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
