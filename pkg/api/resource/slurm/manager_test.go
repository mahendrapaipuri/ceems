package slurm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/stretchr/testify/require"
)

var (
	start, _       = time.Parse(slurmTimeFormat, "2023-02-21T15:00:00+0100")
	end, _         = time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	current, _     = time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	sacctCmdOutput = `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|2023-02-21T14:37:07+0100|NA|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320.5G,node=2|compute-0|test_script1|/home/usr
1481508|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T13:49:20+0100|2023-02-21T13:49:06+0100|2023-02-21T15:10:23+0100|00:08:17|4920|0:0|COMPLETED|billing=1,cpu=2,mem=4M,node=1|compute-[0-2]|test_script2|/home/usr`
	sacctMgrCmdOutput = `root|
root|root
prj1|
prj2|
prj3|
prj3|usr1
prj3|usr2
prj4|
prj4|usr2
prj4|usr3`
	expectedBatchJobs = []models.Unit{
		{
			ID:              0,
			ResourceManager: "slurm",
			UUID:            "1479763",
			Name:            "test_script1",
			Project:         "acc1",
			Group:           "grp",
			User:            "usr",
			CreatedAt:       "2023-02-21T14:37:02+0100",
			StartedAt:       "2023-02-21T14:37:07+0100",
			EndedAt:         "NA",
			CreatedAtTS:     1676986622000,
			StartedAtTS:     1676986627000,
			EndedAtTS:       0,
			Elapsed:         "01:49:22",
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(900),
				"alloc_cputime":    models.JSONFloat(144000),
				"alloc_gputime":    models.JSONFloat(7200),
				"alloc_gpumemtime": models.JSONFloat(900),
				"alloc_cpumemtime": models.JSONFloat(294912000),
			},
			State: "RUNNING",
			Allocation: models.Generic{
				"cpus":    int64(160),
				"gpus":    int64(8),
				"mem":     int64(343597383680),
				"nodes":   int64(2),
				"billing": int64(80),
			},
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
			Group:           "grp",
			User:            "usr",
			CreatedAt:       "2023-02-21T13:49:20+0100",
			StartedAt:       "2023-02-21T13:49:06+0100",
			EndedAt:         "2023-02-21T15:10:23+0100",
			CreatedAtTS:     1676983760000,
			StartedAtTS:     1676983746000,
			EndedAtTS:       1676988623000,
			Elapsed:         "00:08:17",
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(623),
				"alloc_cputime":    models.JSONFloat(1246),
				"alloc_cpumemtime": models.JSONFloat(2492),
				"alloc_gputime":    models.JSONFloat(0),
				"alloc_gpumemtime": models.JSONFloat(0),
			},
			State: "COMPLETED",
			Allocation: models.Generic{
				"cpus":    int64(2),
				"gpus":    int64(0),
				"mem":     int64(4194304),
				"nodes":   int64(1),
				"billing": int64(1),
			},
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
	expectedProjects = []models.Project{
		{
			Name:          "prj3",
			Users:         models.List{"usr1", "usr2"},
			LastUpdatedAt: "2023-02-21T15:15:00+0100",
		},
		{
			Name:          "prj4",
			Users:         models.List{"usr2", "usr3"},
			LastUpdatedAt: "2023-02-21T15:15:00+0100",
		},
	}
	expectedUsers = []models.User{
		{
			Name:          "usr1",
			Projects:      models.List{"prj3"},
			LastUpdatedAt: "2023-02-21T15:15:00+0100",
		},
		{
			Name:          "usr2",
			Projects:      models.List{"prj3", "prj4"},
			LastUpdatedAt: "2023-02-21T15:15:00+0100",
		},
		{
			Name:          "usr3",
			Projects:      models.List{"prj4"},
			LastUpdatedAt: "2023-02-21T15:15:00+0100",
		},
	}
)

func TestSLURMFetcherMultiCluster(t *testing.T) {
	// Write sacct and sacctmgr executables
	tmpDir := t.TempDir()
	sacctPath := filepath.Join(tmpDir, "sacct")
	sacctScript := fmt.Sprintf(`#!/bin/bash
printf """%s"""`, sacctCmdOutput)
	os.WriteFile(sacctPath, []byte(sacctScript), 0o700) // #nosec

	sacctMgrPath := filepath.Join(tmpDir, "sacctmgr")
	sacctMgrScript := fmt.Sprintf(`#!/bin/bash
printf """%s"""`, sacctMgrCmdOutput)
	os.WriteFile(sacctMgrPath, []byte(sacctMgrScript), 0o700) // #nosec

	sacctMgrDir := filepath.Dir(sacctMgrPath)

	// mock config
	clusters := []models.Cluster{
		{
			ID:      "slurm-0",
			Manager: "slurm",
			CLI:     models.CLIConfig{Path: sacctMgrDir},
		},
		{
			ID:      "slurm-1",
			Manager: "slurm",
			CLI:     models.CLIConfig{Path: sacctMgrDir},
		},
	}

	start, _ = time.Parse(slurmTimeFormat, "2023-02-21T15:00:00+0100")
	end, _ = time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	current, _ = time.Parse(slurmTimeFormat, "2023-02-21T15:15:00+0100")
	ctx := context.Background()

	for _, cluster := range clusters {
		slurm, err := New(cluster, slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)

		_, err = slurm.FetchUnits(ctx, start, end)
		require.NoError(t, err)

		_, _, err = slurm.FetchUsersProjects(ctx, current)
		require.NoError(t, err)
	}
}
