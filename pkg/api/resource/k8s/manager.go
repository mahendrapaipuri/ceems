// Package k8s implements the fetcher interface to fetch pods from k8s
// resource manager
package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ceems-dev/ceems/internal/common"
	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/cli"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/api/resource"
	"github.com/ceems-dev/ceems/pkg/k8s"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	k8s_resource "k8s.io/apimachinery/pkg/api/resource"
)

const (
	notAvailable = "N/A"
)

// Default config values.
const defaultNSUsersListFile = "/var/run/ceems/users.yaml"

var (
	defaultUserAnnotations  = []string{"ceems.io/created-by"}
	defaultGPUResourceNames = []string{
		"nvidia.com/gpu",
		"amd.com/gpu",
	}
)

// k8sManager is the struct containing the configuration of a given k8s cluster.
type k8sManager struct {
	logger     *slog.Logger
	cluster    models.Cluster
	client     *k8s.Client
	config     *k8sConfig
	nsUsersMap map[string][]string
}

type k8sConfig struct {
	KubeConfigFile      string   `yaml:"kubeconfig_file"`
	NSUsersListFile     string   `yaml:"ns_users_list_file"`
	GPUResourceNames    []string `yaml:"gpu_resource_names"`
	UsernameAnnotations []string `yaml:"username_annotations"`
	ProjectAnnotations  []string `yaml:"project_annotations"`
}

// defaults set struct fields to default values.
func (c *k8sConfig) defaults() *k8sConfig {
	// Check if config is empty
	if c == nil {
		return &k8sConfig{
			NSUsersListFile:     defaultNSUsersListFile,
			UsernameAnnotations: defaultUserAnnotations,
			GPUResourceNames:    defaultGPUResourceNames,
		}
	} else {
		// When config is not nil, check for vital fields
		if len(c.UsernameAnnotations) == 0 {
			c.UsernameAnnotations = defaultUserAnnotations
		}

		if len(c.GPUResourceNames) == 0 {
			c.GPUResourceNames = defaultGPUResourceNames
		}

		if c.NSUsersListFile == "" {
			c.NSUsersListFile = defaultNSUsersListFile
		}

		return c
	}
}

const k8sPodManager = "k8s"

func init() {
	// Register k8s manager
	resource.Register(k8sPodManager, New)
}

// New returns a new openstackManager that returns compute instances.
func New(cluster models.Cluster, logger *slog.Logger) (resource.Fetcher, error) {
	// Fetch any provided from extra_config
	var c k8sConfig
	if err := cluster.Extra.Decode(&c); err != nil {
		logger.Error("Failed to decode extra_config for k8s cluster", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Set defaults
	config := c.defaults()

	// Make k8s client
	client, err := k8s.New(config.KubeConfigFile, "", logger)
	if err != nil {
		logger.Error("Failed to create k8s client", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Get update interval from main config file
	mainConfig, err := common.MakeConfig[cli.CEEMSAPIAppConfig](base.ConfigFilePath, base.ConfigFileExpandEnvVars)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	// Set directory for reading files
	mainConfig.SetDirectory(filepath.Dir(base.ConfigFilePath))

	// Create a new pod informer
	if err := client.NewPodInformer(time.Duration(mainConfig.Server.Data.UpdateInterval)); err != nil {
		logger.Error("Failed to create k8s pod informer", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Start pod informer
	if err := client.StartInformer(); err != nil {
		logger.Error("Failed to start k8s pod informer", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Make k8sManager configs from clusters
	k8sManager := &k8sManager{
		logger:     logger,
		cluster:    cluster,
		client:     client,
		config:     config,
		nsUsersMap: make(map[string][]string),
	}

	logger.Info("Pods from k8s cluster will be fetched", "id", cluster.ID)

	return k8sManager, nil
}

// FetchUnits fetches pods from k8s.
func (k *k8sManager) FetchUnits(
	_ context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
	// Fetch pods from client
	units := k.fetchPods(start, end)

	return []models.ClusterUnits{{Cluster: k.cluster, Units: units}}, nil
}

// FetchUsersProjects fetches current k8s users and namespaces.
func (k *k8sManager) FetchUsersProjects(
	ctx context.Context,
	currentTime time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	// Fetch users and namespaces association
	userModels, projectModels := k.fetchUserNSs(ctx, currentTime)

	return []models.ClusterUsers{
			{Cluster: k.cluster, Users: userModels},
		}, []models.ClusterProjects{
			{Cluster: k.cluster, Projects: projectModels},
		}, nil
}

func (k *k8sManager) fetchPods(
	start time.Time,
	end time.Time,
) []models.Unit {
	// Fetch all pods
	pods := k.client.Pods()

	// Get current time location
	loc := end.Location()

	// Transform pods into units
	units := make([]models.Unit, len(pods))

	for ipod, pod := range pods {
		// Convert CreatedAt to current time location
		createdAt := pod.CreationTimestamp.In(loc)

		// Always get elapsed time based on createdAt which gives
		// a more stable value
		elapsedTime := common.Timespan(end.Sub(createdAt)).Format("15:04:05")

		// Initialise vars
		var startedAt, endedAt time.Time

		var activeTimeSeconds float64

		var status string

		// Check Pod phase to determine if pod is running or not
		switch pod.Status.Phase {
		// Pod can be in Running state and still have errored containers
		// The typical CrashBackLoopOff in kubectl output is a result of
		// pod in Running phase but containers cannot be started.
		// So, we have to check the pod phases to get "real" status of the
		// pod.
		// We use PodReadyToStartContainers as proxy for start time as it
		// indicates that the k8s has done everything to start containers
		// which launches apps. If anything is wrong with app, containers fail
		// to start and based on restart polciy pod will keep trying to start
		// failed containers. This should be regarded as pod "running" as containers
		// are being launched (even if they fail) and thus consuming resources.
		case v1.PodRunning:
			// Get pod start time based on status conditions
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReadyToStartContainers {
					if cond.Status == v1.ConditionTrue {
						startedAt = cond.LastTransitionTime.In(loc)
						status = fmt.Sprintf("%s/%s", v1.PodRunning, v1.PodReady)

						// If the pod has started in this update interval
						// update activeTime from start till now
						if startedAt.After(start) {
							activeTimeSeconds = startedAt.Sub(start).Seconds()
						} else {
							activeTimeSeconds = end.Sub(start).Seconds()
						}
					} else {
						status = fmt.Sprintf("%s/%s", v1.PodRunning, cond.Reason)
					}

					// Break loop once the status is updated
					break
				}
			}
		// Both these status indicate pod has been terminated with or without
		// exit code 1. This means we should be able to get deletion time.
		case v1.PodSucceeded, v1.PodFailed:
			// Get pod start time based on status conditions
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReadyToStartContainers && cond.Status == v1.ConditionTrue {
					startedAt = cond.LastTransitionTime.In(loc)
				}
			}

			// Get pod deletion time
			if pod.DeletionTimestamp != nil {
				endedAt = pod.DeletionTimestamp.In(loc)

				// Override elapsed time for deleted pods
				elapsedTime = common.Timespan(endedAt.Sub(createdAt)).Format("15:04:05")

				// If the pod has been terminated in this update interval
				// update activeTime from start till termination
				if endedAt.Before(end) && endedAt.After(start) {
					activeTimeSeconds = endedAt.Sub(start).Seconds()
				}
			}

			// Set status
			status = string(pod.Status.Phase)
		case v1.PodPending:
			status = string(v1.PodPending)
			elapsedTime = notAvailable
		case v1.PodUnknown:
			status = string(v1.PodUnknown)
			elapsedTime = notAvailable
		default:
			status = "Unknown"
			elapsedTime = notAvailable
		}

		// Check if startedAt and endedAt are valid
		var startedAtString, endedAtString string

		var startedAtTS, endedAtTS int64

		if startedAt.IsZero() {
			startedAtString = notAvailable
		} else {
			startedAtString = startedAt.Format(base.DatetimezoneLayout)
			startedAtTS = startedAt.UnixMilli()
		}

		if endedAt.IsZero() {
			endedAtString = notAvailable
		} else {
			endedAtString = endedAt.Format(base.DatetimezoneLayout)
			endedAtTS = endedAt.UnixMilli()
		}

		// Use namespace as fallback for project
		project := pod.Namespace

		for key, value := range pod.GetObjectMeta().GetAnnotations() {
			if slices.Contains(k.config.ProjectAnnotations, key) {
				project = value

				break
			}
		}

		// Use project:unknown as fallback username
		username := fmt.Sprintf("%s:%s", project, base.UnknownUser)

		for key, value := range pod.GetObjectMeta().GetAnnotations() {
			if slices.Contains(k.config.UsernameAnnotations, key) {
				if !strings.Contains(value, "serviceaccount") {
					username = value
				}

				break
			}
		}

		// Add username and project association to map
		if !slices.Contains(k.nsUsersMap[project], username) {
			k.nsUsersMap[project] = append(k.nsUsersMap[project], username)
		}

		// Get resources
		// PodResources have been added only in 1.32 and it is still in alpha as of 20250706.
		// If pod.Spec.Resources is nil, check limits on each container and sum them up.
		var cpus, cpuMem float64
		if pod.Spec.Resources != nil {
			cpus = pod.Spec.Resources.Limits.Cpu().AsApproximateFloat64()
			cpuMem = pod.Spec.Resources.Limits.Memory().AsApproximateFloat64()
		} else {
			for _, cont := range pod.Spec.Containers {
				cpus += cont.Resources.Limits.Cpu().AsApproximateFloat64()
				cpuMem += cont.Resources.Limits.Memory().AsApproximateFloat64()
			}
		}

		// Ensure cpus and cpuMem is non zero. When no resources are set on pod
		// use a milliCPU as cpu resource and 1 byte as cpu memory
		cpus = math.Max(cpus, 0.001)
		cpuMem = math.Max(cpuMem, 1)

		// Allocation
		allocation := models.Allocation{
			"vcpus": cpus,
			"mem":   cpuMem,
		}

		// Get GPU resources
		// The downside is that we are treating all types of GPUs as same
		// and ideally we should have weighting factor for each GPU type
		// so that we can "accurately" estimate GPU allocation time.
		// Something to think for the future!!
		var gpus float64

		for _, name := range k.config.GPUResourceNames {
			for _, cont := range pod.Spec.Containers {
				if n := cont.Resources.Limits.Name(v1.ResourceName(name), k8s_resource.DecimalSI).AsApproximateFloat64(); n > 0 {
					allocation[name] = n
					gpus += n
				}
			}
		}

		// Total time
		totalTime := models.MetricMap{
			"walltime":         models.JSONFloat(activeTimeSeconds),
			"alloc_cputime":    models.JSONFloat(cpus * activeTimeSeconds),
			"alloc_cpumemtime": models.JSONFloat(cpuMem * activeTimeSeconds),
			"alloc_gputime":    models.JSONFloat(gpus * activeTimeSeconds),
			"alloc_gpumemtime": models.JSONFloat(gpus * activeTimeSeconds),
		}

		// Tags
		tags := models.Tag{
			"qos": string(pod.Status.QOSClass),
		}

		if len(pod.Annotations) > 0 {
			tags["annotations"] = pod.Annotations
		}

		if len(pod.Labels) > 0 {
			tags["labels"] = pod.Labels
		}

		units[ipod] = models.Unit{
			ClusterID:       k.cluster.ID,
			ResourceManager: k8sPodManager,
			UUID:            string(pod.UID),
			Name:            pod.Name,
			User:            username,
			Project:         project,
			CreatedAt:       createdAt.Format(base.DatetimezoneLayout),
			CreatedAtTS:     createdAt.UnixMilli(),
			StartedAt:       startedAtString,
			StartedAtTS:     startedAtTS,
			EndedAt:         endedAtString,
			EndedAtTS:       endedAtTS,
			State:           status,
			Elapsed:         elapsedTime,
			TotalTime:       totalTime,
			Allocation:      allocation,
			Tags:            tags,
		}
	}

	k.logger.Info("k8s pods fetched", "cluster_id", k.cluster.ID, "start", start, "end", end, "num_pods", len(units))

	return units
}

func (k *k8sManager) fetchUserNSs(ctx context.Context, current time.Time) ([]models.User, []models.Project) {
	// Initialise maps
	usersNSs := make(map[string][]string)

	nsUsers := make(map[string][]string)

	// Current time string
	currentTime := current.Format(base.DatetimezoneLayout)

	// Check if the configmap is available to fetch users
	if content, err := os.ReadFile(k.config.NSUsersListFile); err == nil {
		var usersDB struct {
			NSUsers map[string][]string `yaml:"users"`
		}

		if err := yaml.Unmarshal(content, &usersDB); err == nil {
			nsUsers = usersDB.NSUsers

			for ns, users := range usersDB.NSUsers {
				for _, user := range users {
					usersNSs[user] = append(usersNSs[user], ns)
				}
			}
		}
	}

	// Merge users and namespaces from RBAC
	if rbacUsers, err := k.client.ListUsers(ctx, ""); err == nil {
		for ns, users := range rbacUsers {
			for _, user := range users {
				usersNSs[user] = append(usersNSs[user], ns)
				nsUsers[ns] = append(nsUsers[ns], user)
			}
		}
	}

	// Finally merge users and namespaces from fetched pods
	for ns, users := range k.nsUsersMap {
		for _, user := range users {
			usersNSs[user] = append(usersNSs[user], ns)
			nsUsers[ns] = append(nsUsers[ns], user)
		}
	}

	// Remove duplicates and make user and project models
	var userModels []models.User

	var projectModels []models.Project

	for ns, users := range nsUsers {
		slices.Sort(users)
		p := models.Project{
			Name:            ns,
			ClusterID:       k.cluster.ID,
			ResourceManager: k.cluster.Manager,
			Users:           models.List{slices.Compact(users)},
			LastUpdatedAt:   currentTime,
		}

		projectModels = append(projectModels, p)
	}

	for user, nss := range usersNSs {
		slices.Sort(nss)
		u := models.User{
			Name:            user,
			ClusterID:       k.cluster.ID,
			ResourceManager: k.cluster.Manager,
			Projects:        models.List{slices.Compact(nss)},
			LastUpdatedAt:   currentTime,
		}

		userModels = append(userModels, u)
	}

	// If we have not found any user data emit a warning
	if len(userModels) == 0 && len(projectModels) == 0 {
		k.logger.Warn("No users and namespaces associations found", "id", k.cluster.ID)
	}

	k.logger.Info("k8s user data fetched", "cluster_id", k.cluster.ID, "num_users", len(userModels), "num_projects", len(projectModels))

	return userModels, projectModels
}
