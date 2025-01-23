package slurm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	internal_osexec "github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

var (
	// SLURM AllocTRES gives memory as 200M, 250.5G and we dont know if it gives without
	// units. So, regex will capture the number and unit (if exists) and we convert it
	// to bytes.
	memRegex = regexp.MustCompile("([0-9.]+)([K|M|G|T]?)")
	toBytes  = map[string]int64{
		"K": 1024,
		"M": 1024 * 1024,
		"G": 1024 * 1024 * 1024,
		"T": 1024 * 1024 * 1024 * 1024,
		"Z": 1024 * 1024 * 1024 * 1024 * 1024,
	}

	// Required capabilities to execute SLURM commands.
	requiredCaps = []string{"cap_setuid", "cap_setgid"}
)

// Run preflights for CLI execution mode.
func preflightsCLI(slurm *slurmScheduler) error {
	// We hit this only when fetch mode is sacct command
	// Assume execMode is always native
	slurm.fetchMode = cliMode
	slurm.cmdExecMode = "native"
	slurm.logger.Debug("Using SLURM CLI commands")

	// If no sacct path is provided, assume it is available on PATH
	if slurm.cluster.CLI.Path == "" {
		path, err := exec.LookPath("sacct")
		if err != nil {
			slurm.logger.Error("Failed to find SLURM utility executables on PATH", "err", err)

			return err
		}

		slurm.cluster.CLI.Path = filepath.Dir(path)
	} else {
		// Check if slurm binary directory exists at the given path
		if _, err := os.Stat(slurm.cluster.CLI.Path); err != nil {
			slurm.logger.Error("Failed to open SLURM bin dir", "path", slurm.cluster.CLI.Path, "err", err)

			return err
		}
	}

	// Check if current capabilities have required caps
	haveCaps := true

	currentCaps := cap.GetProc().String()
	for _, cap := range requiredCaps {
		if !strings.Contains(currentCaps, cap) {
			haveCaps = false

			break
		}
	}

	// If current user is root or if current process has necessary caps setup security context
	if currentUser, err := user.Current(); err == nil && currentUser.Uid == "0" || haveCaps {
		slurm.cmdExecMode = capabilityMode
		slurm.logger.Info("Current user/process have enough privileges to execute SLURM commands", "user", currentUser.Username)

		var caps []cap.Value

		var err error

		for _, name := range requiredCaps {
			value, err := cap.FromName(name)
			if err != nil {
				slurm.logger.Error("Error parsing capability %s: %w", name, err)

				continue
			}

			caps = append(caps, value)
		}

		// If we choose capability mode, setup security context
		// Setup new security context(s)
		slurm.securityContexts[slurmExecCmdCtx], err = security.NewSecurityContext(
			slurmExecCmdCtx,
			caps,
			security.ExecAsUser,
			slurm.logger,
		)
		if err != nil {
			slurm.logger.Error("Failed to create a security context for SLURM", "err", err)

			return err
		}

		return nil
	}

	// sacct path
	sacctPath := filepath.Join(slurm.cluster.CLI.Path, "sacct")

	// Last attempt to run sacct with sudo
	if _, err := internal_osexec.ExecuteWithTimeout("sudo", []string{sacctPath, "--help"}, 5, nil); err == nil {
		slurm.cmdExecMode = sudoMode
		slurm.logger.Info("sudo will be used to execute SLURM commands")

		return nil
	}

	// If nothing works give up. In the worst case DB will be updated with only jobs from current user
	slurm.logger.Warn("SLURM commands will be executed as current user. Might not fetch jobs of all users")

	return nil
}

// Parse sacct command output and return batchjob slice.
func parseSacctCmdOutput(sacctOutput string, start time.Time, end time.Time) ([]models.Unit, int) {
	// No header in output
	sacctOutputLines := strings.Split(sacctOutput, "\n")

	// Update period
	intStartTS := start.UnixMilli()
	intEndTS := end.UnixMilli()

	// Get current location
	loc := end.Location()

	numJobs := 0

	jobs := make([]models.Unit, len(sacctOutputLines))

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

			// Convert time strings to configured time location
			eventTS := make(map[string]int64, 3)

			for _, c := range []string{"submit", "start", "end"} {
				if t, err := time.Parse(base.DatetimezoneLayout, components[sacctFieldMap[c]]); err == nil {
					components[sacctFieldMap[c]] = t.In(loc).Format(base.DatetimezoneLayout)
				}

				eventTS[c] = helper.TimeToTimestamp(base.DatetimezoneLayout, components[sacctFieldMap[c]])
			}

			// Parse alloctres to get billing, nnodes, ncpus, ngpus and mem
			var billing, nnodes, ncpus, ngpus int64

			var memString string

			for _, elem := range strings.Split(components[sacctFieldMap["alloctres"]], ",") {
				tresKV := strings.Split(elem, "=")
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
			// The following logic covers the cases when memory is of form 200M, 250.5G
			// and also without unit eg 20000, 40000. When there is no unit we assume
			// it is already in bytes
			matches := memRegex.FindStringSubmatch(memString)

			var mem int64

			if len(matches) >= 2 {
				if memFloat, err := strconv.ParseFloat(matches[1], 64); err == nil {
					if len(matches) == 3 {
						if unitConv, ok := toBytes[matches[2]]; ok {
							mem = int64(memFloat) * unitConv
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
			if eventTS["start"] == 0 {
				endMark = startMark

				goto elapsed
			}

			// If job has already finished in the past we need to get boundaries from
			// job's start and end time. This case should not arrive in production as
			// there is no reason SLURM gives us the jobs that have finished in the past
			// that do not overlap with interval boundaries
			if eventTS["end"] > 0 && eventTS["end"] < intStartTS {
				startMark = eventTS["start"]
				endMark = eventTS["end"]

				goto elapsed
			}

			// If job has started **after** start of interval, we should mark job's start
			// time as start of elapsed time
			if eventTS["start"] > intStartTS {
				startMark = eventTS["start"]
			}

			// If job has ended before end of interval, we should mark job's end time
			// as elapsed end time.
			if eventTS["end"] > 0 && eventTS["end"] < intEndTS {
				endMark = eventTS["end"]
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
				cpuMemSeconds = elapsedSeconds
			}

			// Currently we use walltime as GPU mem time. This wont be a correct proxy
			// if MIG is enabled in GPUs where different portions of memory can be
			// allocated
			// NOTE: Not sure how SLURM outputs the gres/gpu when MIG is activated.
			// We need to check it and update this part to take GPU memory into account
			if ngpus > 0 {
				gpuMemSeconds = elapsedSeconds
			}

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
				CreatedAtTS:     eventTS["submit"],
				StartedAtTS:     eventTS["start"],
				EndedAtTS:       eventTS["end"],
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

// Parse sacctmgr command output and return association.
func parseSacctMgrCmdOutput(sacctMgrOutput string, currentTime string) ([]models.User, []models.Project) {
	// No header in output
	sacctMgrOutputLines := strings.Split(sacctMgrOutput, "\n")

	wg := &sync.WaitGroup{}
	wg.Add(len(sacctMgrOutputLines))

	projectUserMap := make(map[string][]string)

	userProjectMap := make(map[string][]string)

	var users []string

	var projects []string

	for _, line := range sacctMgrOutputLines {
		go func(l string) {
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
		}(line)
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
	projectModels := make([]models.Project, len(projects))

	for i := range len(projects) {
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
	userModels := make([]models.User, len(users))

	for i := range len(users) {
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

// runSacctCmd executes sacct command and return output.
func (s *slurmScheduler) runSacctCmd(ctx context.Context, start, end time.Time) ([]byte, error) {
	// sacct path
	sacctPath := filepath.Join(s.cluster.CLI.Path, "sacct")

	// Use SLURM_TIME_FORMAT env var to get timezone offset
	env := []string{"SLURM_TIME_FORMAT=%Y-%m-%dT%H:%M:%S%z"}
	for name, value := range s.cluster.CLI.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", name, value))
	}

	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{
		"-D", "-X", "--noheader", "--allusers", "--parsable2",
		"--format", strings.Join(sacctFields, ","),
		"--state", strings.Join(slurmStates, ","),
		"--starttime", start.Format(base.DatetimeLayout),
		"--endtime", end.Format(base.DatetimeLayout),
	}

	// Run command as slurm user
	if s.cmdExecMode == capabilityMode {
		// Get security context
		var securityCtx *security.SecurityContext

		var ok bool
		if securityCtx, ok = s.securityContexts[slurmExecCmdCtx]; !ok {
			return nil, security.ErrNoSecurityCtx
		}

		cmd := []string{sacctPath}
		cmd = append(cmd, args...)

		// security context data
		dataPtr := &security.ExecSecurityCtxData{
			Context: ctx,
			Cmd:     cmd,
			Environ: env,
			Logger:  s.logger,
			UID:     0,
			GID:     0,
		}

		return executeInSecurityContext(securityCtx, dataPtr)
	} else if s.cmdExecMode == sudoMode {
		// Important that we need to export env as well as we set environment variables in the
		// command execution
		args = append([]string{"-E", sacctPath}, args...)

		return internal_osexec.ExecuteContext(ctx, sudoMode, args, env)
	}

	return internal_osexec.ExecuteContext(ctx, sacctPath, args, env)
}

// Run sacctmgr command and return output.
func (s *slurmScheduler) runSacctMgrCmd(ctx context.Context) ([]byte, error) {
	// Use jobIDRaw that outputs the array jobs as regular job IDs instead of id_array format
	args := []string{"--parsable2", "--noheader", "list", "associations", "format=Account,User"}

	// sacct path
	sacctMgrPath := filepath.Join(s.cluster.CLI.Path, "sacctmgr")

	// Use SLURM_TIME_FORMAT env var to get timezone offset
	var env []string
	for name, value := range s.cluster.CLI.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", name, value))
	}

	// Run command as slurm user
	if s.cmdExecMode == capabilityMode {
		// Get security context
		var securityCtx *security.SecurityContext

		var ok bool
		if securityCtx, ok = s.securityContexts[slurmExecCmdCtx]; !ok {
			return nil, security.ErrNoSecurityCtx
		}

		cmd := []string{sacctMgrPath}
		cmd = append(cmd, args...)

		// security context data
		dataPtr := &security.ExecSecurityCtxData{
			Context: ctx,
			Cmd:     cmd,
			Environ: env,
			Logger:  s.logger,
			UID:     0,
			GID:     0,
		}

		return executeInSecurityContext(securityCtx, dataPtr)
	} else if s.cmdExecMode == sudoMode {
		// Important that we need to export env as well as we set environment variables in the
		// command execution
		args = append([]string{"-E", sacctMgrPath}, args...)

		return internal_osexec.ExecuteContext(ctx, sudoMode, args, env)
	}

	return internal_osexec.ExecuteContext(ctx, sacctMgrPath, args, env)
}

// executeInSecurityContext executes SLURM command within a security context.
func executeInSecurityContext(
	securityCtx *security.SecurityContext,
	dataPtr *security.ExecSecurityCtxData,
) ([]byte, error) {
	// Read stdOut of command into data
	if err := securityCtx.Exec(dataPtr); err != nil {
		return nil, err
	}

	return dataPtr.StdOut, nil
}

// Run preflight checks on provided config.
func preflightChecks(s *slurmScheduler) error {
	// // Always prefer REST API mode if configured
	// if clusterConfig.Web.URL != "" {
	// 	return checkRESTAPI(clusterConfig, logger)
	// }
	return preflightsCLI(s)
}
