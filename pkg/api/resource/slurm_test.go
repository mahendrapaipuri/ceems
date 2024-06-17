package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

var (
	sacctCmdOutput = `JobID|Partition|QoS|Account|Group|GID|User|UID|CreatedAt|Start|End|Elapsed|ElapsedRaw|ExitCode|State|NNodes|Ncpus|NodeList|JobName|WorkDir
1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|2023-02-21T14:37:07+0100|NA|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr
1481508|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T13:49:20+0100|2023-02-21T13:49:06+0100|2023-02-21T15:10:23+0100|00:08:17|4920|0:0|COMPLETED|billing=1,cpu=2,mem=4M,node=1|compute-[0-2]|test_script2|/home/usr`
	expectedBatchJobs = []models.Unit{
		{
			ID:              0,
			ResourceManager: "slurm",
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
			Elapsed:         "01:49:22",
			TotalWallTime:   int64(900),
			State:           "RUNNING",
			Allocation: models.Generic{
				"cpus":    int64(160),
				"gpus":    int64(8),
				"mem":     int64(343597383680),
				"nodes":   int64(2),
				"billing": int64(80),
			},
			TotalCPUTime:    int64(144000),
			TotalGPUTime:    int64(7200),
			TotalCPUMemTime: 294912000,
			TotalGPUMemTime: 900,
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
			ResourceManager: "slurm",
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
			Elapsed:         "00:08:17",
			TotalWallTime:   int64(623),
			State:           "COMPLETED",
			Allocation: models.Generic{
				"cpus":    int64(2),
				"gpus":    int64(0),
				"mem":     int64(4194304),
				"nodes":   int64(1),
				"billing": int64(1),
			},
			TotalCPUTime:    int64(1246),
			TotalCPUMemTime: 2492,
			TotalGPUMemTime: 623,
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

func TestSLURMFetcherTwoClusters(t *testing.T) {
	// Write sacct executable
	sacctPath := filepath.Join(t.TempDir(), "sacct")
	sacctScript := fmt.Sprintf(`#!/bin/bash
printf """%s"""`, sacctCmdOutput)
	os.WriteFile(sacctPath, []byte(sacctScript), 0755)

	sacctDir := filepath.Dir(sacctPath)

	// mock config
	clusters := []models.Cluster{
		{
			ID:      "slurm-0",
			Manager: "slurm",
			CLI:     models.CLIConfig{Path: sacctDir},
		},
		{
			ID:      "slurm-1",
			Manager: "slurm",
			CLI:     models.CLIConfig{Path: sacctDir},
		},
	}

	slurm, err := NewSlurmScheduler(clusters[0], log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create SLURM instance: %s", err)
	}

	start, _ := time.Parse(slurmTimeFormat, "2023-02-21T15:00:00+0100")
	end, _ := time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	expected := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID:      "slurm-0",
				Manager: "slurm",
			},
			Units: expectedBatchJobs,
		},
	}

	batchJobs, err := slurm.Fetch(start, end)
	if err != nil {
		t.Errorf("Failed to fetch jobs: %s", err)
	}
	if !reflect.DeepEqual(batchJobs[0].Units, expected[0].Units) {
		t.Errorf("Expected batch jobs %#v. \n\nGot %#v", expected, batchJobs)
	}
}
