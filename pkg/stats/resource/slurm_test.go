package resource

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/stats/types"
)

var (
	sacctCmdOutput = `JobID|Partition|QoS|Account|Group|GID|User|UID|Submit|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|Ncpus|NodeList|JobName|WorkDir
1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|2023-02-21T14:37:07+0100|2023-02-21T15:26:29+0100|00:49:22|3000|0:0|CANCELLED by 302137|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr
1481508|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20+0100|2023-02-21T15:49:06+0100|2023-02-21T15:57:23+0100|00:08:17|4920|0:0|CANCELLED by 302137|billing=1,cpu=2,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr
1481510|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20+0100|2023-02-21T15:49:06+0100|2023-02-21T15:57:23+0100|00:00:17|17|0:0|CANCELLED by 302137|billing=10,cpu=2,energy=15346,gres/gpu=1,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr`
	logger            = log.NewNopLogger()
	expectedBatchJobs = []types.Unit{
		{
			UUID:            "1479763",
			Partition:       "part1",
			QoS:             "qos1",
			Project:         "acc1",
			Grp:             "grp",
			Gid:             1000,
			Usr:             "usr",
			Uid:             1000,
			Submit:          "2023-02-21T14:37:02+0100",
			Start:           "2023-02-21T14:37:07+0100",
			End:             "2023-02-21T15:26:29+0100",
			SubmitTS:        1676986622000,
			StartTS:         1676986627000,
			EndTS:           1676989589000,
			Elapsed:         "00:49:22",
			ElapsedRaw:      3000,
			Exitcode:        "0:0",
			State:           "CANCELLED by 302137",
			AllocNodes:      2,
			AllocCPUs:       160,
			AllocGPUs:       8,
			AllocMem:        "320G",
			Nodelist:        "compute-0",
			NodelistExp:     "compute-0",
			Name:            "test_script1",
			WorkDir:         "/home/usr",
			TotalGPUBilling: 80,
		},
		{
			UUID:            "1481508",
			Partition:       "part1",
			QoS:             "qos1",
			Project:         "acc1",
			Grp:             "grp",
			Gid:             1000,
			Usr:             "usr",
			Uid:             1000,
			Submit:          "2023-02-21T15:48:20+0100",
			Start:           "2023-02-21T15:49:06+0100",
			End:             "2023-02-21T15:57:23+0100",
			SubmitTS:        1676990900000,
			StartTS:         1676990946000,
			EndTS:           1676991443000,
			Elapsed:         "00:08:17",
			ElapsedRaw:      4920,
			Exitcode:        "0:0",
			State:           "CANCELLED by 302137",
			AllocNodes:      1,
			AllocCPUs:       2,
			AllocMem:        "4G",
			Nodelist:        "compute-[0-2]",
			NodelistExp:     "compute-0|compute-1|compute-2",
			Name:            "test_script2",
			WorkDir:         "/home/usr",
			TotalCPUBilling: 1,
		},
		{
			UUID:            "1481510",
			Partition:       "part1",
			QoS:             "qos1",
			Project:         "acc1",
			Grp:             "grp",
			Gid:             1000,
			Usr:             "usr",
			Uid:             1000,
			Submit:          "2023-02-21T15:48:20+0100",
			Start:           "2023-02-21T15:49:06+0100",
			End:             "2023-02-21T15:57:23+0100",
			SubmitTS:        1676990900000,
			StartTS:         1676990946000,
			EndTS:           1676991443000,
			Elapsed:         "00:00:17",
			ElapsedRaw:      17,
			Exitcode:        "0:0",
			State:           "CANCELLED by 302137",
			AllocNodes:      1,
			AllocCPUs:       2,
			AllocGPUs:       1,
			AllocMem:        "4G",
			Nodelist:        "compute-[0-2]",
			NodelistExp:     "compute-0|compute-1|compute-2",
			Name:            "test_script2",
			WorkDir:         "/home/usr",
			TotalGPUBilling: 10,
		},
	}
)

func TestParseSacctCmdOutput(t *testing.T) {
	batchJobs, numJobs := parseSacctCmdOutput(sacctCmdOutput, logger)
	if !reflect.DeepEqual(batchJobs, expectedBatchJobs) {
		t.Errorf("Expected batch jobs %#v. \n\nGot %#v", expectedBatchJobs, batchJobs)
	}
	if numJobs != len(expectedBatchJobs) {
		t.Errorf("Expected batch jobs num %d. Got %d", len(expectedBatchJobs), numJobs)
	}
}
