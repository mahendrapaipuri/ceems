// Main entrypoint for batchjob_stats

package main

import (
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	_ "modernc.org/sqlite"

	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
)

var (
	jobsTimestampFile   = "jobslasttimestamp"
	vacuumTimeStampFile = "vacuumlasttimestamp"
)

func main() {
	var (
		batchScheduler = kingpin.Flag(
			"batch.scheduler",
			"Name of batch scheduler (eg slurm, lsf, pbs).",
		).Default("slurm").String()
		dataPath = kingpin.Flag(
			"path.data",
			"Absolute path to a directory where job data is placed. SQLite DB that contains jobs stats will be saved to this directory.",
		).Default("/var/lib/jobstats").String()
		jobstatDBFile = kingpin.Flag(
			"path.db.name",
			"Name of the SQLite DB file that contains jobs stats.",
		).Default("jobstats.db").String()
		jobstatDBTable = kingpin.Flag(
			"path.db.table.name",
			"Name of the table in SQLite DB file that contains jobs stats.",
		).Default("jobs").String()
		retentionPeriod = kingpin.Flag(
			"data.retention.period",
			"Period in days for which job stats data will be retained.",
		).Default("365").Int()
		maxProcs = kingpin.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("batchjobstat"))
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Running batchjob_jobstat", "version", version.Info())
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
	jobCollector := jobstats.NewJobStats(
		logger,
		*batchScheduler,
		jobstatDBPath,
		*jobstatDBTable,
		*retentionPeriod,
		jobsLastTimeStampFile,
		vacuumLastTimeStampFile,
	)
	err := jobCollector.GetJobStats()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to get job stats", "err", err)
	}
	// defer pprof.StopCPUProfile()
}
