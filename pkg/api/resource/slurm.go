package resource

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
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

	// SLURM AllocTRES gives memory as 200M, 250G and we dont know if it gives without
	// units. So, regex will capture the number and unit (if exists) and we convert it
	// to bytes
	memRegex = regexp.MustCompile("([0-9]+)([K|M|G|T]?)")
	toBytes  = map[string]int64{
		"K": 1024,
		"M": 1024 * 1024,
		"G": 1024 * 1024 * 1024,
		"T": 1024 * 1024 * 1024 * 1024,
		"Z": 1024 * 1024 * 1024 * 1024 * 1024,
	}
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
		level.Error(logger).Log("msg", "Failed to lookup SLURM user for executing sacct cmd", "err", err)
		goto sudomode
	}

	slurmUserUID, err = strconv.Atoi(slurmUser.Uid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert SLURM user uid to int", "uid", slurmUserUID, "err", err)
		goto sudomode
	}

	slurmUserGID, err = strconv.Atoi(slurmUser.Gid)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to convert SLURM user gid to int", "gid", slurmUserGID, "err", err)
		goto sudomode
	}

	if _, err := internal_osexec.ExecuteAs(*sacctPath, []string{"--help"}, slurmUserUID, slurmUserGID, nil, logger); err == nil {
		execMode = "cap"
		level.Debug(logger).Log("msg", "Linux capabilities will be used to execute sacct as SLURM user")
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
		level.Error(logger).Log("msg", "Failed to setup SLURM batch scheduler for retreiving jobs", "err", err)
		return nil, err
	}

	level.Info(logger).Log("msg", "Fetching batch jobs from SLURM")
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
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), start, end)
	level.Info(s.logger).Log("msg", "Slurm jobs fetched", "start", startTime, "end", endTime, "njobs", numJobs)
	return jobs, nil
}

// Run sacct command and return output
func runSacctCmd(execMode string, startTime string, endTime string, logger log.Logger) ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{
		"-D", "-X", "--allusers", "--parsable2",
		"--format", strings.Join(sacctFields, ","),
		"--state", "RUNNING,CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
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
func parseSacctCmdOutput(sacctOutput string, start time.Time, end time.Time) ([]models.Unit, int) {
	// Strip first line
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")[1:]

	// Update period
	intStartTS := start.Local().UnixMilli()
	intEndTS := end.Local().UnixMilli()

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
			var gidInt, uidInt int64
			gidInt, _ = strconv.ParseInt(components[sacctFieldMap["gid"]], 10, 64)
			uidInt, _ = strconv.ParseInt(components[sacctFieldMap["uid"]], 10, 64)
			// elapsedSeconds, _ = strconv.ParseInt(components[sacctFieldMap["elapsedraw"]], 10, 64)

			// Get job submit, start and end times
			jobSubmitTS := helper.TimeToTimestamp(slurmTimeFormat, components[8])
			jobStartTS := helper.TimeToTimestamp(slurmTimeFormat, components[9])
			jobEndTS := helper.TimeToTimestamp(slurmTimeFormat, components[10])

			// Parse alloctres to get billing, nnodes, ncpus, ngpus and mem
			var billing, nnodes, ncpus, ngpus int64
			var memString string
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
				// For MIG devices, it can be gres/gpu:<MIG ID>
				// https://github.com/SchedMD/slurm/blob/db91ac3046b3b7b845cce4a99127db8c6f14a8e8/testsuite/expect/test39.19#L70
				// Use a regex gres\/gpu:([^=]+)=(\d+) for identifying number of instances
				// For the moment, use strings.HasPrefix to identify GPU
				if strings.HasPrefix(tresKV[0], "gres/gpu") {
					ngpus, _ = strconv.ParseInt(tresKV[1], 10, 64)
				}
				if tresKV[0] == "mem" {
					memString = tresKV[1]
				}
			}

			// If mem is not empty string, convert the units [K|M|G|T] into numeric bytes
			// The following logic covers the cases when memory is of form 200M, 250G
			// and also without unit eg 20000, 40000. When there is no unit we assume
			// it is already in bytes
			matches := memRegex.FindStringSubmatch(memString)
			var mem int64
			var err error
			if len(matches) >= 2 {
				if mem, err = strconv.ParseInt(matches[1], 10, 64); err == nil {
					if len(matches) == 3 {
						if unitConv, ok := toBytes[matches[2]]; ok {
							mem = mem * unitConv
						}
					}
				}
			}

			// Assume job's elapsed time during this interval overlaps with interval's
			// boundaries
			startMark := intStartTS
			endMark := intEndTS

			// If job has not started between interval's start and end time,
			// elapsedTime should be zero. This can happen when job is in pending state
			// after submission
			if jobStartTS == 0 {
				endMark = startMark
				goto elapsed
			}

			// If job has already finished in the past we need to get boundaries from
			// job's start and end time. This case should not arrive in production as
			// there is no reason SLURM gives us the jobs that have finished in the past
			// that do not overlap with interval boundaries
			if jobEndTS > 0 && jobEndTS < intStartTS {
				startMark = jobStartTS
				endMark = jobEndTS
				goto elapsed
			}

			// If job has started **after** start of interval, we should mark job's start
			// time as start of elapsed time
			if jobStartTS > intStartTS {
				startMark = jobStartTS
			}
			// If job has ended before end of interval, we should mark job's end time
			// as elapsed end time.
			if jobEndTS > 0 && jobEndTS < intEndTS {
				endMark = jobEndTS
			}

		elapsed:
			// Get elapsed time of job in this interval in seconds
			elapsedSeconds := (endMark - startMark) / 1000

			// Get cpuSeconds and gpuSeconds of the current interval
			var cpuSeconds, gpuSeconds int64
			cpuSeconds = ncpus * elapsedSeconds
			gpuSeconds = ngpus * elapsedSeconds

			// Get cpuMemSeconds and gpuMemSeconds of current interval in MB
			var cpuMemSeconds, gpuMemSeconds int64
			if mem > 0 {
				cpuMemSeconds = mem * elapsedSeconds / toBytes["M"]
			} else {
				cpuMemSeconds = elapsedSeconds / toBytes["M"]
			}

			// Currently we use walltime as GPU mem time. This wont be a correct proxy
			// if MIG is enabled in GPUs where different portions of memory can be
			// allocated
			// NOTE: Not sure how SLURM outputs the gres/gpu when MIG is activated.
			// We need to check it and update this part to take GPU memory into account
			gpuMemSeconds = elapsedSeconds

			// Expand nodelist range expressions
			allNodes := helper.NodelistParser(components[sacctFieldMap["nodelist"]])
			nodelistExp := strings.Join(allNodes, "|")

			// Allocation
			allocation := models.Allocation{
				"nodes":   nnodes,
				"cpus":    ncpus,
				"mem":     mem,
				"gpus":    ngpus,
				"billing": billing,
			}

			// Tags
			tags := models.Tag{
				"uid":         uidInt,
				"gid":         gidInt,
				"partition":   components[sacctFieldMap["partition"]],
				"qos":         components[sacctFieldMap["qos"]],
				"exit_code":   components[sacctFieldMap["exitcode"]],
				"nodelist":    components[sacctFieldMap["nodelist"]],
				"nodelistexp": nodelistExp,
				"workdir":     components[sacctFieldMap["workdir"]],
			}

			// Make jobStats struct for each job and put it in jobs slice
			jobStat = models.Unit{
				ResourceManager: "slurm",
				UUID:            jobid,
				Name:            components[sacctFieldMap["jobname"]],
				Project:         components[sacctFieldMap["account"]],
				Grp:             components[sacctFieldMap["group"]],
				Usr:             components[sacctFieldMap["user"]],
				CreatedAt:       components[sacctFieldMap["submit"]],
				StartedAt:       components[sacctFieldMap["start"]],
				EndedAt:         components[sacctFieldMap["end"]],
				CreatedAtTS:     jobSubmitTS,
				StartedAtTS:     jobStartTS,
				EndedAtTS:       jobEndTS,
				Elapsed:         components[sacctFieldMap["elapsed"]],
				State:           components[sacctFieldMap["state"]],
				Allocation:      allocation,
				TotalWallTime:   elapsedSeconds,
				TotalCPUTime:    cpuSeconds,
				TotalGPUTime:    gpuSeconds,
				TotalCPUMemTime: cpuMemSeconds,
				TotalGPUMemTime: gpuMemSeconds,
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
