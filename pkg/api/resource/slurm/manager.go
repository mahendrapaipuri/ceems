// Package slurm implements the fetcher interface to fetch compute units from SLURM
// resource manager
package slurm

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	internal_osexec "github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
)

// slurmScheduler is the struct containing the configuration of a given slurm cluster
type slurmScheduler struct {
	logger      log.Logger
	cluster     models.Cluster
	fetchMode   string // Whether to fetch from REST API or CLI commands
	cmdExecMode string // If sacct mode is chosen, the mode of executing command, ie, sudo or cap or native
}

const slurmBatchScheduler = "slurm"

var (
	slurmUserUID    int
	slurmUserGID    int
	slurmTimeFormat = fmt.Sprintf("%s-0700", base.DatetimeLayout)
	jobLock         = sync.RWMutex{}
	assocLock       = sync.RWMutex{}
	sacctFields     = []string{
		"jobidraw", "partition", "qos", "account", "group", "gid", "user", "uid",
		"submit", "start", "end", "elapsed", "elapsedraw", "exitcode", "state",
		"alloctres", "nodelist", "jobname", "workdir",
	}
	sacctFieldMap = make(map[string]int, len(sacctFields))
)

func init() {
	// Register batch scheduler
	resource.RegisterManager(slurmBatchScheduler, NewSlurmScheduler)

	// Convert slice to map with index as value
	for idx, field := range sacctFields {
		sacctFieldMap[field] = idx
	}
}

// NewSlurmScheduler returns a new SlurmScheduler that returns batch job stats
func NewSlurmScheduler(cluster models.Cluster, logger log.Logger) (resource.Fetcher, error) {
	// Make slurmCluster configs from clusters
	var slurmScheduler = slurmScheduler{logger: logger, cluster: cluster}
	if err := preflightChecks(&slurmScheduler); err != nil {
		return nil, err
	}
	level.Info(logger).Log("msg", "Fetching batch jobs from SLURM clusters", "id", cluster.ID)
	return &slurmScheduler, nil
}

// FetchUnits fetches jobs from slurm
func (s *slurmScheduler) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	// Fetch each cluster one by one to reduce memory footprint
	var jobs []models.Unit
	var err error
	if s.fetchMode == "cli" {
		if jobs, err = s.fetchFromSacct(start, end); err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to execute SLURM sacct command", "cluster_id", s.cluster.ID, "err", err)
			return nil, err
		}
		return []models.ClusterUnits{{Cluster: s.cluster, Units: jobs}}, nil
	}
	return nil, fmt.Errorf("unknown fetch mode for compute units SLURM cluster %s", s.cluster.ID)
}

// FetchUsersProjects fetches current SLURM users and accounts
func (s *slurmScheduler) FetchUsersProjects(
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	// Fetch each cluster one by one to reduce memory footprint
	var users []models.User
	var projects []models.Project
	var err error
	if s.fetchMode == "cli" {
		if users, projects, err = s.fetchFromSacctMgr(current); err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to execute SLURM sacctmgr command", "cluster_id", s.cluster.ID, "err", err)
			return nil, nil, err
		}
		return []models.ClusterUsers{
				{Cluster: s.cluster, Users: users},
			}, []models.ClusterProjects{
				{Cluster: s.cluster, Projects: projects},
			}, nil
	}
	return nil, nil, fmt.Errorf("unknown fetch mode for projects for SLURM cluster %s", s.cluster.ID)
}

// Get jobs from slurm sacct command
func (s *slurmScheduler) fetchFromSacct(start time.Time, end time.Time) ([]models.Unit, error) {
	startTime := start.Format(base.DatetimeLayout)
	endTime := end.Format(base.DatetimeLayout)

	// Execute sacct command between start and end times
	sacctOutput, err := s.runSacctCmd(startTime, endTime)
	if err != nil {
		return []models.Unit{}, err
	}

	// Parse sacct output and create BatchJob structs slice
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), start, end)
	level.Info(s.logger).
		Log("msg", "SLURM jobs fetched", "cluster_id", s.cluster.ID, "start", startTime, "end", endTime, "njobs", numJobs)
	return jobs, nil
}

// Get user project association from slurm sacctmgr command
func (s *slurmScheduler) fetchFromSacctMgr(current time.Time) ([]models.User, []models.Project, error) {
	// Get current time string
	currentTime := current.Format(base.DatetimeLayout)

	// Execute sacctmgr command
	sacctMgrOutput, err := s.runSacctMgrCmd()
	if err != nil {
		return nil, nil, err
	}

	// Parse sacctmgr output to get user project associations
	users, projects := parseSacctMgrCmdOutput(string(sacctMgrOutput), currentTime)
	level.Info(s.logger).
		Log("msg", "SLURM user account data fetched",
			"cluster_id", s.cluster.ID, "num_users", len(users), "num_accounts", len(projects))
	return users, projects, nil
}

// Run sacct command and return output
func (s *slurmScheduler) runSacctCmd(startTime string, endTime string) ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{
		"-D", "-X", "--noheader", "--allusers", "--parsable2",
		"--format", strings.Join(sacctFields, ","),
		"--state", "RUNNING,CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
		"--starttime", startTime,
		"--endtime", endTime,
	}

	// sacct path
	sacctPath := filepath.Join(s.cluster.CLI.Path, "sacct")

	// Use SLURM_TIME_FORMAT env var to get timezone offset
	env := []string{"SLURM_TIME_FORMAT=%Y-%m-%dT%H:%M:%S%z"}
	for name, value := range s.cluster.CLI.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", name, value))
	}

	// Run command as slurm user
	if s.cmdExecMode == "cap" {
		return internal_osexec.ExecuteAs(sacctPath, args, slurmUserUID, slurmUserGID, env, s.logger)
	} else if s.cmdExecMode == "sudo" {
		// Important that we need to export env as well as we set environment variables in the
		// command execution
		args = append([]string{"-E", sacctPath}, args...)
		return internal_osexec.Execute("sudo", args, env, s.logger)
	}
	return internal_osexec.Execute(sacctPath, args, env, s.logger)
}

// Run sacctmgr command and return output
func (s *slurmScheduler) runSacctMgrCmd() ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{"--parsable2", "--noheader", "list", "associations", "format=Account,User"}

	// sacctmgr path
	sacctMgrPath := filepath.Join(s.cluster.CLI.Path, "sacctmgr")

	// Set configured env vars
	var env = []string{}
	for name, value := range s.cluster.CLI.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", name, value))
	}

	// Run command as slurm user
	if s.cmdExecMode == "cap" {
		return internal_osexec.ExecuteAs(sacctMgrPath, args, slurmUserUID, slurmUserGID, env, s.logger)
	} else if s.cmdExecMode == "sudo" {
		// Important that we need to export env as well as we set environment variables in the
		// command execution
		args = append([]string{"-E", sacctMgrPath}, args...)
		return internal_osexec.Execute("sudo", args, env, s.logger)
	}
	return internal_osexec.Execute(sacctMgrPath, args, env, s.logger)
}

// Run preflight checks on provided config
func preflightChecks(s *slurmScheduler) error {
	// // Always prefer REST API mode if configured
	// if clusterConfig.Web.URL != "" {
	// 	return checkRESTAPI(clusterConfig, logger)
	// }

	return preflightsCLI(s)
}
