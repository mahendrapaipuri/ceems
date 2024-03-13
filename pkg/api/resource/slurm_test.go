package resource

import (
	"reflect"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

var (
	sacctCmdOutput = `JobID|Partition|QoS|Account|Group|GID|User|UID|CreatedAt|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|Ncpus|NodeList|JobName|WorkDir
1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|2023-02-21T14:37:07+0100|NA|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr
1481508|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T13:49:20+0100|2023-02-21T13:49:06+0100|2023-02-21T15:10:23+0100|00:08:17|4920|0:0|COMPLETED|billing=1,cpu=2,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr
1481510|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:08:20+0100|2023-02-21T15:08:06+0100|NA|00:00:17|17|0:0|RUNNING|billing=10,cpu=2,energy=15346,gres/gpu=1,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr
1481518|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:02:06+0100|2023-02-21T15:02:06+0100|2023-02-21T15:12:23+0100|00:08:17|4920|0:0|COMPLETED|billing=1,cpu=2,mem=4G,node=1|compute-[0-2]|test_script2|/home/usr`
	expectedBatchJobs = []models.Unit{
		{
			ID:              0,
			UUID:            "1479763",
			Name:            "test_script1",
			Project:         "acc1",
			Grp:             "grp",
			Usr:             "usr",
			CreatedAt:       "2023-02-21T14:37:02+0100",
			StartedAt:       "2023-02-21T14:37:07+0100",
			EndedAt:         "NA",
			CreatedAtTS:     1676986622000,
			StartedAtTS:     1676986627000,
			EndedAtTS:       0,
			ElapsedRaw:      3000,
			State:           "RUNNING",
			Allocation:      models.Generic{"cpus": int64(160), "gpus": int64(8), "mem": "320G", "nodes": int64(2)},
			TotalCPUBilling: 0,
			TotalGPUBilling: 72000,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-0",
				"nodelistexp": "compute-0",
				"exit_code":   "0:0",
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
			CreatedAt:       "2023-02-21T13:49:20+0100",
			StartedAt:       "2023-02-21T13:49:06+0100",
			EndedAt:         "2023-02-21T15:10:23+0100",
			CreatedAtTS:     1676983760000,
			StartedAtTS:     1676983746000,
			EndedAtTS:       1676988623000,
			ElapsedRaw:      4920,
			State:           "COMPLETED",
			Allocation:      models.Generic{"cpus": int64(2), "gpus": int64(0), "mem": "4G", "nodes": int64(1)},
			TotalCPUBilling: 623,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"exit_code":   "0:0",
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
			CreatedAt:       "2023-02-21T15:08:20+0100",
			StartedAt:       "2023-02-21T15:08:06+0100",
			EndedAt:         "NA",
			CreatedAtTS:     1676988500000,
			StartedAtTS:     1676988486000,
			EndedAtTS:       0,
			ElapsedRaw:      17,
			State:           "RUNNING",
			Allocation:      models.Generic{"cpus": int64(2), "gpus": int64(1), "mem": "4G", "nodes": int64(1)},
			TotalCPUBilling: 0,
			TotalGPUBilling: 4140,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"exit_code":   "0:0",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1000),
				"workdir":     "/home/usr",
			},
			Ignore: 0,
		},
		{
			ID:              0,
			UUID:            "1481518",
			Name:            "test_script2",
			Project:         "acc1",
			Grp:             "grp",
			Usr:             "usr",
			CreatedAt:       "2023-02-21T15:02:06+0100",
			StartedAt:       "2023-02-21T15:02:06+0100",
			EndedAt:         "2023-02-21T15:12:23+0100",
			CreatedAtTS:     1676988126000,
			StartedAtTS:     1676988126000,
			EndedAtTS:       1676988743000,
			ElapsedRaw:      4920,
			State:           "COMPLETED",
			Allocation:      models.Generic{"cpus": int64(2), "gpus": int64(0), "mem": "4G", "nodes": int64(1)},
			TotalCPUBilling: 617,
			Tags: models.Generic{
				"gid":         int64(1000),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"exit_code":   "0:0",
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
	start, _ := time.Parse(slurmTimeFormat, "2023-02-21T15:00:00+0100")
	end, _ := time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	batchJobs, numJobs := parseSacctCmdOutput(sacctCmdOutput, start, end)
	if !reflect.DeepEqual(batchJobs, expectedBatchJobs) {
		t.Errorf("Expected batch jobs %#v. \n\nGot %#v", expectedBatchJobs, batchJobs)
	}
	if numJobs != len(expectedBatchJobs) {
		t.Errorf("Expected batch jobs num %d. Got %d", len(expectedBatchJobs), numJobs)
	}
}
