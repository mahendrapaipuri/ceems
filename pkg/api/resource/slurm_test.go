package resource

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

var (
	sacctCmdOutput = `JobID|Partition|QoS|Account|Group|GID|User|UID|Submit|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|Ncpus|NodeList|JobName|WorkDir
1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|2023-02-21T14:37:07+0100|2023-02-21T15:26:29+0100|00:49:22|3000|0:0|CANCELLED by 302137|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr
1481508|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20+0100|2023-02-21T15:49:06+0100|2023-02-21T15:57:23+0100|00:08:17|4920|0:0|CANCELLED by 302137|billing=1,cpu=2,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr
1481510|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20+0100|2023-02-21T15:49:06+0100|2023-02-21T15:57:23+0100|00:00:17|17|0:0|CANCELLED by 302137|billing=10,cpu=2,energy=15346,gres/gpu=1,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr`
	logger            = log.NewNopLogger()
	expectedBatchJobs = []models.Unit{
		{
			ID:              0,
			UUID:            "1479763",
			Name:            "test_script1",
			Project:         "acc1",
			Grp:             "grp",
			Usr:             "usr",
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
			Allocation:      models.Generic{"cpus": int64(160), "gpus": int64(8), "mem": "320G", "nodes": int64(2)},
			TotalGPUBilling: 80,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-0",
				"nodelistexp": "compute-0",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1000),
				"workdir":     "/home/usr",
			},
			Ignore: 0,
		},
		{
			ID:              0,
			UUID:            "1481508",
			Name:            "test_script2",
			Project:         "acc1",
			Grp:             "grp",
			Usr:             "usr",
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
			Allocation:      models.Generic{"cpus": int64(2), "gpus": int64(0), "mem": "4G", "nodes": int64(1)},
			TotalCPUBilling: 1,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1000),
				"workdir":     "/home/usr",
			},
			Ignore: 0,
		},
		{
			ID:              0,
			UUID:            "1481510",
			Name:            "test_script2",
			Project:         "acc1",
			Grp:             "grp",
			Usr:             "usr",
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
			Allocation:      models.Generic{"cpus": int64(2), "gpus": int64(1), "mem": "4G", "nodes": int64(1)},
			TotalGPUBilling: 10,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1000),
				"workdir":     "/home/usr",
			},
			Ignore: 0,
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
