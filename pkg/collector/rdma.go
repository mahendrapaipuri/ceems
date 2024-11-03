//go:build !nordma
// +build !nordma

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/sysfs"
)

const rdmaCollectorSubsystem = "rdma"

// CLI opts.
var (
	rdmaStatsEnabled = CEEMSExporterApp.Flag(
		"collector.rdma.stats",
		"Enables collection of RDMA stats (default: disabled)",
	).Default("false").Bool()

	// test related opts.
	rdmaCmd = CEEMSExporterApp.Flag(
		"collector.rdma.cmd",
		"Path to rdma command",
	).Default("").Hidden().String()
)

type mr struct {
	num int
	len uint64
	dev string
}

type cq struct {
	num int
	len uint64
	dev string
}

type qp struct {
	num        int
	dev        string
	port       string
	hwCounters map[string]uint64
}

type rdmaCollector struct {
	sysfs            sysfs.FS
	procfs           procfs.FS
	logger           *slog.Logger
	cgroupManager    *cgroupManager
	hostname         string
	isAvailable      bool
	rdmaCmd          string
	qpModes          map[string]bool
	securityContexts map[string]*security.SecurityContext
	metricDescs      map[string]*prometheus.Desc
	hwCounters       []string
}

// Security context names.
const (
	rdmaExecCmdCtx = "rdma_exec_cmd"
)

// NewRDMACollector returns a new Collector exposing RAPL metrics.
func NewRDMACollector(logger *slog.Logger, cgManager *cgroupManager) (*rdmaCollector, error) {
	sysfs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}

	// Instantiate a new Proc FS
	procfs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		return nil, err
	}

	// Setup RDMA command
	var rdmaCmdPath string
	if *rdmaCmd != "" {
		rdmaCmdPath = *rdmaCmd
	} else {
		if rdmaCmdPath, err = exec.LookPath("rdma"); err != nil {
			logger.Error("rdma command not found. Not all RDMA metrics will be reported.", "err", err)
		}
	}

	// Check if RDMA devices exist
	_, err = sysfs.InfiniBandClass()
	if err != nil && errors.Is(err, os.ErrNotExist) {
		logger.Error("RDMA devices do not exist. RDMA collector wont return any data", "err", err)

		return &rdmaCollector{isAvailable: false}, nil
	}

	// Get current qp mode
	// We cannot turn on per PID counters when link is already being used by a process.
	// So we keep a state variable of modes of all links and attempt to turn them on
	// on every scrape request if they are not turned on already.
	// As this per PID counters are only supported by Mellanox devices, we setup
	// this map only for them. This map will be nil for other types of devices
	qpModes, err := qpMode(rdmaCmdPath)
	if err != nil {
		logger.Error("Failed to get RDMA qp mode", "err", err)
	}

	// If per QP counters are enabled, we need to disable them when exporter exits.
	// So create a security context with cap_setuid and cap_setgid to be able to
	// disable per QP counters
	//
	// Setup necessary capabilities.
	securityContexts := make(map[string]*security.SecurityContext)

	if len(qpModes) > 0 {
		logger.Info("Per-PID QP stats available")

		caps := setupCollectorCaps(logger, rdmaCollectorSubsystem, []string{"cap_setuid", "cap_setgid"})

		// Setup new security context(s)
		securityContexts[rdmaExecCmdCtx], err = security.NewSecurityContext(rdmaExecCmdCtx, caps, security.ExecAsUser, logger)
		if err != nil {
			logger.Error("Failed to create a security context for RDMA collector", "err", err)

			return nil, err
		}
	}

	// Port counters descriptions.
	portCountersDecs := map[string]string{
		"port_constraint_errors_received_total":    "Number of packets received on the switch physical port that are discarded",
		"port_constraint_errors_transmitted_total": "Number of packets not transmitted from the switch physical port",
		"port_data_received_bytes_total":           "Number of data octets received on all links",
		"port_data_transmitted_bytes_total":        "Number of data octets transmitted on all links",
		"port_discards_received_total":             "Number of inbound packets discarded by the port because the port is down or congested",
		"port_discards_transmitted_total":          "Number of outbound packets discarded by the port because the port is down or congested",
		"port_errors_received_total":               "Number of packets containing an error that were received on this port",
		"port_packets_received_total":              "Number of packets received on all VLs by this port (including errors)",
		"port_packets_transmitted_total":           "Number of packets transmitted on all VLs from this port (including errors)",
		"state_id":                                 "State of the InfiniBand port (0: no change, 1: down, 2: init, 3: armed, 4: active, 5: act defer)",
	}

	// HW counters descriptions.
	hwCountersDecs := map[string]string{
		"rx_write_requests":          "Number of received write requests for the associated QPs",
		"rx_read_requests":           "Number of received read requests for the associated QPs",
		"rx_atomic_requests":         "Number of received atomic request for the associated QPs",
		"req_cqe_error":              "Number of times requester detected CQEs completed with errors",
		"req_cqe_flush_error":        "Number of times requester detected CQEs completed with flushed errors",
		"req_remote_access_errors":   "Number of times requester detected remote access errors",
		"req_remote_invalid_request": "Number of times requester detected remote invalid request errors",
		"resp_cqe_error":             "Number of times responder detected CQEs completed with errors",
		"resp_cqe_flush_error":       "Number of times responder detected CQEs completed with flushed errors",
		"resp_local_length_error":    "Number of times responder detected local length errors",
		"resp_remote_access_errors":  "Number of times responder detected remote access errors",
	}

	// HW counters descriptions.
	wpsCountersDecs := map[string]string{
		"qps_active":     "Number of active QPs",
		"cqs_active":     "Number of active CQs",
		"mrs_active":     "Number of active MRs",
		"cqe_len_active": "Length of active CQs",
		"mrs_len_active": "Length of active MRs",
	}

	metricDescs := make(map[string]*prometheus.Desc)

	for metricName, description := range portCountersDecs {
		metricDescs[metricName] = prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, rdmaCollectorSubsystem, metricName),
			description,
			[]string{"manager", "hostname", "device", "port"},
			nil,
		)
	}

	var hwCounters []string
	for metricName, description := range hwCountersDecs {
		hwCounters = append(hwCounters, metricName)
		metricDescs[metricName] = prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, rdmaCollectorSubsystem, metricName),
			description,
			[]string{"manager", "hostname", "device", "port", "uuid"},
			nil,
		)
	}

	for metricName, description := range wpsCountersDecs {
		metricDescs[metricName] = prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, rdmaCollectorSubsystem, metricName),
			description,
			[]string{"manager", "hostname", "device", "port", "uuid"},
			nil,
		)
	}

	return &rdmaCollector{
		sysfs:            sysfs,
		procfs:           procfs,
		logger:           logger,
		cgroupManager:    cgManager,
		hostname:         hostname,
		rdmaCmd:          rdmaCmdPath,
		isAvailable:      true,
		qpModes:          qpModes,
		securityContexts: securityContexts,
		metricDescs:      metricDescs,
		hwCounters:       hwCounters,
	}, nil
}

// Update implements Collector and exposes RDMA related metrics.
func (c *rdmaCollector) Update(ch chan<- prometheus.Metric, cgroups []cgroup) error {
	if !c.isAvailable {
		return ErrNoData
	}

	// Check QP modes and attempt to enable PID if not already done
	if err := c.perPIDCounters(true); err != nil {
		c.logger.Error("Failed to enable Per-PID QP stats", "err", err)
	}

	return c.update(ch, cgroups)
}

// Stop releases system resources used by the collector.
func (c *rdmaCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", rdmaCollectorSubsystem)

	return c.perPIDCounters(false)
}

// perPIDCounters enables/disables per PID counters for supported devices.
func (c *rdmaCollector) perPIDCounters(enable bool) error {
	// If there no supported devices, return
	if len(c.qpModes) == 0 {
		return nil
	}

	// Return if there is no security context found
	securityCtx, ok := c.securityContexts[rdmaExecCmdCtx]
	if !ok {
		return security.ErrNoSecurityCtx
	}

	// Set per QP counters off when exiting
	var allErrs error

	for link, mode := range c.qpModes {
		if mode != enable {
			var cmd []string
			if enable {
				cmd = []string{"rdma", "statistic", "qp", "set", "link", link, "auto", "type,pid", "on"}
			} else {
				cmd = []string{"rdma", "statistic", "qp", "set", "link", link, "auto", "off"}
			}

			// Execute command as root
			dataPtr := &security.ExecSecurityCtxData{
				Cmd:    cmd,
				Logger: c.logger,
				UID:    0,
				GID:    0,
			}

			// If command didnt return error, we successfully enabled/disabled mode
			if err := securityCtx.Exec(dataPtr); err != nil {
				allErrs = errors.Join(allErrs, err)
			} else {
				c.qpModes[link] = enable
			}
		}
	}

	if allErrs != nil {
		return allErrs
	}

	return nil
}

// update fetches different RDMA stats.
func (c *rdmaCollector) update(ch chan<- prometheus.Metric, cgroups []cgroup) error {
	// Make invert mapping of cgroups
	procCgroup := c.procCgroupMapper(cgroups)

	// Initialise a wait group
	wg := sync.WaitGroup{}

	// Fetch MRs
	wg.Add(1)

	go func(p map[string]string) {
		defer wg.Done()

		mrs, err := c.devMR(p)
		if err != nil {
			c.logger.Error("Failed to fetch RDMA MR stats", "err", err)

			return
		}

		for uuid, mr := range mrs {
			ch <- prometheus.MustNewConstMetric(c.metricDescs["mrs_active"], prometheus.GaugeValue, float64(mr.num), c.cgroupManager.manager, c.hostname, mr.dev, "", uuid)
			ch <- prometheus.MustNewConstMetric(c.metricDescs["mrs_len_active"], prometheus.GaugeValue, float64(mr.len), c.cgroupManager.manager, c.hostname, mr.dev, "", uuid)
		}
	}(procCgroup)

	// Fetch CQs
	wg.Add(1)

	go func(p map[string]string) {
		defer wg.Done()

		cqs, err := c.devCQ(p)
		if err != nil {
			c.logger.Error("Failed to fetch RDMA CQ stats", "err", err)

			return
		}

		for uuid, cq := range cqs {
			ch <- prometheus.MustNewConstMetric(c.metricDescs["cqs_active"], prometheus.GaugeValue, float64(cq.num), c.cgroupManager.manager, c.hostname, cq.dev, "", uuid)
			ch <- prometheus.MustNewConstMetric(c.metricDescs["cqe_len_active"], prometheus.GaugeValue, float64(cq.len), c.cgroupManager.manager, c.hostname, cq.dev, "", uuid)
		}
	}(procCgroup)

	// Fetch QPs
	wg.Add(1)

	go func(p map[string]string) {
		defer wg.Done()

		qps, err := c.linkQP(p)
		if err != nil {
			c.logger.Error("Failed to fetch RDMA QP stats", "err", err)

			return
		}

		for uuid, qp := range qps {
			ch <- prometheus.MustNewConstMetric(c.metricDescs["qps_active"], prometheus.GaugeValue, float64(qp.num), c.cgroupManager.manager, c.hostname, qp.dev, qp.port, uuid)

			for _, hwCounter := range c.hwCounters {
				if qp.hwCounters[hwCounter] > 0 {
					ch <- prometheus.MustNewConstMetric(c.metricDescs[hwCounter], prometheus.CounterValue, float64(qp.hwCounters[hwCounter]), c.cgroupManager.manager, c.hostname, qp.dev, qp.port, uuid)
				}
			}
		}
	}(procCgroup)

	// Fetch sys wide counters
	wg.Add(1)

	go func() {
		defer wg.Done()

		counters, err := c.linkCountersSysWide()
		if err != nil {
			c.logger.Error("Failed to fetch system wide RDMA counters", "err", err)

			return
		}

		var vType prometheus.ValueType

		for link, cnts := range counters {
			l := strings.Split(link, "/")
			device := l[0]
			port := l[1]

			for n, v := range cnts {
				if v > 0 {
					if n == "state_id" {
						vType = prometheus.GaugeValue
					} else {
						vType = prometheus.CounterValue
					}
					ch <- prometheus.MustNewConstMetric(c.metricDescs[n], vType, float64(v), c.cgroupManager.manager, c.hostname, device, port)
				}
			}
		}
	}()

	// Wait for all go routines
	wg.Wait()

	return nil
}

// procCgroupMapper returns cgroup ID of all relevant processes map.
func (c *rdmaCollector) procCgroupMapper(cgroups []cgroup) map[string]string {
	// Make invert mapping of cgroups
	procCgroup := make(map[string]string)

	for _, cgroup := range cgroups {
		uuid := cgroup.uuid

		for _, proc := range cgroup.procs {
			p := strconv.FormatInt(int64(proc.PID), 10)
			procCgroup[p] = uuid
		}
	}

	return procCgroup
}

// devMR returns Memory Regions (MRs) stats of all active cgroups.
func (c *rdmaCollector) devMR(procCgroup map[string]string) (map[string]*mr, error) {
	// Arguments to command
	args := []string{"resource", "show", "mr"}

	// Execute command
	out, err := osexec.Execute(c.rdmaCmd, args, nil)
	if err != nil {
		return nil, err
	}

	// Define regexes
	devRegex := regexp.MustCompile(`^dev\s*([a-z0-9_]+)`)
	pidRegex := regexp.MustCompile(`.+?pid\s*([\d]+)`)
	mrlenRegex := regexp.MustCompile(`.+?mrlen\s*([\d]+)`)

	// Read line by line and match dev, pid and mrlen
	mrs := make(map[string]*mr)

	for _, line := range strings.Split(string(out), "\n") {
		if devMatch := devRegex.FindStringSubmatch(line); len(devMatch) > 1 {
			if pidMatch := pidRegex.FindStringSubmatch(line); len(pidMatch) > 1 {
				if uuid, ok := procCgroup[pidMatch[1]]; ok {
					if mrLenMatch := mrlenRegex.FindStringSubmatch(line); len(mrLenMatch) > 1 {
						if l, err := strconv.ParseUint(mrLenMatch[1], 10, 64); err == nil {
							if _, ok := mrs[uuid]; ok {
								mrs[uuid].num++
								mrs[uuid].len += l
							} else {
								mrs[uuid] = &mr{1, l, devMatch[1]}
							}
						}
					}
				}
			}
		}
	}

	return mrs, nil
}

// devCQ returns Completion Queues (CQs) stats of all active cgroups.
func (c *rdmaCollector) devCQ(procCgroup map[string]string) (map[string]*cq, error) {
	// Arguments to command
	args := []string{"resource", "show", "cq"}

	// Execute command
	out, err := osexec.Execute(c.rdmaCmd, args, nil)
	if err != nil {
		return nil, err
	}

	// Define regexes
	devRegex := regexp.MustCompile(`^dev\s*([a-z0-9_]+)`)
	pidRegex := regexp.MustCompile(`.+?pid\s*([\d]+)`)
	cqeRegex := regexp.MustCompile(`.+?cqe\s*([\d]+)`)

	// Read line by line and match dev, pid and mrlen
	cqs := make(map[string]*cq)

	for _, line := range strings.Split(string(out), "\n") {
		if devMatch := devRegex.FindStringSubmatch(line); len(devMatch) > 1 {
			if pidMatch := pidRegex.FindStringSubmatch(line); len(pidMatch) > 1 {
				if uuid, ok := procCgroup[pidMatch[1]]; ok {
					if cqeMatch := cqeRegex.FindStringSubmatch(line); len(cqeMatch) > 1 {
						if l, err := strconv.ParseUint(cqeMatch[1], 10, 64); err == nil {
							if _, ok := cqs[uuid]; ok {
								cqs[uuid].num++
								cqs[uuid].len += l
							} else {
								cqs[uuid] = &cq{1, l, devMatch[1]}
							}
						}
					}
				}
			}
		}
	}

	return cqs, nil
}

// linkQP returns Queue Pairs (QPs) stats of all active cgroups.
func (c *rdmaCollector) linkQP(procCgroup map[string]string) (map[string]*qp, error) {
	// Arguments to command
	args := []string{"resource", "show", "qp"}

	// Execute command
	out, err := osexec.Execute(c.rdmaCmd, args, nil)
	if err != nil {
		return nil, err
	}

	// Define regexes
	linkRegex := regexp.MustCompile(`^link\s*([a-z0-9_/]+)`)
	pidRegex := regexp.MustCompile(`.+?pid\s*([\d]+)`)

	// Read line by line and match dev, pid and mrlen
	qps := make(map[string]*qp)

	for _, line := range strings.Split(string(out), "\n") {
		if linkMatch := linkRegex.FindStringSubmatch(line); len(linkMatch) > 1 {
			if pidMatch := pidRegex.FindStringSubmatch(line); len(pidMatch) > 1 {
				if uuid, ok := procCgroup[pidMatch[1]]; ok {
					if _, ok := qps[uuid]; ok {
						qps[uuid].num++
					} else {
						link := strings.Split(linkMatch[1], "/")
						if len(link) == 2 {
							qps[uuid] = &qp{1, link[0], link[1], make(map[string]uint64)}
						}
					}
				}
			}
		}
	}

	// If per PID counters are enabled, fetch them
	if len(c.qpModes) > 0 {
		// Arguments to command
		args := []string{"statistic", "qp", "show"}

		// Execute command
		out, err := osexec.Execute(c.rdmaCmd, args, nil)
		if err != nil {
			c.logger.Error("Failed to fetch per PID QP stats", "err", err)

			return qps, nil
		}

		for _, line := range strings.Split(string(out), "\n") {
			if linkMatch := linkRegex.FindStringSubmatch(line); len(linkMatch) > 1 {
				for _, hwCounter := range c.hwCounters {
					if pidMatch := pidRegex.FindStringSubmatch(line); len(pidMatch) > 1 {
						if uuid, ok := procCgroup[pidMatch[1]]; ok {
							counterRegex := regexp.MustCompile(fmt.Sprintf(`.+?%s\s*([\d]+)`, hwCounter))
							if counterMatch := counterRegex.FindStringSubmatch(line); len(counterMatch) > 1 {
								if v, err := strconv.ParseUint(counterMatch[1], 10, 64); err == nil {
									if _, ok := qps[uuid]; !ok {
										link := strings.Split(linkMatch[1], "/")
										qps[uuid] = &qp{1, link[0], link[1], make(map[string]uint64)}
									}

									qps[uuid].hwCounters[hwCounter] = v
								}
							}
						}
					}
				}
			}
		}
	}

	return qps, nil
}

// linkCountersSysWide returns system wide counters of all RDMA devices.
func (c *rdmaCollector) linkCountersSysWide() (map[string]map[string]uint64, error) {
	devices, err := c.sysfs.InfiniBandClass()
	if err != nil {
		return nil, fmt.Errorf("error obtaining InfiniBand class info: %w", err)
	}

	counters := make(map[string]map[string]uint64)

	for _, device := range devices {
		for _, port := range device.Ports {
			link := fmt.Sprintf("%s/%d", device.Name, port.Port)
			counters[link] = map[string]uint64{
				"port_constraint_errors_received_total":    sanitizeMetric(port.Counters.PortRcvConstraintErrors),
				"port_constraint_errors_transmitted_total": sanitizeMetric(port.Counters.PortXmitConstraintErrors),
				"port_data_received_bytes_total":           sanitizeMetric(port.Counters.PortRcvData),
				"port_data_transmitted_bytes_total":        sanitizeMetric(port.Counters.PortXmitData),
				"port_discards_received_total":             sanitizeMetric(port.Counters.PortRcvDiscards),
				"port_discards_transmitted_total":          sanitizeMetric(port.Counters.PortXmitDiscards),
				"port_errors_received_total":               sanitizeMetric(port.Counters.PortRcvErrors),
				"port_packets_received_total":              sanitizeMetric(port.Counters.PortRcvPackets),
				"port_packets_transmitted_total":           sanitizeMetric(port.Counters.PortXmitPackets),
				"state_id":                                 uint64(port.StateID),
			}
		}
	}

	return counters, nil
}

// sanitizeMetric returns 0 if pointer is nil else metrics value.
func sanitizeMetric(value *uint64) uint64 {
	if value == nil {
		return 0
	}

	return *value
}

// qpMode returns current QP mode for all links.
func qpMode(rdmaCmd string) (map[string]bool, error) {
	args := []string{"statistic", "qp", "mode"}

	// Execute command
	out, err := osexec.Execute(rdmaCmd, args, nil)
	if err != nil {
		return nil, err
	}

	// Define regexes
	linkRegex := regexp.MustCompile(`^link\s*([a-z0-9_/]+)`)
	autoRegex := regexp.MustCompile(`.+?auto\s*([a-z,]+)`)

	// Split output and get mode for each device
	linkMode := make(map[string]bool)

	for _, line := range strings.Split(string(out), "\n") {
		if linkMatch := linkRegex.FindStringSubmatch(line); len(linkMatch) > 1 && strings.HasPrefix(linkMatch[1], "mlx") {
			if autoMatch := autoRegex.FindStringSubmatch(line); len(autoMatch) > 1 {
				if autoMatch[1] == "off" {
					linkMode[linkMatch[1]] = false
				} else {
					linkMode[linkMatch[1]] = true
				}
			}
		}
	}

	return linkMode, nil
}

// rdmaCollectorEnabled returns true if RDMA stats are enabled.
func rdmaCollectorEnabled() bool {
	return *rdmaStatsEnabled
}
