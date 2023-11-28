package jobstats

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
)

var (
	sacctCmdOutput = `JobID|Cluster|Partition|Account|Group|GID|User|UID|Submit|Eligible|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|NCPUS|ReqCPUS|ReqMem|ReqTRES|Timelimit|NodeList|JobName|WorkDir
1479763|test|part1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02|2023-02-21T14:37:02|2023-02-21T14:37:07|2023-02-21T15:26:29|00:49:22|2962|0:0|CANCELLED by 302137|1|2|1|4G|cpu=1,mem=4G,node=1|01:00:00|compute-0|test_script2|/home/usr
1481508|test|part1|acc1|grp|1000|usr|1000|2023-02-21T15:48:20|2023-02-21T15:48:20|2023-02-21T15:49:06|2023-02-21T15:57:23|00:08:17|497|0:0|CANCELLED by 302137|2|40|40|80G|cpu=40,mem=80G,node=2|01:00:00|compute-[0-2]|test_script2|/home/usr`
	logger            = log.NewNopLogger()
	expectedBatchJobs = []batchJob{
		{Jobid: "1479763", Jobuuid: "d4b98df4-e636-ea14-59ff-37db752e6c30", Cluster: "test", Partition: "part1", Account: "acc1", Grp: "grp", Gid: "1000", Usr: "usr", Uid: "1000", Submit: "2023-02-21T14:37:02", Eligible: "2023-02-21T14:37:02", Start: "2023-02-21T14:37:07", End: "2023-02-21T15:26:29", Elapsed: "00:49:22", Elapsedraw: "2962", Exitcode: "0:0", State: "CANCELLED by 302137", Nnodes: "1", Ncpus: "2", Reqcpus: "1", Reqmem: "4G", Reqtres: "cpu=1,mem=4G,node=1", Timelimit: "01:00:00", Nodelist: "compute-0", NodelistExp: "compute-0", Jobname: "test_script2", Workdir: "/home/usr"},
		{Jobid: "1481508", Jobuuid: "44cbb62d-db8f-c0c5-e123-76766afbd190", Cluster: "test", Partition: "part1", Account: "acc1", Grp: "grp", Gid: "1000", Usr: "usr", Uid: "1000", Submit: "2023-02-21T15:48:20", Eligible: "2023-02-21T15:48:20", Start: "2023-02-21T15:49:06", End: "2023-02-21T15:57:23", Elapsed: "00:08:17", Elapsedraw: "497", Exitcode: "0:0", State: "CANCELLED by 302137", Nnodes: "2", Ncpus: "40", Reqcpus: "40", Reqmem: "80G", Reqtres: "cpu=40,mem=80G,node=2", Timelimit: "01:00:00", Nodelist: "compute-[0-2]", NodelistExp: "compute-0|compute-1|compute-2", Jobname: "test_script2", Workdir: "/home/usr"},
	}
)

func TestParseSacctCmdOutput(t *testing.T) {
	batchJobs, numJobs := parseSacctCmdOutput(sacctCmdOutput, logger)
	if !reflect.DeepEqual(batchJobs, expectedBatchJobs) {
		t.Errorf("Expected batch jobs %#v. Got %#v", expectedBatchJobs, batchJobs)
	}
	if numJobs != 2 {
		t.Errorf("Expected batch jobs num %d. Got %d", 2, numJobs)
	}
}
