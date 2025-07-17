// Package slurm implements the fetcher interface to fetch compute units from SLURM
// resource manager
package slurm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ceems-dev/ceems/internal/security"
	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/api/resource"
)

// Execution modes.
const (
	sudoMode       = "sudo"
	capabilityMode = "cap"
)

// Fetch modes.
const (
	cliMode = "cli"
)

// Security contexts.
const (
	slurmExecCmdCtx = "slurm_exec_cmd"
)

// slurmScheduler is the struct containing the configuration of a given slurm cluster.
type slurmScheduler struct {
	logger           *slog.Logger
	cluster          models.Cluster
	fetchMode        string // Whether to fetch from REST API or CLI commands
	cmdExecMode      string // If sacct mode is chosen, the mode of executing command, ie, sudo or cap or native
	securityContexts map[string]*security.SecurityContext
}

const slurmBatchScheduler = "slurm"

var (
	jobLock     = sync.RWMutex{}
	assocLock   = sync.RWMutex{}
	sacctFields = []string{
		"jobidraw", "partition", "qos", "account", "group", "gid", "user", "uid",
		"submit", "start", "end", "elapsed", "elapsedraw", "exitcode", "state",
		"alloctres", "nodelist", "jobname", "workdir",
	}
	slurmStates = []string{
		"CANCELLED", "COMPLETED", "FAILED", "NODE_FAIL", "PREEMPTED", "TIMEOUT",
		"RUNNING",
	}
	sacctFieldMap = make(map[string]int, len(sacctFields))
)

func init() {
	// Register batch scheduler
	resource.Register(slurmBatchScheduler, New)

	// Convert slice to map with index as value
	for idx, field := range sacctFields {
		sacctFieldMap[field] = idx
	}
}

// New returns a new SlurmScheduler that returns batch job stats.
func New(cluster models.Cluster, logger *slog.Logger) (resource.Fetcher, error) {
	// Make slurmCluster configs from clusters
	slurmScheduler := slurmScheduler{
		logger:           logger,
		cluster:          cluster,
		securityContexts: make(map[string]*security.SecurityContext),
	}

	if err := preflightChecks(&slurmScheduler); err != nil {
		return nil, err
	}

	logger.Info("Batch jobs from SLURM cluster will be fetched", "id", cluster.ID)

	return &slurmScheduler, nil
}

// FetchUnits fetches jobs from slurm.
func (s *slurmScheduler) FetchUnits(
	ctx context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
	// Fetch each cluster one by one to reduce memory footprint
	var jobs []models.Unit

	var err error
	if s.fetchMode == cliMode {
		if jobs, err = s.fetchFromSacct(ctx, start, end); err != nil {
			s.logger.Error("Failed to execute SLURM sacct command", "cluster_id", s.cluster.ID, "err", err)

			return nil, err
		}

		return []models.ClusterUnits{{Cluster: s.cluster, Units: jobs}}, nil
	}

	return nil, fmt.Errorf("unknown fetch mode for compute units SLURM cluster %s", s.cluster.ID)
}

// FetchUsersProjects fetches current SLURM users and accounts.
func (s *slurmScheduler) FetchUsersProjects(
	ctx context.Context,
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	// Fetch each cluster one by one to reduce memory footprint
	var users []models.User

	var projects []models.Project

	var err error
	if s.fetchMode == cliMode {
		if users, projects, err = s.fetchFromSacctMgr(ctx, current); err != nil {
			s.logger.Error("Failed to execute SLURM sacctmgr command", "cluster_id", s.cluster.ID, "err", err)

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

// Get jobs from slurm sacct command.
func (s *slurmScheduler) fetchFromSacct(ctx context.Context, start time.Time, end time.Time) ([]models.Unit, error) {
	// Execute sacct command between start and end times
	sacctOutput, err := s.runSacctCmd(ctx, start, end)
	if err != nil {
		s.logger.Error("Failed to run sacct command", "cluster_id", s.cluster.ID, "err", err)

		return []models.Unit{}, err
	}

	// Parse sacct output and create BatchJob structs slice
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), start, end)
	s.logger.Info("SLURM jobs fetched", "cluster_id", s.cluster.ID, "start", start, "end", end, "num_jobs", numJobs)

	return jobs, nil
}

// Get user project association from slurm sacctmgr command.
func (s *slurmScheduler) fetchFromSacctMgr(
	ctx context.Context,
	current time.Time,
) ([]models.User, []models.Project, error) {
	// Get current time string
	currentTime := current.Format(base.DatetimeLayout)

	// Execute sacctmgr command
	sacctMgrOutput, err := s.runSacctMgrCmd(ctx)
	if err != nil {
		s.logger.Error("Failed to run sacctmgr command", "cluster_id", s.cluster.ID, "err", err)

		return nil, nil, err
	}

	// Parse sacctmgr output to get user project associations
	users, projects := parseSacctMgrCmdOutput(string(sacctMgrOutput), currentTime)
	s.logger.Info("SLURM user account data fetched", "cluster_id", s.cluster.ID, "num_users", len(users), "num_accounts", len(projects))

	return users, projects, nil
}
