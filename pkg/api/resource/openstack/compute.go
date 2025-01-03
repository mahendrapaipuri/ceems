package openstack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

var (
	inactiveStatus = []string{
		"SHUTOFF",
		"SUSPENDED",
		"SHELVED",
		"SHELVED_OFFLOADED",
		"ERROR",
	}
	deletedStatus = []string{
		"DELETED",
		"SOFT_DELETED",
	}

	serversLock = sync.RWMutex{}
	projectLock = sync.RWMutex{}
	errsLock    = sync.RWMutex{}
)

// Timespan is a custom type to format time.Duration.
type Timespan time.Duration

// Format formats the time.Duration.
func (t Timespan) Format(format string) string {
	z := time.Unix(0, 0).UTC()
	duration := time.Duration(t)
	day := 24 * time.Hour

	if duration > day {
		days := duration / day

		return fmt.Sprintf("%d-%s", days, z.Add(duration).Format(format))
	}

	return z.Add(duration).Format(format)
}

func (o *openstackManager) activeInstances(ctx context.Context, start time.Time, end time.Time) ([]models.Unit, error) {
	// Check if service is online
	if err := o.ping("compute"); err != nil {
		return nil, err
	}

	// Get current time location
	loc := end.Location()

	// Start a wait group
	wg := sync.WaitGroup{}

	// Increment by 2 one for active instances, one for deleted instances
	wg.Add(2)

	var allServers []Server

	var allErrs error

	// Active instances
	go func() {
		defer wg.Done()

		// Fetch active servers
		servers, err := o.fetchInstances(ctx, start, end, false)
		if err != nil {
			errsLock.Lock()
			allErrs = errors.Join(allErrs, fmt.Errorf("failed to fetch active instances: %w", err))
			errsLock.Unlock()

			return
		}

		serversLock.Lock()
		allServers = append(allServers, servers...)
		serversLock.Unlock()
	}()

	// Deleted instances
	go func() {
		defer wg.Done()

		// Fetch active servers
		servers, err := o.fetchInstances(ctx, start, end, true)
		if err != nil {
			errsLock.Lock()
			allErrs = errors.Join(allErrs, fmt.Errorf("failed to fetch active instances: %w", err))
			errsLock.Unlock()

			return
		}

		serversLock.Lock()
		allServers = append(allServers, servers...)
		serversLock.Unlock()
	}()

	// Wait all go routines
	wg.Wait()

	// If no servers found, return error(s)
	if allErrs != nil {
		return nil, allErrs
	}

	// // Check if there are any new flavors in list of instances
	// for _, server := range allServers {
	// 	if _, ok := o.activeFlavors[server.Flavor.ID]; !ok {
	// 		if err := o.updateFlavors(ctx); err != nil {
	// 			level.Info(o.logger).Log("Failed to update instance flavors for Openstack cluster", "id", o.cluster.ID, "err", err)
	// 		}

	// 		break
	// 	}
	// }

	// Transform servers into units
	units := make([]models.Unit, len(allServers))

	// Update interval period
	updateIntPeriod := end.Sub(start).Seconds()

	var iServer int

	for _, server := range allServers {
		// Convert CreatedAt, UpdatedAt, LaunchedAt, TerminatedAt to current time location
		server.CreatedAt = server.CreatedAt.In(loc)
		server.LaunchedAt = server.LaunchedAt.In(loc)
		server.UpdatedAt = server.UpdatedAt.In(loc)
		server.TerminatedAt = server.TerminatedAt.In(loc)

		// Get elapsed time of instance including shutdowns, suspended states
		elapsedTime := Timespan(end.Sub(server.LaunchedAt)).Format("15:04:05")

		// Initialise endedAt, endedAtTS
		endedAt := "N/A"

		var endedAtTS int64 = 0

		// Get actual running time of the instance within this
		// update period
		var activeTimeSeconds float64

		if slices.Contains(deletedStatus, server.Status) {
			// Override elapsed time for deleted instances
			elapsedTime = Timespan(server.TerminatedAt.Sub(server.LaunchedAt)).Format("15:04:05")

			// Get instance termination time
			endedAt = server.TerminatedAt.Format(osTimeFormat)
			endedAtTS = server.TerminatedAt.UnixMilli()

			// If the instance has been terminated in this update interval
			// update activeTime from start till termination
			if server.TerminatedAt.After(start) {
				activeTimeSeconds = server.TerminatedAt.Sub(start).Seconds()
			}
		} else if slices.Contains(inactiveStatus, server.Status) {
			// If the server status has changed in this update interval,
			// update activeTime from start till update time
			if server.UpdatedAt.After(start) {
				activeTimeSeconds = server.UpdatedAt.Sub(start).Seconds()
			}
		} else {
			// If the update time is after start of this interval, it means instance
			// has changed from inactive to active
			if server.UpdatedAt.After(start) {
				activeTimeSeconds = end.Sub(server.UpdatedAt).Seconds()
			} else {
				activeTimeSeconds = updateIntPeriod
			}
		}

		// Parse vCPUs
		var vcpu, cpuMem, vgpu float64
		// Ignore any errors during parsing. Should not happen
		vcpu = float64(server.Flavor.VCPUs)

		// RAM is always in MiB. Convert it to Bytes
		cpuMem = float64(server.Flavor.RAM)

		// Check if instance has vGPUs
		if v, ok := server.Flavor.ExtraSpecs["resources:VGPU"]; ok {
			// Ignore any errors during parsing. Should not happen
			vgpu, _ = strconv.ParseFloat(v, 64)
		}

		// Total time
		totalTime := models.MetricMap{
			"walltime":         models.JSONFloat(activeTimeSeconds),
			"alloc_cputime":    models.JSONFloat(vcpu * activeTimeSeconds),
			"alloc_cpumemtime": models.JSONFloat(cpuMem * activeTimeSeconds),
			"alloc_gputime":    models.JSONFloat(vgpu * activeTimeSeconds),
			"alloc_gpumemtime": models.JSONFloat(vgpu * activeTimeSeconds),
		}

		// Allocation
		allocation := models.Allocation{
			"vcpus":       server.Flavor.VCPUs,
			"mem":         server.Flavor.RAM,
			"disk":        server.Flavor.Disk,
			"swap":        server.Flavor.Swap,
			"name":        server.Flavor.Name,
			"extra_specs": server.Flavor.ExtraSpecs,
		}

		// Tags
		tags := models.Tag{
			"metadata":       server.Metadata,
			"tags":           server.Tags,
			"server_groups":  strings.Join(server.ServerGroups, ","),
			"hypervisor":     server.HypervisorHostname,
			"reservation_id": server.ReservationID,
			"power_state":    server.PowerState.String(),
			"az":             server.AvailabilityZone,
		}

		units[iServer] = models.Unit{
			ResourceManager: openstackVMManager,
			UUID:            server.ID,
			Name:            server.Name,
			Project:         o.userProjectsCache.projectIDNameMap[server.TenantID],
			User:            o.userProjectsCache.userIDNameMap[server.UserID],
			CreatedAt:       server.CreatedAt.Format(osTimeFormat),
			StartedAt:       server.LaunchedAt.Format(osTimeFormat),
			EndedAt:         endedAt,
			CreatedAtTS:     server.CreatedAt.UnixMilli(),
			StartedAtTS:     server.LaunchedAt.UnixMilli(),
			EndedAtTS:       endedAtTS,
			Elapsed:         elapsedTime,
			State:           server.Status,
			TotalTime:       totalTime,
			Allocation:      allocation,
			Tags:            tags,
		}

		iServer++
	}

	o.logger.Info("Openstack VM instances fetched", "cluster_id", o.cluster.ID, "start", start, "end", end, "num_instances", len(units))

	return units, nil
}

// fetchInstances fetches a list of active/deleted compute instances from Openstack cluster.
func (o *openstackManager) fetchInstances(ctx context.Context, start time.Time, end time.Time, deleted bool) ([]Server, error) {
	// Create a new GET request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		o.servers().String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to fetch Openstack instances: %w", err)
	}

	// Add token to request headers
	req, err = o.addTokenHeader(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to rotate api token for openstack cluster: %w", err)
	}

	// Add query parameters
	q := req.URL.Query()
	q.Add("all_tenants", "true")

	if deleted {
		q.Add("deleted", "true")
		q.Add("changes-since", start.Format(osTimeFormat))
		q.Add("changes-before", end.Format(osTimeFormat))
	}

	req.URL.RawQuery = q.Encode()

	// Get response
	resp, err := apiRequest[ServersResponse](req, o.client)
	if err != nil {
		return nil, fmt.Errorf("failed to complete request to fetch Openstack instances: %w", err)
	}

	return resp.Servers, nil
}

// // fetchFlavors fetches a list of active instance flavors from Openstack cluster.
// func (o *openstackManager) fetchFlavors(ctx context.Context) ([]Flavor, error) {
// 	// Create a new GET request
// 	req, err := http.NewRequestWithContext(
// 		ctx,
// 		http.MethodGet,
// 		o.flavors().String(),
// 		nil,
// 	)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Get response
// 	resp, err := apiRequest[FlavorsResponse](req, o.client)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return resp.Flavors, nil
// }

// func (o *openstackManager) updateFlavors(ctx context.Context) error {
// 	// Fetch current flavors and update map
// 	if flavors, err := o.fetchFlavors(ctx); err != nil {
// 		return err
// 	} else {
// 		for _, flavor := range flavors {
// 			o.activeFlavors[flavor.ID] = flavor
// 		}
// 	}

// 	return nil
// }
