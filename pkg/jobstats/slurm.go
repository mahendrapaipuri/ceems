package jobstats

import (
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/utils"
)

var (
	jobLock         = sync.RWMutex{}
	slurmDateFormat = "2006-01-02T15:04:05"
	sacctPath       = kingpin.Flag(
		"slurm.sacct.path",
		"Absolute path to sacct executable.",
	).Default("/usr/bin/sacct").String()
)

// Run sacct command and return output
func runSacctCmd(startTime string, endTime string, logger log.Logger) ([]byte, error) {
	args := []string{"-X", "--allusers", "--parsable2",
		"--format", "jobid,cluster,partition,account,group,gid,user,uid,submit,eligible,start,end,elapsed,elapsedraw,exitcode,state,nnodes,ncpus,reqcpus,reqmem,reqtres,timelimit,nodelist,jobname,workdir",
		"--state", "CANCELLED,COMPLETED,FAILED,NODE_FAIL,PREEMPTED,TIMEOUT",
		"--starttime", startTime, "--endtime", endTime}
	return utils.Execute(*sacctPath, args, logger)
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string, logger log.Logger) ([]batchJob, int) {
	// Strip first line
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")[1:]
	var numJobs int = 0
	var jobs = make([]batchJob, len(sacctOutputLines))
	wg := &sync.WaitGroup{}
	// Exclude first line
	wg.Add(len(sacctOutputLines))
	for iline, line := range sacctOutputLines {
		go func(i int, l string) {
			var jobStat batchJob
			components := strings.Split(l, "|")
			jobid := components[0]
			// Ignore job steps
			if strings.Contains(jobid, ".") {
				wg.Done()
				return
			}
			if len(components) < 25 {
				wg.Done()
				return
			}
			jobUuid, err := utils.GetUuidFromString([]string{components[0], components[6], components[8], components[24]})
			if err != nil {
				level.Error(logger).Log("msg", "Failed to generate UUID for job", "jobid", jobid, "err", err)
				jobUuid = jobid
			}
			allNodes := utils.NodelistParser(components[22])
			nodelistExp := strings.Join(allNodes, "|")
			jobStat = batchJob{
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
				components[15],
				components[16],
				components[17],
				components[18],
				components[19],
				components[20],
				components[21],
				components[22],
				nodelistExp,
				components[23],
				components[24],
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

// Execute SLURM sacct command and return batchJob object
func getSlurmJobs(start time.Time, end time.Time, logger log.Logger) ([]batchJob, error) {
	startTime := start.Format(slurmDateFormat)
	endTime := end.Format(slurmDateFormat)
	level.Info(logger).Log("msg", "Retrieving Slurm jobs", "start", startTime, "end", endTime)
	sacctOutput, err := runSacctCmd(startTime, endTime, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute SLURM sacct command", "err", err)
		return []batchJob{}, err
	}
	jobs, numJobs := parseSacctCmdOutput(string(sacctOutput), logger)
	level.Info(logger).Log("msg", "Number of Slurm jobs.", "njobs", numJobs)
	return jobs, nil
}

// Run basic sanity checks
func slurmChecks(logger log.Logger) {
	if user, err := user.Current(); err == nil && user.Uid != "0" && user.Name != "slurm" {
		level.Warn(logger).Log("msg", "Batch Job Stats needs to run as root user or slurm user.")
	}
}
