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

var (
	slurmUserUid    int
	slurmUserGid    int
	execMode        = "native"
	jobLock         = sync.RWMutex{}
	slurmDateFormat = "2006-01-02T15:04:05"
	sacctPath       = JobstatsApp.Flag(
		"slurm.sacct.path",
		"Absolute path to sacct executable.",
	).Default("/usr/local/bin/sacct").String()
	slurmWalltimeCutoff = JobstatsApp.Flag(
		"slurm.elapsed.time.cutoff",
		"Jobs that have elapsed time less than this value (in seconds) will be ignored.",
	).Default("60").Int()
	// runSacctWithSudo = JobstatsApp.Flag(
	// 	"slurm.sacct.run.with.sudo",
	// 	"sacct command will be run with sudo. This option requires current user has sudo "+
	// 		"privileges on sacct command. This option is mutually exclusive with --slurm.sacct.run.as.slurmuser.",
	// ).Default("false").Bool()
	// runSacctAsSlurmUser = JobstatsApp.Flag(
	// 	"slurm.sacct.run.as.slurmuser",
	// 	"sacct command will be run as slurm user. This requires CAP_SET(UID,GID) capabilities on "+
	// 		"current process. This option is mutually exclusive with --slurm.sacct.run.with.sudo.",
	// ).Default("false").Bool()
)

// Run sacct command and return output
func runSacctCmd(startTime string, endTime string, logger log.Logger) ([]byte, error) {
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
				components[0],
				jobUuid,
				components[1],
				components[2],
				components[3],
				components[4],
				components[5],
				components[6],
				components[7],
				components[8],
				components[9],
				components[10],
				components[12],
				components[13],
				components[14],
				components[15],
				nodelistExp,
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

// Execute SLURM sacct command and return BatchJob object
func getSlurmJobs(start time.Time, end time.Time, logger log.Logger) ([]BatchJob, error) {
	startTime := start.Format(slurmDateFormat)
	endTime := end.Format(slurmDateFormat)

	level.Info(logger).Log("msg", "Retrieving Slurm jobs", "start", startTime, "end", endTime)

	sacctOutput, err := runSacctCmd(startTime, endTime, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute SLURM sacct command", "err", err)
		return []BatchJob{}, err
	}

	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), *slurmWalltimeCutoff, logger)
	level.Info(logger).Log("msg", "Number of Slurm jobs.", "njobs", numJobs)
	return jobs, nil
}

// Run basic sanity checks
func slurmChecks(logger log.Logger) error {
	// Check if sacct binary exists
	if _, err := os.Stat(*sacctPath); err != nil {
		level.Error(logger).Log("msg", "Failed to open sacct executable", "path", *sacctPath, "err", err)
		return err
	}

	// If current user is slurm or root pass checks
	if currentUser, err := user.Current(); err == nil && (currentUser.Username == "slurm" || currentUser.Uid == "0") {
		level.Debug(logger).
			Log("msg", "Current user have enough privileges to get job data for all users", "user", currentUser.Username)
		return nil
	}

	// // Check if sacctCmd can give job details of all users
	// // If admins want to use a wrapper that has setuid, it should give job activity ofm
	// // all users without any privs for current process
	// startTime := time.Now().Add(-5 * time.Minute).Format(slurmDateFormat)
	// sacctOut, err := helpers.Execute(*sacctPath, []string{"-D", "--parsable2", "--format=uid", "--start", startTime}, logger)
	// if err == nil {
	// 	allUids := slices.Compact(strings.Split(string(sacctOut), "\n")[1:])
	// 	fmt.Println(allUids)
	// 	if len(allUids) > 1 {
	// 		execMode = "native"
	// 		return nil
	// 	}
	// }

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
		return nil
	}

sudomode:
	// Last attempt to run sacct with sudo
	if _, err := helpers.ExecuteWithTimeout("sudo", []string{*sacctPath, "--help"}, 5, logger); err == nil {
		execMode = "sudo"
		level.Debug(logger).Log("msg", "sudo will be used to execute sacct command")
		return nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	return nil
}
