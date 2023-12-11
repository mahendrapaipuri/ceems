// Main entrypoint for batchjob_stats

package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/go-kit/log/level"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
)

var (
	jobsTimestampFile   = "jobslasttimestamp"
	vacuumTimeStampFile = "vacuumlasttimestamp"
)

func main() {
	var (
		batchScheduler = jobstats.JobstatDBApp.Flag(
			"batch.scheduler",
			"Name of batch scheduler (eg slurm, lsf, pbs).",
		).Default("slurm").String()
		dataPath = jobstats.JobstatDBApp.Flag(
			"path.data",
			"Absolute path to a directory where job data is placed. SQLite DB that contains jobs stats will be saved to this directory.",
		).Default("/var/lib/jobstats").String()
		jobstatDBFile = jobstats.JobstatDBApp.Flag(
			"db.name",
			"Name of the SQLite DB file that contains jobs stats.",
		).Default("jobstats.db").String()
		jobstatDBTable = jobstats.JobstatDBApp.Flag(
			"db.table.name",
			"Name of the table in SQLite DB file that contains jobs stats.",
		).Default("jobs").String()
		updateInterval = jobstats.JobstatDBApp.Flag(
			"db.update.interval",
			"Time period in seconds at which DB will be updated with jobs stats.",
		).Default("1800").Int()
		retentionPeriod = jobstats.JobstatDBApp.Flag(
			"data.retention.period",
			"Period in days for which job stats data will be retained.",
		).Default("365").Int()
		maxProcs = jobstats.JobstatDBApp.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(jobstats.JobstatDBApp, promlogConfig)
	jobstats.JobstatDBApp.Version(version.Print(jobstats.JobstatDBAppName))
	jobstats.JobstatDBApp.UsageWriter(os.Stdout)
	jobstats.JobstatDBApp.HelpFlag.Short('h')
	_, err := jobstats.JobstatDBApp.Parse(os.Args[1:])
	if err != nil {
		panic(fmt.Sprintf("Failed to parse %s command", jobstats.JobstatDBAppName))
	}
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Running %s", jobstats.JobstatDBAppName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	jobstatDBPath := filepath.Join(*dataPath, *jobstatDBFile)
	jobsLastTimeStampFile := filepath.Join(*dataPath, jobsTimestampFile)
	vacuumLastTimeStampFile := filepath.Join(*dataPath, vacuumTimeStampFile)

	// f, err := os.Create("myprogram.prof")
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// pprof.StartCPUProfile(f)
	jobCollector := jobstats.NewJobStatsDB(
		logger,
		*batchScheduler,
		jobstatDBPath,
		*jobstatDBTable,
		*retentionPeriod,
		jobsLastTimeStampFile,
		vacuumLastTimeStampFile,
	)

	// Create a channel to propagate signals
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Start a ticker
	ticker := time.NewTicker(time.Second * time.Duration(*updateInterval))
	defer ticker.Stop()

loop:
	for {
		level.Info(logger).Log("msg", "Updating JobStats DB")
		err := jobCollector.GetJobStats()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get job stats", "err", err)
		}

		select {
		case <-ticker.C:
			continue
		case <-interrupt:
			level.Info(logger).Log("msg", "Received Interrupt. Stopping DB update")
			break loop
		}
	}
	// defer pprof.StopCPUProfile()
}
