package resource

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
	internal_osexec "github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

type slurmScheduler struct {
	logger   log.Logger
	execMode string
}

const slurmBatchScheduler = "slurm"

var (
	slurmUserUID    int
	slurmUserGID    int
	slurmTimeFormat = fmt.Sprintf("%s-0700", base.DatetimeLayout)
	jobLock         = sync.RWMutex{}
	sacctPath       = base.CEEMSServerApp.Flag(
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
	RegisterManager(slurmBatchScheduler, NewSlurmScheduler)

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

	slurmUserUID, err = strconv.Atoi(slurmUser.Uid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert slurm user uid to int", "uid", slurmUserUID, "err", err)
		goto sudomode
	}

	slurmUserGID, err = strconv.Atoi(slurmUser.Gid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert slurm user gid to int", "gid", slurmUserGID, "err", err)
		goto sudomode
	}

	if _, err := internal_osexec.ExecuteAs(*sacctPath, []string{"--help"}, slurmUserUID, slurmUserGID, nil, logger); err == nil {
		execMode = "cap"
		level.Debug(logger).Log("msg", "Linux capabilities will be used to execute sacct as slurm user")
		return execMode, nil
	}

sudomode:
	// Last attempt to run sacct with sudo
	if _, err := internal_osexec.ExecuteWithTimeout("sudo", []string{*sacctPath, "--help"}, 5, nil, logger); err == nil {
		execMode = "sudo"
		level.Debug(logger).Log("msg", "sudo will be used to execute sacct command")
		return execMode, nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	return execMode, nil
}

// NewSlurmScheduler returns a new SlurmScheduler that returns batch job stats
func NewSlurmScheduler(logger log.Logger) (Fetcher, error) {
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
func (s *slurmScheduler) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	startTime := start.Format(base.DatetimeLayout)
	endTime := end.Format(base.DatetimeLayout)

	// Execute sacct command between start and end times
	sacctOutput, err := runSacctCmd(s.execMode, startTime, endTime, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to execute SLURM sacct command", "err", err)
		return []models.Unit{}, err
	}

	// Parse sacct output and create BatchJob structs slice
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput))
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
		return internal_osexec.ExecuteAs(*sacctPath, args, slurmUserUID, slurmUserGID, env, logger)
	} else if execMode == "sudo" {
		args = append([]string{*sacctPath}, args...)
		return internal_osexec.Execute("sudo", args, env, logger)
	}
	return internal_osexec.Execute(*sacctPath, args, env, logger)
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string) ([]models.Unit, int) {
	// Strip first line
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")[1:]

	var numJobs = 0
	var jobs = make([]models.Unit, len(sacctOutputLines))

	wg := &sync.WaitGroup{}
	wg.Add(len(sacctOutputLines))

	for iline, line := range sacctOutputLines {
		go func(i int, l string) {
			var jobStat models.Unit
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
			var gidInt, uidInt, elapsedrawInt int64
			gidInt, _ = strconv.ParseInt(components[sacctFieldMap["gid"]], 10, 64)
			uidInt, _ = strconv.ParseInt(components[sacctFieldMap["uid"]], 10, 64)
			elapsedrawInt, _ = strconv.ParseInt(components[sacctFieldMap["elapsedraw"]], 10, 64)

			// Parse alloctres to get billing, nnodes, ncpus, ngpus and mem
			var billing, nnodes, ncpus, ngpus int64
			var mem string
			for _, elem := range strings.Split(components[sacctFieldMap["alloctres"]], ",") {
				var tresKV = strings.Split(elem, "=")
				if tresKV[0] == "billing" {
					billing, _ = strconv.ParseInt(tresKV[1], 10, 64)
				}
				if tresKV[0] == "node" {
					nnodes, _ = strconv.ParseInt(tresKV[1], 10, 64)
				}
				if tresKV[0] == "cpu" {
					ncpus, _ = strconv.ParseInt(tresKV[1], 10, 64)
				}
				if tresKV[0] == "gres/gpu" {
					ngpus, _ = strconv.ParseInt(tresKV[1], 10, 64)
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

			// Expand nodelist range expressions
			allNodes := helper.NodelistParser(components[sacctFieldMap["nodelist"]])
			nodelistExp := strings.Join(allNodes, "|")

			// Allocation
			allocation := models.Allocation{
				"nodes": nnodes,
				"cpus":  ncpus,
				"mem":   mem,
				"gpus":  ngpus,
			}

			// Tags
			tags := models.Tag{
				"uid":         uidInt,
				"gid":         gidInt,
				"partition":   components[sacctFieldMap["partition"]],
				"qos":         components[sacctFieldMap["qos"]],
				"nodelist":    components[sacctFieldMap["nodelist"]],
				"nodelistexp": nodelistExp,
				"workdir":     components[sacctFieldMap["workdir"]],
			}

			// Make jobStats struct for each job and put it in jobs slice
			jobStat = models.Unit{
				UUID:            jobid,
				Name:            components[sacctFieldMap["jobname"]],
				Project:         components[sacctFieldMap["account"]],
				Grp:             components[sacctFieldMap["group"]],
				Usr:             components[sacctFieldMap["user"]],
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
				Allocation:      allocation,
				TotalCPUBilling: cpuBilling,
				TotalGPUBilling: gpuBilling,
				Tags:            tags,
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
