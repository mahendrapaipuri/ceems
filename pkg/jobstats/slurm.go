package jobstats

import (
	"errors"
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
	jobLock         = sync.RWMutex{}
	slurmDateFormat = "2006-01-02T15:04:05"
	sacctPath       = JobstatsApp.Flag(
		"slurm.sacct.path",
		"Absolute path to sacct executable.",
	).Default("/usr/local/bin/sacct").String()
	runSacctWithSudo = JobstatsApp.Flag(
		"slurm.sacct.run.with.sudo",
		"sacct command will be run with sudo. This option requires current user has sudo "+
			"privileges on sacct command. This option is mutually exclusive with --slurm.sacct.run.as.slurmuser.",
	).Default("false").Bool()
	runSacctAsSlurmUser = JobstatsApp.Flag(
		"slurm.sacct.run.as.slurmuser",
		"sacct command will be run as slurm user. This option requires cap_setuid capability on "+
			"current process. This option is mutually exclusive with --slurm.sacct.run.with.sudo.",
	).Default("false").Bool()
)

// Run sacct command and return output
func runSacctCmd(startTime string, endTime string, logger log.Logger) ([]byte, error) {
	args := []string{"-D", "--allusers", "--parsable2",
		"--format", "jobid,partition,account,group,gid,user,uid,submit,start,end,elapsed,exitcode,state,nnodes,nodelist,jobname,workdir",
		"--state", "CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
		"--starttime", startTime, "--endtime", endTime}

	// Run command as slurm user
	if *runSacctAsSlurmUser {
		return helpers.ExecuteAs(*sacctPath, args, slurmUserUid, slurmUserGid, logger)
	}

	// Run command with sudo
	if *runSacctWithSudo {
		args = append([]string{*sacctPath}, args...)
		return helpers.Execute("sudo", args, logger)
	}
	return helpers.Execute(*sacctPath, args, logger)
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string, logger log.Logger) ([]BatchJob, int) {
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
			if len(components) < 17 {
				wg.Done()
				return
			}

			// Ignore job steps
			if strings.Contains(jobid, ".") {
				wg.Done()
				return
			}

			// Ignore jobs that never ran
			if components[14] == "None assigned" {
				wg.Done()
				return
			}

			// Generate UUID from jobID, uid, account, nodelist(lowercase)
			jobUuid, err := helpers.GetUuidFromString(
				[]string{
					strings.TrimSpace(components[0]),
					strings.TrimSpace(components[6]),
					strings.ToLower(strings.TrimSpace(components[2])),
					strings.ToLower(strings.TrimSpace(components[14])),
				},
			)
			if err != nil {
				level.Error(logger).
					Log("msg", "Failed to generate UUID for job", "jobid", jobid, "err", err)
				jobUuid = jobid
			}

			allNodes := NodelistParser(components[14])
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
				components[11],
				components[12],
				components[13],
				components[14],
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

	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), logger)
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

	// If current user is slurm pass checks
	if user, err := user.Current(); err == nil && (user.Name == "slurm" || user.Uid == "0") {
		if *runSacctAsSlurmUser {
			level.Warn(logger).Log(
				"msg",
				"Current user has already enough privileges to run sacct command. "+
					"Flag --slurm.sacct.run.as.slurmuser is redundant. Ignoring...",
			)
			*runSacctAsSlurmUser = false
		}
		if *runSacctWithSudo {
			level.Warn(logger).Log(
				"msg",
				"Current user has already enough privileges to run sacct command. "+
					"Flag --slurm.sacct.run.with.sudo is redundant. Ignoring...",
			)
			*runSacctWithSudo = false
		}
		return nil
	}

	// If runSacctAsSlurmUser and runSacctWithSudo are both true, raise error
	if *runSacctAsSlurmUser && *runSacctWithSudo {
		level.Error(logger).Log(
			"msg",
			"--slurm.sacct.run.as.slurmuser and --slurm.sacct.run.with.sudo are enabled. "+
				"They are mutually exclusive. Please choose one.",
		)
		return errors.New("Failed to parse command line options")
	}

	// if runSacctAsSlurmUser is true check if we have enough privileges
	if *runSacctAsSlurmUser {
		user, err := user.Lookup("slurm")
		if err != nil {
			level.Error(logger).Log("msg", "Failed to lookup slurm user for executing sacct cmd", "err", err)
			return err
		}

		slurmUserUid, err = strconv.Atoi(user.Uid)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to convert slurm user uid to int", "uid", slurmUserUid, "err", err)
			return err
		}

		slurmUserGid, err = strconv.Atoi(user.Gid)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to convert slurm user gid to int", "gid", slurmUserGid, "err", err)
			return err
		}

		_, err = helpers.ExecuteAs("sacct", []string{"--help"}, slurmUserUid, slurmUserGid, logger)
		if err != nil {
			level.Error(logger).Log(
				"msg",
				"Flag --slurm.sacct.run.as.slurmuser is set but current process does not have privileges. "+
					"Consider setting capabilities cap_setuid/cap_setgid on current "+
					"process", "err", err,
			)
			return err
		}
	}

	// if runSacctWithSudo is true check if we have enough privileges
	if *runSacctWithSudo {
		_, err := helpers.ExecuteWithTimeout("sudo", []string{*sacctPath, "--help"}, 5, logger)
		if err != nil {
			level.Error(logger).Log(
				"msg",
				"Flag --slurm.sacct.run.with.sudo is set but current process does not have privileges. "+
					"Consider adding an entry in sudoers file to give current user privileges to execute "+
					"sacct with sudo.", "err", err,
			)
			return err
		}
	}
	return nil
}
