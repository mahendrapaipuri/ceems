package slurm

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	internal_osexec "github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Run preflights for CLI execution mode
func preflightsCLI(slurm *slurmScheduler) error {
	// We hit this only when fetch mode is sacct command
	// Assume execMode is always native
	slurm.fetchMode = "cli"
	slurm.cmdExecMode = "native"
	level.Debug(slurm.logger).Log("msg", "SLURM jobs will be fetched using CLI commands")

	// If no sacct path is provided, assume it is available on PATH
	if slurm.cluster.CLI.Path == "" {
		path, err := exec.LookPath("sacct")
		if err != nil {
			level.Error(slurm.logger).Log("msg", "Failed to find SLURM utility executables on PATH", "err", err)
			return err
		}
		slurm.cluster.CLI.Path = filepath.Dir(path)
	} else {
		// Check if slurm binary directory exists at the given path
		if _, err := os.Stat(slurm.cluster.CLI.Path); err != nil {
			level.Error(slurm.logger).Log("msg", "Failed to open SLURM bin dir", "path", slurm.cluster.CLI.Path, "err", err)
			return err
		}
	}

	// sacct path
	sacctPath := filepath.Join(slurm.cluster.CLI.Path, "sacct")

	// If current user is slurm or root pass checks
	if currentUser, err := user.Current(); err == nil && (currentUser.Username == "slurm" || currentUser.Uid == "0") {
		level.Info(slurm.logger).
			Log("msg", "Current user have enough privileges to execute SLURM commands", "user", currentUser.Username)
		return nil
	}

	// First try to run as slurm user in a subprocess. If current process have capabilities
	// it will be a success
	slurmUser, err := user.Lookup("slurm")
	if err != nil {
		level.Debug(slurm.logger).
			Log("msg", "User slurm not found. Next attempt to execute SLURM commands with sudo", "err", err)
		goto sudomode
	}

	slurmUserUID, err = strconv.Atoi(slurmUser.Uid)
	if err != nil {
		level.Debug(slurm.logger).
			Log("msg", "Failed to convert SLURM user uid to int. Next attempt to execute SLURM commands with sudo", "uid", slurmUserUID, "err", err)
		goto sudomode
	}

	slurmUserGID, err = strconv.Atoi(slurmUser.Gid)
	if err != nil {
		level.Debug(slurm.logger).
			Log("msg", "Failed to convert SLURM user gid to int. Next attempt to execute SLURM commands with sudo", "gid", slurmUserGID, "err", err)
		goto sudomode
	}

	if _, err := internal_osexec.ExecuteAs(sacctPath, []string{"--help"}, slurmUserUID, slurmUserGID, nil, slurm.logger); err == nil {
		slurm.cmdExecMode = "cap"
		level.Info(slurm.logger).Log("msg", "Linux capabilities will be used to execute SLURM commands as slurm user")
		return nil
	}

sudomode:
	// Last attempt to run sacct with sudo
	if _, err := internal_osexec.ExecuteWithTimeout("sudo", []string{sacctPath, "--help"}, 5, nil, slurm.logger); err == nil {
		slurm.cmdExecMode = "sudo"
		level.Info(slurm.logger).Log("msg", "sudo will be used to execute SLURM commands")
		return nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	level.Warn(slurm.logger).
		Log("msg", "SLURM commands will be executed as current user. Might not fetch jobs of all users")
	return nil
}

// Parse sacct command output and return batchjob slice
func parseSacctCmdOutput(sacctOutput string, start time.Time, end time.Time) ([]models.Unit, int) {
	// No header in output
	sacctOutputLines := strings.Split(string(sacctOutput), "\n")

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
				Group:           components[sacctFieldMap["group"]],
				User:            components[sacctFieldMap["user"]],
				CreatedAt:       components[sacctFieldMap["submit"]],
				StartedAt:       components[sacctFieldMap["start"]],
				EndedAt:         components[sacctFieldMap["end"]],
				CreatedAtTS:     jobSubmitTS,
				StartedAtTS:     jobStartTS,
				EndedAtTS:       jobEndTS,
				Elapsed:         components[sacctFieldMap["elapsed"]],
				State:           components[sacctFieldMap["state"]],
				Allocation:      allocation,
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(elapsedSeconds),
					"alloc_cputime":    models.JSONFloat(cpuSeconds),
					"alloc_cpumemtime": models.JSONFloat(cpuMemSeconds),
					"alloc_gputime":    models.JSONFloat(gpuSeconds),
					"alloc_gpumemtime": models.JSONFloat(gpuMemSeconds),
				},
				Tags: tags,
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

// Parse sacctmgr command output and return association
func parseSacctMgrCmdOutput(sacctMgrOutput string, currentTime string) ([]models.User, []models.Project) {
	// No header in output
	sacctMgrOutputLines := strings.Split(string(sacctMgrOutput), "\n")

	wg := &sync.WaitGroup{}
	wg.Add(len(sacctMgrOutputLines))

	var projectUserMap = make(map[string][]string)
	var userProjectMap = make(map[string][]string)
	var users []string
	var projects []string
	for iline, line := range sacctMgrOutputLines {
		go func(i int, l string) {
			components := strings.Split(l, "|")

			// Ignore if we cannot get all components
			if len(components) < 2 {
				wg.Done()
				return
			}

			// Ignore root user/account
			if components[0] == "root" || components[1] == "root" {
				wg.Done()
				return
			}

			// Ignore empty lines
			if components[0] == "" || components[1] == "" {
				wg.Done()
				return
			}

			// Add user project association to map
			assocLock.Lock()
			userProjectMap[components[1]] = append(userProjectMap[components[1]], components[0])
			projectUserMap[components[0]] = append(projectUserMap[components[0]], components[1])
			users = append(users, components[1])
			projects = append(projects, components[0])
			assocLock.Unlock()
			wg.Done()
		}(iline, line)
	}
	wg.Wait()

	// Here we sort projects and users to get deterministic
	// output as order in Go maps is undefined

	// Sort and compact projects
	slices.Sort(projects)
	projects = slices.Compact(projects)

	// Sort and compact users slice to get unique users
	slices.Sort(users)
	users = slices.Compact(users)

	// Transform map into slice of projects
	var projectModels = make([]models.Project, len(projects))
	for i := 0; i < len(projects); i++ {
		projectUsers := projectUserMap[projects[i]]

		// Sort users
		slices.Sort(projectUsers)
		var usersList models.List
		for _, u := range slices.Compact(projectUsers) {
			usersList = append(usersList, u)
		}

		// Make Association
		projectModels[i] = models.Project{
			Name:          projects[i],
			Users:         usersList,
			LastUpdatedAt: currentTime,
		}
	}

	// Transform map into slice of users
	var userModels = make([]models.User, len(users))
	for i := 0; i < len(users); i++ {
		userProjects := userProjectMap[users[i]]

		// Sort projects
		slices.Sort(userProjects)
		var projectsList models.List
		for _, p := range slices.Compact(userProjects) {
			projectsList = append(projectsList, p)
		}

		// Make Association
		userModels[i] = models.User{
			Name:          users[i],
			Projects:      projectsList,
			LastUpdatedAt: currentTime,
		}
	}
	return userModels, projectModels
}
