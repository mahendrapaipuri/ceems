package schedulers

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/internal/helpers"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/helper"
	jobstats_helper "github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/helper"
)

type slurmScheduler struct {
	logger   log.Logger
	execMode string
}

const slurmBatchScheduler = "slurm"

var (
	slurmUserUid    int
	slurmUserGid    int
	slurmTimeFormat = fmt.Sprintf("%s-0700", base.DatetimeLayout)
	jobLock         = sync.RWMutex{}
	sacctPath       = base.BatchJobStatsServerApp.Flag(
		"slurm.sacct.path",
		"Absolute path to sacct executable. If empty sacct on PATH will be used.",
	).Hidden().Default("").String()
	sacctFields = []string{
		"jobidraw", "partition", "qos", "account", "group", "gid", "user", "uid",
		"submit", "start", "end", "elapsed", "elapsedraw", "exitcode", "state",
		"alloctres", "nodelist", "jobname", "workdir",
	}
	sacctFieldMap = make(map[string]int, len(sacctFields))
)

func init() {
	// Register batch scheduler
	RegisterBatch(slurmBatchScheduler, NewSlurmScheduler)

	// Convert slice to map with index as value
	for idx, field := range sacctFields {
		sacctFieldMap[field] = idx
	}
}

// Run basic checks like checking path of executable etc
func preflightChecks(logger log.Logger) (string, error) {
	// Assume execMode is always native
	execMode := "native"

	// If no sacct path is provided, assume it is available on PATH
	if *sacctPath == "" {
		path, err := exec.LookPath("sacct")
		if err != nil {
			level.Error(logger).Log("msg", "Failed to find sacct executable on PATH", "err", err)
			return "", err
		}
		*sacctPath = path
	} else {
		// Check if sacct binary exists at the given path
		if _, err := os.Stat(*sacctPath); err != nil {
			level.Error(logger).Log("msg", "Failed to open sacct executable", "path", *sacctPath, "err", err)
			return "", err
		}
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

	if _, err := helpers.ExecuteAs(*sacctPath, []string{"--help"}, slurmUserUid, slurmUserGid, nil, logger); err == nil {
		execMode = "cap"
		level.Debug(logger).Log("msg", "Linux capabilities will be used to execute sacct as slurm user")
		return execMode, nil
	}

sudomode:
	// Last attempt to run sacct with sudo
	if _, err := helpers.ExecuteWithTimeout("sudo", []string{*sacctPath, "--help"}, 5, nil, logger); err == nil {
		execMode = "sudo"
		level.Debug(logger).Log("msg", "sudo will be used to execute sacct command")
		return execMode, nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	return execMode, nil
}

// NewSlurmScheduler returns a new SlurmScheduler that returns batch job stats
func NewSlurmScheduler(logger log.Logger) (BatchJobFetcher, error) {
	execMode, err := preflightChecks(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to setup Slurm batch scheduler for retreiving jobs", "err", err)
		return nil, err
	}

	level.Info(logger).Log("msg", "Jobs from slurm batch scheduler will be retrieved")
	return &slurmScheduler{
		logger:   logger,
		execMode: execMode,
	}, nil
}

// Get jobs from slurm
func (s *slurmScheduler) Fetch(start time.Time, end time.Time) ([]base.Job, error) {
	startTime := start.Format(base.DatetimeLayout)
	endTime := end.Format(base.DatetimeLayout)

	// Execute sacct command between start and end times
	sacctOutput, err := runSacctCmd(s.execMode, startTime, endTime, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to execute SLURM sacct command", "err", err)
		return []base.Job{}, err
	}

	// Parse sacct output and create BatchJob structs slice
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), s.logger)
	level.Info(s.logger).Log("msg", "Slurm jobs fetched", "start", startTime, "end", endTime, "njobs", numJobs)
	return jobs, nil
}

// Run sacct command and return output
func runSacctCmd(execMode string, startTime string, endTime string, logger log.Logger) ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{
		"-D", "-X", "--allusers", "--parsable2",
		"--format", strings.Join(sacctFields, ","),
		"--state", "CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
		"--starttime", startTime,
		"--endtime", endTime,
	}

	// Use SLURM_TIME_FORMAT env var to get timezone offset
	env := []string{"SLURM_TIME_FORMAT=%Y-%m-%dT%H:%M:%S%z"}

	// Run command as slurm user
	if execMode == "cap" {
		return helpers.ExecuteAs(*sacctPath, args, slurmUserUid, slurmUserGid, env, logger)
	} else if execMode == "sudo" {
		args = append([]string{*sacctPath}, args...)
		return helpers.Execute("sudo", args, env, logger)
	}
	return helpers.Execute(*sacctPath, args, env, logger)
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string, logger log.Logger) ([]base.Job, int) {
	// Strip first line
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")[1:]

	var numJobs int = 0
	var jobs = make([]base.Job, len(sacctOutputLines))

	wg := &sync.WaitGroup{}
	wg.Add(len(sacctOutputLines))

	for iline, line := range sacctOutputLines {
		go func(i int, l string) {
			var err error
			var jobStat base.Job
			components := strings.Split(l, "|")
			jobid := components[sacctFieldMap["jobidraw"]]

			// Ignore if we cannot get all components
			if len(components) < len(sacctFields) {
				wg.Done()
				return
			}

			// Ignore job steps
			if strings.Contains(jobid, ".") {
				wg.Done()
				return
			}

			// Ignore jobs that never ran
			if components[sacctFieldMap["nodelist"]] == "None assigned" {
				wg.Done()
				return
			}

			// Attempt to convert strings to int and ignore any errors in conversion
			var jobidInt, gidInt, uidInt, elapsedrawInt int64
			if jobidInt, err = strconv.ParseInt(jobid, 10, 64); err != nil {
				wg.Done()
				return
			}
			gidInt, _ = strconv.ParseInt(components[sacctFieldMap["gid"]], 10, 64)
			uidInt, _ = strconv.ParseInt(components[sacctFieldMap["uid"]], 10, 64)
			elapsedrawInt, _ = strconv.ParseInt(components[sacctFieldMap["elapsedraw"]], 10, 64)

			// Parse alloctres to get billing, nnodes, ncpus, ngpus and mem
			var billing int64
			var nnodes, ncpus, ngpus int
			var mem string
			for _, elem := range strings.Split(components[sacctFieldMap["alloctres"]], ",") {
				var tresKV = strings.Split(elem, "=")
				if tresKV[0] == "billing" {
					billing, _ = strconv.ParseInt(tresKV[1], 10, 64)
				}
				if tresKV[0] == "node" {
					nnodes, _ = strconv.Atoi(tresKV[1])
				}
				if tresKV[0] == "cpu" {
					ncpus, _ = strconv.Atoi(tresKV[1])
				}
				if tresKV[0] == "gres/gpu" {
					ngpus, _ = strconv.Atoi(tresKV[1])
				}
				if tresKV[0] == "mem" {
					mem = tresKV[1]
				}
			}

			// Attribute billing to CPUBilling if ngpus is 0 or attribute to GPUBilling
			var cpuBilling, gpuBilling int64
			if ngpus == 0 {
				cpuBilling = billing
			} else {
				gpuBilling = billing
			}

			// Generate UUID from jobID, user, account, nodelist(lowercase)
			jobUuid, err := helpers.GetUuidFromString(
				[]string{
					strings.TrimSpace(components[sacctFieldMap["jobidraw"]]),
					strings.TrimSpace(components[sacctFieldMap["user"]]),
					strings.ToLower(strings.TrimSpace(components[sacctFieldMap["account"]])),
					strings.ToLower(strings.TrimSpace(components[sacctFieldMap["nodelist"]])),
				},
			)
			if err != nil {
				level.Error(logger).
					Log("msg", "Failed to generate UUID for job", "jobid", jobid, "err", err)
				jobUuid = jobid
			}

			// Expand nodelist range expressions
			allNodes := jobstats_helper.NodelistParser(components[sacctFieldMap["nodelist"]])
			nodelistExp := strings.Join(allNodes, "|")

			// Make jobStats struct for each job and put it in jobs slice
			jobStat = base.Job{
				Jobid:           jobidInt,
				Jobuuid:         jobUuid,
				Partition:       components[sacctFieldMap["partition"]],
				QoS:             components[sacctFieldMap["qos"]],
				Account:         components[sacctFieldMap["account"]],
				Grp:             components[sacctFieldMap["group"]],
				Gid:             gidInt,
				Usr:             components[sacctFieldMap["user"]],
				Uid:             uidInt,
				Submit:          components[sacctFieldMap["submit"]],
				Start:           components[sacctFieldMap["start"]],
				End:             components[sacctFieldMap["end"]],
				SubmitTS:        helper.TimeToTimestamp(slurmTimeFormat, components[8]),
				StartTS:         helper.TimeToTimestamp(slurmTimeFormat, components[9]),
				EndTS:           helper.TimeToTimestamp(slurmTimeFormat, components[10]),
				Elapsed:         components[sacctFieldMap["elapsed"]],
				ElapsedRaw:      elapsedrawInt,
				Exitcode:        components[sacctFieldMap["exitcode"]],
				State:           components[sacctFieldMap["state"]],
				Nnodes:          nnodes,
				Ncpus:           ncpus,
				Mem:             mem,
				Ngpus:           ngpus,
				Nodelist:        components[sacctFieldMap["nodelist"]],
				NodelistExp:     nodelistExp,
				JobName:         components[sacctFieldMap["jobname"]],
				WorkDir:         components[sacctFieldMap["workdir"]],
				TotalCPUBilling: cpuBilling,
				TotalGPUBilling: gpuBilling,
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
