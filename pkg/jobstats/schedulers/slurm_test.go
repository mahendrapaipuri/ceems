package schedulers

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/base"
)

var (
	sacctCmdOutput = `JobID|Partition|Account|Group|GID|User|UID|Submit|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|NodeList|JobName|WorkDir
1479763|part1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02|2023-02-21T14:37:07|2023-02-21T15:26:29|00:49:22|3000|0:0|CANCELLED by 302137|1|compute-0|test_script1|/home/usr
1481508|part1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20|2023-02-21T15:49:06|2023-02-21T15:57:23|00:08:17|4920|0:0|CANCELLED by 302137|2|compute-[0-2]|test_script2|/home/usr
1481510|part1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20|2023-02-21T15:49:06|2023-02-21T15:57:23|00:00:17|17|0:0|CANCELLED by 302137|2|compute-[0-2]|test_script2|/home/usr`
	logger            = log.NewNopLogger()
	expectedBatchJobs = []base.BatchJob{
		{
			Jobid:       "1479763",
			Jobuuid:     "a3bd0ca1-5021-7e4d-943e-9529f8390f05",
			Partition:   "part1",
			Account:     "acc1",
			Grp:         "grp",
			Gid:         "1000",
			Usr:         "usr",
			Uid:         "1000",
			Submit:      "2023-02-21T14:37:02",
			Start:       "2023-02-21T14:37:07",
			End:         "2023-02-21T15:26:29",
			Elapsed:     "00:49:22",
			Exitcode:    "0:0",
			State:       "CANCELLED by 302137",
			Nnodes:      "1",
			Nodelist:    "compute-0",
			NodelistExp: "compute-0",
		},
		{
			Jobid:       "1481508",
			Jobuuid:     "759a4e2e-1e47-c58b-3d1b-885a03ca323b",
			Partition:   "part1",
			Account:     "acc1",
			Grp:         "grp",
			Gid:         "1000",
			Usr:         "usr",
			Uid:         "1000",
			Submit:      "2023-02-21T15:48:20",
			Start:       "2023-02-21T15:49:06",
			End:         "2023-02-21T15:57:23",
			Elapsed:     "00:08:17",
			Exitcode:    "0:0",
			State:       "CANCELLED by 302137",
			Nnodes:      "2",
			Nodelist:    "compute-[0-2]",
			NodelistExp: "compute-0|compute-1|compute-2",
		},
		{}, // Ignored jobs will have empty struct
	}
)

func TestParseSacctCmdOutput(t *testing.T) {
	batchJobs, numJobs := parseSacctCmdOutput(sacctCmdOutput, 60, logger)
	if !reflect.DeepEqual(batchJobs, expectedBatchJobs) {
		t.Errorf("Expected batch jobs %#v. \n\nGot %#v", expectedBatchJobs, batchJobs)
	}
	if numJobs != 2 {
		t.Errorf("Expected batch jobs num %d. Got %d", 2, numJobs)
	}
}
