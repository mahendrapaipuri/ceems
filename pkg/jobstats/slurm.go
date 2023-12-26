package jobstats

import (
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
)

type slurmScheduler struct {
	logger          log.Logger
	execMode        string
	slurmDateFormat string
}

const slurmBatchScheduler = "slurm"

var (
	slurmUserUid int
	slurmUserGid int
	jobLock      = sync.RWMutex{}
	sacctPath    = BatchJobStatsServerApp.Flag(
		"slurm.sacct.path",
		"Absolute path to sacct executable.",
	).Default("/usr/local/bin/sacct").String()
	slurmWalltimeCutoff = BatchJobStatsServerApp.Flag(
		"slurm.elapsed.time.cutoff",
		"Jobs that have elapsed time less than this value (in seconds) will be ignored.",
	).Default("60").Int()
)

func init() {
	// Register batch scheduler
	RegisterBatch(slurmBatchScheduler, false, NewSlurmScheduler)
}

// Run basic checks like checking path of executable etc
func preflightChecks(logger log.Logger) (string, error) {
	// Assume execMode is always native
	execMode := "native"
	// Check if sacct binary exists
	if _, err := os.Stat(*sacctPath); err != nil {
		level.Error(logger).Log("msg", "Failed to open sacct executable", "path", *sacctPath, "err", err)
		return "", err
	}

	// If current user is slurm or root pass checks
	if currentUser, err := user.Current(); err == nil && (currentUser.Username == "slurm" || currentUser.Uid == "0") {
		level.Debug(logger).
			Log("msg", "Current user have enough privileges to get job data for all users", "user", currentUser.Username)
		return execMode, nil
	}

	// First try to run as slurm user in a subprocess. If current process have capabilities
	// it will be a success
	slurmUser, err := user.Lookup("slurm")
	if err != nil {
		level.Error(logger).Log("msg", "Failed to lookup slurm user for executing sacct cmd", "err", err)
		goto sudomode
	}

	slurmUserUid, err = strconv.Atoi(slurmUser.Uid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert slurm user uid to int", "uid", slurmUserUid, "err", err)
		goto sudomode
	}

	slurmUserGid, err = strconv.Atoi(slurmUser.Gid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert slurm user gid to int", "gid", slurmUserGid, "err", err)
		goto sudomode
	}

	if _, err := helpers.ExecuteAs(*sacctPath, []string{"--help"}, slurmUserUid, slurmUserGid, logger); err == nil {
		execMode = "cap"
		level.Debug(logger).Log("msg", "Linux capabilities will be used to execute sacct as slurm user")
		return execMode, nil
	}

sudomode:
	// Last attempt to run sacct with sudo
	if _, err := helpers.ExecuteWithTimeout("sudo", []string{*sacctPath, "--help"}, 5, logger); err == nil {
		execMode = "sudo"
		level.Debug(logger).Log("msg", "sudo will be used to execute sacct command")
		return execMode, nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	return execMode, nil
}

// NewSlurmScheduler returns a new SlurmScheduler that returns batch job stats
func NewSlurmScheduler(logger log.Logger) (Batch, error) {
	execMode, err := preflightChecks(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to setup Slurm batch scheduler for retreiving jobs", "err", err)
		return nil, err
	}
	level.Info(logger).Log("msg", "Jobs from slurm batch scheduler will be retrieved")
	return &slurmScheduler{
		logger:          logger,
		execMode:        execMode,
		slurmDateFormat: "2006-01-02T15:04:05",
	}, nil
}

// Get jobs from slurm
func (s *slurmScheduler) GetJobs(start time.Time, end time.Time) ([]BatchJob, error) {
	startTime := start.Format(s.slurmDateFormat)
	endTime := end.Format(s.slurmDateFormat)

	// Execute sacct command between start and end times
	sacctOutput, err := runSacctCmd(s.execMode, startTime, endTime, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to execute SLURM sacct command", "err", err)
		return []BatchJob{}, err
	}

	// Parse sacct output and create BatchJob structs slice
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), *slurmWalltimeCutoff, s.logger)
	level.Info(s.logger).Log("msg", "Retrieved Slurm jobs", "start", startTime, "end", endTime, "njobs", numJobs)
	return jobs, nil
}

// Run sacct command and return output
func runSacctCmd(execMode string, startTime string, endTime string, logger log.Logger) ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{"-D", "--allusers", "--parsable2",
		"--format", "jobidraw,partition,account,group,gid,user,uid,submit,start,end,elapsed,elapsedraw,exitcode,state,nnodes,nodelist,jobname,workdir",
		"--state", "CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
		"--starttime", startTime, "--endtime", endTime}

	// Run command as slurm user
	if execMode == "cap" {
		return helpers.ExecuteAs(*sacctPath, args, slurmUserUid, slurmUserGid, logger)
	} else if execMode == "sudo" {
		args = append([]string{*sacctPath}, args...)
		return helpers.Execute("sudo", args, logger)
	}
	return helpers.Execute(*sacctPath, args, logger)
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string, elapsedCutoff int, logger log.Logger) ([]BatchJob, int) {
	// Strip first line
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")[1:]

	var numJobs int = 0
	var jobs = make([]BatchJob, len(sacctOutputLines))

	wg := &sync.WaitGroup{}
	wg.Add(len(sacctOutputLines))

	for iline, line := range sacctOutputLines {
		go func(i int, l string) {
			var jobStat BatchJob
			components := strings.Split(l, "|")
			jobid := components[0]

			// Ignore if we cannot get all components
			if len(components) < 18 {
				wg.Done()
				return
			}

			// Ignore job steps
			if strings.Contains(jobid, ".") {
				wg.Done()
				return
			}

			// Ignore jobs that never ran
			if components[15] == "None assigned" {
				wg.Done()
				return
			}

			// Ignore jobs that ran for less than slurmWalltimeCutoff seconds
			if elapsedTime, err := strconv.Atoi(components[11]); err == nil && elapsedTime < elapsedCutoff {
				wg.Done()
				return
			}

			// Generate UUID from jobID, uid, account, nodelist(lowercase)
			jobUuid, err := helpers.GetUuidFromString(
				[]string{
					strings.TrimSpace(components[0]),
					strings.TrimSpace(components[6]),
					strings.ToLower(strings.TrimSpace(components[2])),
					strings.ToLower(strings.TrimSpace(components[15])),
				},
			)
			if err != nil {
				level.Error(logger).
					Log("msg", "Failed to generate UUID for job", "jobid", jobid, "err", err)
				jobUuid = jobid
			}

			allNodes := NodelistParser(components[15])
			nodelistExp := strings.Join(allNodes, "|")
			jobStat = BatchJob{
				Jobid:       components[0],
				Jobuuid:     jobUuid,
				Partition:   components[1],
				Account:     components[2],
				Grp:         components[3],
				Gid:         components[4],
				Usr:         components[5],
				Uid:         components[6],
				Submit:      components[7],
				Start:       components[8],
				End:         components[9],
				Elapsed:     components[10],
				Exitcode:    components[12],
				State:       components[13],
				Nnodes:      components[14],
				Nodelist:    components[15],
				NodelistExp: nodelistExp,
			}
			jobLock.Lock()
			jobs[i] = jobStat
			numJobs += 1
			jobLock.Unlock()
			wg.Done()
		}(iline, line)
	}
	wg.Wait()
	return jobs, numJobs
}
