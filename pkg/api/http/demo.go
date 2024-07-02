package http

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"time"

	google_uuid "github.com/google/uuid"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Number of units and usage stats to generate
const (
	numUnits       = 100
	numUsage       = 50
	maxInt64 int64 = 1<<63 - 1
)

// Resource manager specific definitions
var (
	resourceMgrs = []string{"slurm", "openstack", "k8s"}
	states       = map[string][]string{
		"openstack": {
			"ACTIVE", "BUILD", "DELETED", "ERROR", "HARD_REBOOT", "MIGRATING",
			"PAUSED", "REBOOT", "SHUTOFF", "SOFT_DELETED", "SUSPENDED", "UNKNOWN",
		},
		"k8s": {"Pending", "Running", "Succeeded", "Failed"},
		"slurm": {
			"CANCELLED", "COMPLETED", "PENDING", "RUNNING", "REQUEUED", "STOPPED",
			"TIMEOUT",
		},
	}
	runningStates = map[string]string{
		"slurm":     "RUNNING",
		"openstack": "ACTIVE",
		"k8s":       "Running",
	}
	projects = map[string][]string{
		"slurm":     {"acc1", "acc2", "acc3", "acc4", "acc5"},
		"openstack": {"tenant1", "tenant2", "tenant3"},
		"k8s":       {"ns1", "ns2", "ns3", "ns4", "ns5", "ns6"},
	}
	users = []string{
		"user1", "user2", "user3", "user4", "user5", "user6", "user7",
	}
	allProjects []string
)

// Get a slice of all projects
func init() {
	for _, p := range projects {
		allProjects = append(allProjects, p...)
	}
}

// randomFloats returns random float64s in the range
func randomFloats(min, max float64) models.JSONFloat {
	return models.JSONFloat(min + rand.Float64()*(max-min)) // #nosec
}

// random returns random number between min and max excluding max
func random(min, max int64) int64 {
	return randomHelper(max-min-1) + min
}

// randomHelper returns max int64 if n is more than max
func randomHelper(n int64) int64 {
	if n < maxInt64 {
		return int64(rand.Int63n(int64(n + 1))) // #nosec
	}
	x := int64(rand.Uint64()) // #nosec
	for x > n {
		x = int64(rand.Uint64()) // #nosec
	}
	return x
}

// mockUnits will generate units with randomised data
func mockUnits() []models.Unit {
	// Define mock group, projects
	user := users[0]
	group := "group"
	numResourceMgrs := len(resourceMgrs)

	// Current time in epoch
	currentEpoch := time.Now().Local().Unix()

	// Minimum start time. Using 1 day before current time
	minStartTime := currentEpoch - 86400

	// Minimum end time. Must in last 30min
	minEndTime := currentEpoch - 1800

	// Max waiting time between creation and start time in seconds
	var maxWait int64 = 7200

	// Generate units
	var units = make([]models.Unit, numUnits)
	for i := 0; i < numUnits; i++ {
		resourceMgr := resourceMgrs[random(0, int64(numResourceMgrs))]
		clusterID := fmt.Sprintf("%s-%d", resourceMgr, random(0, int64(3)))

		// Use manager specific uuid
		var uuid string
		if resourceMgr == "slurm" {
			uuid = strconv.FormatInt(currentEpoch+int64(i), 10)
		} else {
			uuid = google_uuid.New().String()
		}

		// Get random project based on manager
		project := projects[resourceMgr][random(0, 2)]

		// Name is always demo followed by ID
		name := fmt.Sprintf("demo-%d", numUnits-i)

		// Generate a random start time based on current and min start time
		startTimeTS := random(minStartTime, currentEpoch)
		createTimeTS := startTimeTS - random(5, maxWait)
		startedAt := time.Unix(startTimeTS, 0).Format(time.RFC1123)

		// First 20 jobs must be running and rest should have different status
		var state, endedAt string
		var endTimeTS, elapsedRaw int64
		if i < 20 {
			state = runningStates[resourceMgr]
			endTimeTS = 0
			endedAt = "Unknown"
			elapsedRaw = currentEpoch - startTimeTS
		} else {
			state = states[resourceMgr][random(0, int64(len(states[resourceMgr])))]
			endTimeTS = random(minEndTime, currentEpoch)
			endedAt = time.Unix(endTimeTS, 0).Format(time.RFC1123)
			elapsedRaw = endTimeTS - startTimeTS
		}

		// If state is pending, starttime, elapsed must be zero
		avgUsageFlag := models.JSONFloat(1)
		if slices.Contains([]string{"PENDING", "Pending", "REQUEUED", "BUILD", "UNKNOWN"}, state) {
			startTimeTS = 0
			elapsedRaw = 0
			startedAt = "Unknown"
			endTimeTS = 0
			endedAt = "Unknown"
			avgUsageFlag = 0
		}

		units[i] = models.Unit{
			ID:              int64(i),
			ResourceManager: resourceMgr,
			ClusterID:       clusterID,
			UUID:            uuid,
			Name:            name,
			Project:         project,
			User:            user,
			Group:           group,
			CreatedAt:       time.Unix(createTimeTS, 0).Format(time.RFC1123),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
			CreatedAtTS:     createTimeTS,
			StartedAtTS:     startTimeTS,
			EndedAtTS:       endTimeTS,
			Elapsed:         time.Duration(elapsedRaw * int64(time.Second)).String(),
			State:           state,
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(elapsedRaw),
				"alloc_cputime":    models.JSONFloat(2 * elapsedRaw),
				"alloc_gputime":    models.JSONFloat(5 * elapsedRaw),
				"alloc_cpumemtime": models.JSONFloat(2 * 2000 * elapsedRaw),
				"alloc_gpumemtime": models.JSONFloat(5 * 8000 * elapsedRaw),
			},
			AveCPUUsage:         models.MetricMap{"usage": avgUsageFlag * randomFloats(0, 100)},
			AveCPUMemUsage:      models.MetricMap{"usage": avgUsageFlag * randomFloats(0, 100)},
			TotalCPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(1.1 * float64(elapsedRaw))},
			TotalCPUEmissions:   models.MetricMap{"rte": models.JSONFloat(17 * float64(elapsedRaw))},
			AveGPUUsage:         models.MetricMap{"usage": avgUsageFlag * randomFloats(0, 100)},
			AveGPUMemUsage:      models.MetricMap{"usage": avgUsageFlag * randomFloats(0, 100)},
			TotalGPUEnergyUsage: models.MetricMap{"usage": models.JSONFloat(15 * float64(elapsedRaw))},
			TotalGPUEmissions:   models.MetricMap{"rte": models.JSONFloat(158 * float64(elapsedRaw))},
			TotalIOWriteStats: models.MetricMap{
				"bytes":    models.JSONFloat(random(1000000, 1000000000)),
				"requests": models.JSONFloat(random(1000000, 1000000000)),
			},
			TotalIOReadStats: models.MetricMap{
				"bytes":    models.JSONFloat(random(1000000, 1000000000)),
				"requests": models.JSONFloat(random(1000000, 1000000000)),
			},
			TotalIngressStats: models.MetricMap{
				"bytes":   models.JSONFloat(random(1000000, 1000000000)),
				"packets": models.JSONFloat(random(1000000, 1000000000)),
			},
			TotalOutgressStats: models.MetricMap{
				"bytes":   models.JSONFloat(random(1000000, 1000000000)),
				"packets": models.JSONFloat(random(1000000, 1000000000)),
			},
		}
	}
	return units
}

// mockUsage will generate usage with randomised data
func mockUsage() []models.Usage {
	group := "group"
	// Set user project map
	var userProjectMap = make(map[string][]string)
	for _, user := range users {
		var userProjects []string
		for i := 0; i < int(random(1, 4)); i++ {
			userProjects = append(
				userProjects, allProjects[int(random(0, int64(len(allProjects))))],
			)
		}
		userProjectMap[user] = userProjects
	}

	// Generate usage
	var usage []models.Usage
	for user, prjs := range userProjectMap {
		for _, prj := range prjs {
			var resourceMgr string
			if strings.HasPrefix(prj, "acc") {
				resourceMgr = "slurm"
			} else if strings.HasPrefix(prj, "tenant") {
				resourceMgr = "openstack"
			} else {
				resourceMgr = "k8s"
			}
			usage = append(usage, models.Usage{
				ResourceManager: resourceMgr,
				Project:         prj,
				User:            user,
				Group:           group,
				NumUnits:        random(0, 1000),
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(random(0, 1e4)),
					"alloc_cputime":    models.JSONFloat(random(0, 1e5)),
					"alloc_gputime":    models.JSONFloat(random(0, 1e4)),
					"alloc_cpumemtime": models.JSONFloat(random(0, 1e8)),
					"alloc_gpumemtime": models.JSONFloat(random(0, 1e8)),
				},
				AveCPUUsage:         models.MetricMap{"usage": randomFloats(0, 100)},
				AveCPUMemUsage:      models.MetricMap{"usage": randomFloats(0, 100)},
				TotalCPUEnergyUsage: models.MetricMap{"usage": randomFloats(0, 5e3)},
				TotalCPUEmissions:   models.MetricMap{"rte": randomFloats(0, 50e3)},
				AveGPUUsage:         models.MetricMap{"usage": randomFloats(0, 100)},
				AveGPUMemUsage:      models.MetricMap{"usage": randomFloats(0, 100)},
				TotalGPUEnergyUsage: models.MetricMap{"usage": randomFloats(0, 50e3)},
				TotalGPUEmissions:   models.MetricMap{"rte": randomFloats(0, 500e3)},
				TotalIOWriteStats: models.MetricMap{
					"bytes":    models.JSONFloat(random(1000000, 1000000000)),
					"requests": models.JSONFloat(random(1000000, 1000000000)),
				},
				TotalIOReadStats: models.MetricMap{
					"bytes":    models.JSONFloat(random(1000000, 1000000000)),
					"requests": models.JSONFloat(random(1000000, 1000000000)),
				},
				TotalIngressStats: models.MetricMap{
					"bytes":   models.JSONFloat(random(1000000, 1000000000)),
					"packets": models.JSONFloat(random(1000000, 1000000000)),
				},
				TotalOutgressStats: models.MetricMap{
					"bytes":   models.JSONFloat(random(1000000, 1000000000)),
					"packets": models.JSONFloat(random(1000000, 1000000000)),
				},
			})
		}
	}
	return usage
}
